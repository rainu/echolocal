package avatar

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"path"
	"strings"
	"unicode/utf8"
)

// Encoded and decoded limits are checked before decoding or allocating textures.
const MaxBackgroundBytes = 8 << 20
const MaxBackgroundPixels = 4 << 20
const MaxBackgroundDimension = 4096
const BackgroundTextureDimension = 2048

func ValidateBackgroundPath(v string) error {
	if v == "" {
		return nil
	}
	ext := strings.ToLower(path.Ext(v))
	if len(v) > 255 || !utf8.ValidString(v) || !strings.HasPrefix(v, "/") || strings.ContainsAny(v, "\x00\\") || (ext != ".png" && ext != ".jpg" && ext != ".jpeg" && ext != ".webp" && ext != ".apng") {
		return errors.New("background_invalid")
	}
	for _, part := range strings.Split(v, "/") {
		if part == "." || part == ".." {
			return errors.New("background_invalid")
		}
		for _, c := range part {
			if c < 32 || c == 127 {
				return errors.New("background_invalid")
			}
		}
	}
	return nil
}

func readBackground(root, name string) (image.Image, error) {
	scene, e := readScene(root, name)
	if e != nil {
		return nil, e
	}
	return scene.frames[0], nil
}

func ValidateBackground(v string) error {
	if v == "" {
		return nil
	}
	_, e := readScene("/", v)
	return e
}

// One immediately unlinked bounded RGBA snapshot per renderer lifetime.
func ResolveBackground(root, dir, name string) (string, func(), error) {
	scene, e := readScene(root, name)
	if e != nil {
		return "", func() {}, e
	}
	if e = os.MkdirAll(dir, 0700); e != nil {
		return "", func() {}, errors.New("background_storage")
	}
	f, e := os.CreateTemp(dir, ".background-*")
	if e != nil {
		return "", func() {}, errors.New("background_storage")
	}
	cleanup := func() { _ = f.Close() }
	fail := func() (string, func(), error) { cleanup(); return "", func() {}, errors.New("background_storage") }
	if e = os.Remove(f.Name()); e != nil {
		cleanup()
		_ = os.Remove(f.Name())
		return "", func() {}, errors.New("background_storage")
	}
	b := scene.frames[0].Bounds()
	w, h := b.Dx(), b.Dy()
	if max(w, h) > BackgroundTextureDimension {
		if w >= h {
			h = max(1, h*BackgroundTextureDimension/w)
			w = BackgroundTextureDimension
		} else {
			w = max(1, w*BackgroundTextureDimension/h)
			h = BackgroundTextureDimension
		}
	}
	header := make([]byte, 12)
	copy(header, "BG01")
	binary.LittleEndian.PutUint32(header[4:8], uint32(w))
	binary.LittleEndian.PutUint32(header[8:12], uint32(h))
	if len(scene.frames) > 1 {
		copy(header, "BG02")
		header = append(header, make([]byte, 12)...)
		binary.LittleEndian.PutUint32(header[12:16], uint32(len(scene.frames)))
		binary.LittleEndian.PutUint32(header[16:20], uint32(scene.loops))
		for _, d := range scene.delays {
			scene.duration += d
		}
		binary.LittleEndian.PutUint32(header[20:24], uint32(scene.duration))
	}
	if _, e = f.Write(header); e != nil {
		return fail()
	}
	for i, img := range scene.frames {
		if len(scene.frames) > 1 {
			var delay [4]byte
			binary.LittleEndian.PutUint32(delay[:], uint32(scene.delays[i]))
			if _, e = f.Write(delay[:]); e != nil {
				return fail()
			}
		}
		if e = writeLayerPixels(f, img, w, h); e != nil {
			return fail()
		}
	}
	return fmt.Sprintf("/proc/%d/fd/%d", os.Getpid(), f.Fd()), cleanup, nil
}

// writeLayerPixels preserves straight alpha; the foreground GPU path premultiplies once.
func writeLayerPixels(f io.Writer, img image.Image, w, h int) error {
	b := img.Bounds()
	row := make([]byte, w*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			rr, gg, bb, aa := img.At(b.Min.X+x*b.Dx()/w, b.Min.Y+y*b.Dy()/h).RGBA()
			i := x * 4
			row[i], row[i+1], row[i+2] = 0, 0, 0
			if aa != 0 {
				row[i], row[i+1], row[i+2] = byte(rr*255/aa), byte(gg*255/aa), byte(bb*255/aa)
			}
			row[i+3] = byte(aa >> 8)
		}
		if _, e := f.Write(row); e != nil {
			return e
		}
	}
	return nil
}
