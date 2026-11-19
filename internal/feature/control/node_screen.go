//go:build !board_biscuit

package control

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"math"
	"regexp"
	"sync"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4/seekablebuffer"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
	"github.com/ygelfand/echolocal/internal/hardware/display"
	connection "github.com/ygelfand/echolocal/internal/openclaw"
	"github.com/ygelfand/libcountertop/pkg/display/surface"
	"github.com/ygelfand/libcountertop/pkg/runtime/control/screencmd"
	"golang.org/x/image/draw"
)

var nodeCaptureGate = make(chan struct{}, 1)

type nodeSnapshotParams struct {
	ExecutionID string   `json:"executionId"`
	ScreenIndex int      `json:"screenIndex"`
	MaxWidth    int      `json:"maxWidth"`
	Quality     *float64 `json:"quality"`
	Format      string   `json:"format"`
}

func encodeNodeSnapshot(img image.Image, p nodeSnapshotParams) (any, error) {
	if (p.ExecutionID != "" && !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(p.ExecutionID)) || p.ScreenIndex != 0 || p.MaxWidth < 0 || (p.Format != "" && p.Format != "jpeg" && p.Format != "png") ||
		(p.Quality != nil && (math.IsNaN(*p.Quality) || *p.Quality < 0 || *p.Quality > 1)) {
		return nil, errors.New("invalid_params")
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	if p.MaxWidth > 0 && (w > p.MaxWidth || h > p.MaxWidth) {
		scale := float64(p.MaxWidth) / float64(max(w, h))
		dst := image.NewRGBA(image.Rect(0, 0, max(1, int(float64(w)*scale)), max(1, int(float64(h)*scale))))
		draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Src, nil)
		img = dst
		w, h = dst.Bounds().Dx(), dst.Bounds().Dy()
	}
	format := p.Format
	if format == "" {
		format = "jpeg"
	}
	var b bytes.Buffer
	var err error
	if format == "png" {
		err = png.Encode(&b, img)
	} else {
		quality := 80
		if p.Quality != nil {
			quality = max(1, int(*p.Quality*100))
		}
		err = jpeg.Encode(&b, img, &jpeg.Options{Quality: quality})
	}
	if err != nil || b.Len() > 5<<20 {
		return nil, errors.New("capture_unavailable")
	}
	return map[string]any{"format": format, "base64": base64.StdEncoding.EncodeToString(b.Bytes()), "width": w, "height": h, "screenIndex": 0, "capturedAtMs": time.Now().UnixMilli()}, nil
}
func registerNodeScreen(a *connection.NodeAccess) {
	_ = nodeRegister(a, connection.NodeOperation{Command: "screen.snapshot", Group: "screen", Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p nodeSnapshotParams
		if decodeNode(raw, &p) != nil {
			return nil, errors.New("invalid_params")
		}
		// Validate before attempting any hardware capture.
		if (p.ExecutionID != "" && !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(p.ExecutionID)) || p.ScreenIndex != 0 || p.MaxWidth < 0 || (p.Format != "" && p.Format != "png" && p.Format != "jpeg") || (p.Quality != nil && (*p.Quality < 0 || *p.Quality > 1)) {
			return nil, errors.New("invalid_params")
		}
		img, err := screencmd.Composed(display.Get())
		if err != nil {
			return nil, errors.New("capture_unavailable")
		}
		return encodeNodeSnapshot(img, p)
	}})
	_ = nodeRegister(a, connection.NodeOperation{Command: "screen.record", Group: "screen", Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p struct {
			DurationMS   int     `json:"durationMs"`
			ScreenIndex  int     `json:"screenIndex"`
			FPS          float64 `json:"fps"`
			Format       string  `json:"format"`
			IncludeAudio bool    `json:"includeAudio"`
		}
		if decodeNode(raw, &p) != nil || p.ScreenIndex != 0 || p.DurationMS < 0 || p.DurationMS > 10000 ||
			(p.Format != "" && p.Format != "mp4") {
			return nil, errors.New("invalid_params")
		}
		if p.IncludeAudio {
			return nil, errors.New("audio_recording_unsupported")
		}
		// The existing Android helper produces 15fps H.264. Dropping dependent
		// frames would corrupt the stream; never pretend to honor a different rate.
		if p.FPS != 0 && p.FPS != 15 {
			return nil, errors.New("unsupported_fps")
		}
		if p.DurationMS == 0 {
			p.DurationMS = 3000
		}
		select {
		case nodeCaptureGate <- struct{}{}:
			defer func() { <-nodeCaptureGate }()
		default:
			return nil, errors.New("busy")
		}
		d := display.Get()
		helper := d.Helper()
		if helper == nil {
			return nil, errors.New("capture_unavailable")
		}
		w, h := d.Native()
		stream, err := helper.Record(w, h, int(d.Orientation())/90)
		if err != nil {
			return nil, errors.New("capture_unavailable")
		}
		recordCtx, cancel := context.WithTimeout(ctx, time.Duration(p.DurationMS)*time.Millisecond)
		defer cancel()
		var once sync.Once
		closeStream := func() { once.Do(func() { _ = stream.Close() }) }
		stop := context.AfterFunc(recordCtx, closeStream)
		defer stop()
		defer closeStream()
		result, err := nodeRecordMP4(recordCtx, stream)
		if ctx.Err() != nil {
			return nil, errors.New("cancelled")
		}
		return result, err
	}})
}

