package avatar

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"path"
	"strings"
	"sync"

	"github.com/gen2brain/webp"
	"github.com/ygelfand/echolocal/internal/devicefs"
)

// Short decorative loops, not a video player. Bound total decoded allocation
// BEFORE invoking libwebp (including highly compressed, many-frame files).
const MaxSceneFrames = 120
const MaxSceneDecodedBytes = 32 << 20
const MaxSceneDimension = 1024
const MaxSceneDurationMS = 30000
const MinSceneDelayMS = 40

type layerScene struct {
	frames          []image.Image
	delays          []int
	loops, duration int
}

var sceneDecodeMu sync.Mutex

func readScene(root, name string) (scene layerScene, err error) {
	invalid := errors.New("background_invalid")
	if name == "" || ValidateBackgroundPath(name) != nil {
		return scene, invalid
	}
	f, e := devicefs.OpenRegular(root, name)
	if e != nil {
		return scene, errors.New("background_unavailable")
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return scene, errors.New("background_unavailable")
	}
	if st.Size() <= 0 || st.Size() > MaxBackgroundBytes {
		return scene, invalid
	}
	data, e := io.ReadAll(io.LimitReader(f, MaxBackgroundBytes+1))
	if e != nil || len(data) > MaxBackgroundBytes {
		return scene, invalid
	}
	// PNG's standard decoder silently ignores animation. Reject APNG explicitly
	// rather than presenting a frozen first frame as successful support.
	if strings.ToLower(path.Ext(name)) == ".apng" || pngAnimation(data) {
		return scene, errors.New("background_unsupported")
	}
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return decodeWebPScene(data)
	}
	cfg, kind, e := image.DecodeConfig(bytes.NewReader(data))
	if e != nil || (kind != "png" && kind != "jpeg") || cfg.Width <= 0 || cfg.Height <= 0 ||
		cfg.Width > MaxBackgroundDimension || cfg.Height > MaxBackgroundDimension ||
		int64(cfg.Width)*int64(cfg.Height) > MaxBackgroundPixels {
		return scene, invalid
	}
	img, _, e := image.Decode(bytes.NewReader(data))
	if e != nil || img.Bounds().Dx() != cfg.Width || img.Bounds().Dy() != cfg.Height {
		return scene, invalid
	}
	return layerScene{frames: []image.Image{img}}, nil
}

func pngAnimation(data []byte) bool {
	if len(data) < 8 || string(data[:8]) != "\x89PNG\r\n\x1a\n" {
		return false
	}
	for off := 8; off+12 <= len(data); {
		n := int64(binary.BigEndian.Uint32(data[off : off+4]))
		if string(data[off+4:off+8]) == "acTL" {
			return true
		}
		if n > int64(len(data)-off-12) {
			return false
		}
		off += 12 + int(n)
	}
	return false
}

func le24(b []byte) int { return int(b[0]) | int(b[1])<<8 | int(b[2])<<16 }

// Container budget inspection only; libwebp remains the format decoder and
// compositor (blend/disposal/subrectangles). No image bitstream decoder here.
func webPBudget(data []byte) (w, h, frames, loops int, delays []int, err error) {
	invalid := errors.New("background_invalid")
	limit := errors.New("background_animation_limit")
	err = invalid
	if len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" ||
		uint64(binary.LittleEndian.Uint32(data[4:8]))+8 != uint64(len(data)) {
		return
	}
	anim, extended, animationHeader, still := false, false, false, false
	duration := 0
	for off := 12; off < len(data); {
		if len(data)-off < 8 {
			return
		}
		n := int64(binary.LittleEndian.Uint32(data[off+4 : off+8]))
		if n > int64(len(data)-off-8) {
			return
		}
		end := off + 8 + int(n)
		b := data[off+8 : end]
		switch string(data[off : off+4]) {
		case "VP8X":
			if extended || off != 12 || len(b) != 10 {
				return
			}
			extended = true
			anim = b[0]&2 != 0
			w, h = le24(b[4:7])+1, le24(b[7:10])+1
		case "ANIM":
			if !anim || animationHeader || len(b) != 6 {
				return
			}
			animationHeader = true
			loops = int(binary.LittleEndian.Uint16(b[4:6]))
		case "ANMF":
			if !animationHeader || len(b) < 24 || still {
				return
			}
			x, y := le24(b[:3])*2, le24(b[3:6])*2
			fw, fh := le24(b[6:9])+1, le24(b[9:12])+1
			if x+fw > w || y+fh > h {
				return
			}
			d := le24(b[12:15])
			duration += d
			frames++
			if d < MinSceneDelayMS || d > 10000 || duration > MaxSceneDurationMS || frames > MaxSceneFrames {
				err = limit
				return
			}
			delays = append(delays, d)
		case "VP8 ", "VP8L":
			if anim || still {
				return
			}
			still = true
		}
		off = end + int(n&1)
		if off > len(data) {
			return
		}
	}
	if anim && (!animationHeader || frames < 1) {
		return
	}
	if !anim {
		if !still {
			return
		}
		cfg, e := webp.DecodeConfig(bytes.NewReader(data))
		if e != nil {
			return
		}
		w, h = cfg.Width, cfg.Height
		frames, loops, delays = 1, 1, []int{100}
	}
	if w <= 0 || h <= 0 {
		return
	}
	if w > MaxSceneDimension || h > MaxSceneDimension ||
		int64(w)*int64(h)*4*int64(frames) > MaxSceneDecodedBytes {
		err = limit
		return
	}
	err = nil
	return
}

func decodeWebPScene(data []byte) (scene layerScene, err error) {
	// The transpiled decoder owns private memory; serialize startup/validation
	// so concurrent API writes cannot multiply its bounded peak allocation.
	sceneDecodeMu.Lock()
	defer sceneDecodeMu.Unlock()
	defer func() {
		if recover() != nil {
			scene = layerScene{}
			err = errors.New("background_invalid")
		}
	}()
	w, h, n, loops, delays, e := webPBudget(data)
	if e != nil {
		return scene, e
	}
	decoded, e := webp.DecodeAll(bytes.NewReader(data))
	if e != nil || len(decoded.Image) != n {
		return scene, errors.New("background_invalid")
	}
	for i, img := range decoded.Image {
		if img.Bounds().Dx() != w || img.Bounds().Dy() != h {
			return scene, errors.New("background_invalid")
		}
		if n > 1 && (i >= len(decoded.Delay) || decoded.Delay[i] != delays[i]) {
			return scene, errors.New("background_invalid")
		}
	}
	return layerScene{frames: decoded.Image, delays: delays, loops: loops}, nil
}
