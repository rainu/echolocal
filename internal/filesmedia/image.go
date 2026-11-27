// Package filesmedia provides bounded local previews, never filesystem discovery.
package filesmedia

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const MaxImageBytes = 16 << 20
const MaxImagePixels = 16 << 20

var ErrPreview = errors.New("preview_unavailable")

func Kind(name string) string {
	n := strings.ToLower(name)
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".gif", ".webp"} {
		if strings.HasSuffix(n, ext) {
			return "image"
		}
	}
	for _, ext := range []string{".mp4", ".webm", ".mkv"} {
		if strings.HasSuffix(n, ext) {
			return "video"
		}
	}
	for _, ext := range []string{".mp3", ".wav", ".flac", ".m4a", ".aac", ".ogg"} {
		if strings.HasSuffix(n, ext) {
			return "audio"
		}
	}
	return ""
}
func Fit(src image.Image, width, height int) *image.NRGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w > width {
		h = max(1, h*width/w)
		w = width
	}
	if h > height {
		w = max(1, w*height/h)
		h = height
	}
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, b, draw.Src, nil)
	return dst
}

var decodeGate = make(chan struct{}, 1)

func Image(f *os.File, width, height int) (*image.NRGBA, error) {
	return ImageContext(context.Background(), f, width, height)
}
func ImageContext(ctx context.Context, f *os.File, width, height int) (*image.NRGBA, error) {
	select {
	case decodeGate <- struct{}{}:
	case <-ctx.Done():
		return nil, ErrPreview
	}
	defer func() { <-decodeGate }()
	if ctx.Err() != nil {
		return nil, ErrPreview
	}

	st, e := f.Stat()
	if e != nil || st.Size() > MaxImageBytes || st.Size() < 1 {
		return nil, ErrPreview
	}
	encoded, e := io.ReadAll(io.LimitReader(f, MaxImageBytes+1))
	if e != nil || len(encoded) > MaxImageBytes {
		return nil, ErrPreview
	}
	cfg, _, e := image.DecodeConfig(bytes.NewReader(encoded))
	if e != nil || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > MaxImagePixels {
		return nil, ErrPreview
	}
	img, _, e := image.Decode(bytes.NewReader(encoded))
	if e != nil {
		return nil, ErrPreview
	}
	return Fit(img, width, height), nil
}

// Video uses the existing video-only, resource-limited native decoder. Only the
// pinned descriptor is inherited. No pathname, URL, network, audio or full copy.
func Video(ctx context.Context, f *os.File, helper string) (*image.NRGBA, error) {
	select {
	case decodeGate <- struct{}{}:
	case <-ctx.Done():
		return nil, ErrPreview
	}
	defer func() { <-decodeGate }()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, helper, "--thumbnail", "/proc/self/fd/3")
	cmd.ExtraFiles = []*os.File{f}
	pipe, e := cmd.StdoutPipe()
	if e != nil {
		return nil, ErrPreview
	}
	if e = cmd.Start(); e != nil {
		return nil, ErrPreview
	}
	r := bufio.NewReader(io.LimitReader(pipe, 1280*800*4+100))
	var w, h int
	_, e = fmt.Fscanf(r, "RGBA %d %d\n", &w, &h)
	var img *image.NRGBA
	if e == nil && w > 0 && w <= 1280 && h > 0 && h <= 800 {
		img = image.NewNRGBA(image.Rect(0, 0, w, h))
		_, e = io.ReadFull(r, img.Pix)
	} else {
		e = ErrPreview
	}
	if e != nil {
		cancel()
	}
	wait := cmd.Wait()
	if e != nil || wait != nil {
		return nil, ErrPreview
	}
	return Fit(img, 192, 192), nil
}