type nodeRecording interface {
	Read() (surface.Encoded, error)
}

func nodeRecordMP4(ctx context.Context, stream nodeRecording) (any, error) {
	var sps, pps []byte
	var samples []*fmp4.Sample
	var stamps []time.Duration
	size := 0
	for {
		frame, err := stream.Read()
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			return nil, errors.New("capture_unavailable")
		}
		if ctx.Err() != nil {
			break
		}
		size += len(frame.Data)
		if size > 4<<20 {
			return nil, errors.New("result_too_large")
		}
		var nals h264.AnnexB
		if nals.Unmarshal(frame.Data) != nil {
			return nil, errors.New("capture_unavailable")
		}
		var au [][]byte
		for _, n := range nals {
			if len(n) == 0 {
				continue
			}
			switch n[0] & 31 {
			case 7:
				sps = append([]byte(nil), n...)
			case 8:
				pps = append([]byte(nil), n...)
			default:
				au = append(au, n)
			}
		}
		if frame.Flags&2 != 0 || len(au) == 0 {
			continue
		}
		sample, err := fmp4.NewSampleH264(0, au)
		if err != nil {
			return nil, errors.New("capture_unavailable")
		}
		// Begin only at a decodable keyframe.
		if len(samples) == 0 && sample.IsNonSyncSample {
			continue
		}
		samples = append(samples, sample)
		stamps = append(stamps, frame.At)
		if len(samples) > 600 {
			return nil, errors.New("result_too_large")
		}
	}
	if len(samples) == 0 || len(sps) == 0 || len(pps) == 0 {
		return nil, errors.New("capture_unavailable")
	}
	var duration uint64
	for i, s := range samples {
		delta := time.Second / 15
		if i+1 < len(stamps) {
			delta = stamps[i+1] - stamps[i]
		}
		if delta <= 0 || delta > time.Second {
			return nil, errors.New("capture_unavailable")
		}
		s.Duration = uint32(delta.Microseconds())
		duration += uint64(s.Duration)
	}
	var b seekablebuffer.Buffer
	init := fmp4.Init{Tracks: []*fmp4.InitTrack{{ID: 1, TimeScale: 1000000, Codec: &codecs.H264{SPS: sps, PPS: pps}}}}
	part := fmp4.Part{SequenceNumber: 1, Tracks: []*fmp4.PartTrack{{ID: 1, Samples: samples}}}
	if init.Marshal(&b) != nil || part.Marshal(&b) != nil || b.Len() > 5<<20 {
		return nil, errors.New("capture_unavailable")
	}
	return map[string]any{"format": "mp4", "base64": base64.StdEncoding.EncodeToString(b.Bytes()), "durationMs": duration / 1000, "fps": 15, "screenIndex": 0, "hasAudio": false}, nil
}
