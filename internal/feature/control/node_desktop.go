//go:build !board_biscuit

package control

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"image"
	"io"
	"os/exec"
	"time"

	featureclaw "github.com/ygelfand/echolocal/internal/feature/openclaw"
	"github.com/ygelfand/echolocal/internal/hardware/display"
	connection "github.com/ygelfand/echolocal/internal/openclaw"
)

// screencap uses SurfaceFlinger composition but owns no persistent surface or
// encoder. Its process is private to this capture and forcibly cancellable,
// unlike the shared UI helper's synchronous ScreenRead call.
type desktopCaptureBuffer struct{ bytes.Buffer }

func (b *desktopCaptureBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 8*1024*1024+16 {
		return 0, errors.New("capture_limit")
	}
	return b.Buffer.Write(p)
}
func captureDesktop(ctx context.Context) (image.Image, error) {
	cmd := exec.CommandContext(ctx, "/system/bin/screencap")
	cmd.WaitDelay = 200 * time.Millisecond
	var out desktopCaptureBuffer
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	if cmd.Run() != nil || ctx.Err() != nil {
		return nil, errors.New("capture_unavailable")
	}
	return decodeDesktopCapture(out.Bytes())
}
func decodeDesktopCapture(b []byte) (image.Image, error) {
	bad := errors.New("capture_unavailable")
	if len(b) < 12 {
		return nil, bad
	}
	w, h, format := int(binary.LittleEndian.Uint32(b)), int(binary.LittleEndian.Uint32(b[4:])), binary.LittleEndian.Uint32(b[8:])
	if w < 1 || h < 1 || w > 2048 || h > 2048 || w*h > 2*1024*1024 || (format != 1 && format != 2) {
		return nil, bad
	}
	offset := len(b) - w*h*4
	// Android versions use either width/height/format or that plus dataspace.
	if offset != 12 && offset != 16 {
		return nil, bad
	}
	return &image.RGBA{Pix: b[offset:], Stride: w * 4, Rect: image.Rect(0, 0, w, h)}, nil
}
func registerNodeDesktop(a *connection.NodeAccess) {
	// Transport-only command: bearer parameters must not appear as plugin tools.
	_ = a.Register(connection.NodeOperation{Command: "desktop.stream", Group: "desktop", Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
		select {
		case nodeCaptureGate <- struct{}{}:
			defer func() { <-nodeCaptureGate }()
		default:
			return nil, errors.New("busy")
		}
		w, h := display.Get().Size()
		if longest := max(w, h); longest > 640 {
			w = max(1, w*640/longest)
			h = max(1, h*640/longest)
		}
		input := nodeDesktopInput(ctx, a, w, h)
		defer input.Close()
		return featureclaw.Get().Node.StreamDesktop(ctx, raw, w, h, captureDesktop, input)
	}})
}
