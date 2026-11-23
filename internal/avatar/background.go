package avatar

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/ygelfand/echolocal/internal/devicefs"
	"image"
	_ "image/jpeg"
	_ "image/png"
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
	if len(v) > 255 || !utf8.ValidString(v) || !strings.HasPrefix(v, "/") || strings.ContainsAny(v, "\x00\\") || (ext != ".png" && ext != ".jpg" && ext != ".jpeg") {
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
	if name == "" || ValidateBackgroundPath(name) != nil {
		return nil, errors.New("background_invalid")
	}
	f, e := devicefs.OpenRegular(root, name)
	if e != nil {
		return nil, errors.New("background_unavailable")
	}
	defer f.Close()
	stat, e := f.Stat()
	if e != nil {
		return nil, errors.New("background_unavailable")
	}
	if stat.Size() <= 0 || stat.Size() > MaxBackgroundBytes {
		return nil, errors.New("background_invalid")
	}
	data, e := io.ReadAll(io.LimitReader(f, MaxBackgroundBytes+1))
	if e != nil || len(data) > MaxBackgroundBytes {
		return nil, errors.New("background_invalid")
	}
	cfg, kind, e := image.DecodeConfig(bytes.NewReader(data))
	if e != nil || (kind != "png" && kind != "jpeg") || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > MaxBackgroundDimension || cfg.Height > MaxBackgroundDimension || int64(cfg.Width)*int64(cfg.Height) > MaxBackgroundPixels {
		return nil, errors.New("background_invalid")
	}
	img, _, e := image.Decode(bytes.NewReader(data))
	if e != nil || img.Bounds().Dx() != cfg.Width || img.Bounds().Dy() != cfg.Height {
		return nil, errors.New("background_invalid")
	}
	return img, nil
}

func ValidateBackground(v string) error {
	if v == "" {
		return nil
	}
	_, e := readBackground("/", v)
	return e
}

// One immediately unlinked bounded RGBA snapshot per renderer lifetime.
func ResolveBackground(root, dir, name string) (string, func(), error) {
	img, e := readBackground(root, name)
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
	b := img.Bounds()
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
	header := []byte{'B', 'G', '0', '1', 0, 0, 0, 0, 0, 0, 0, 0}
	binary.LittleEndian.PutUint32(header[4:8], uint32(w))
	binary.LittleEndian.PutUint32(header[8:12], uint32(h))
	if _, e = f.Write(header); e != nil {
		return fail()
	}
	row := make([]byte, w*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			rr, gg, bb, aa := img.At(b.Min.X+x*b.Dx()/w, b.Min.Y+y*b.Dy()/h).RGBA()
			i := x * 4
			if aa != 0 {
				row[i] = byte(rr * 255 / aa)
				row[i+1] = byte(gg * 255 / aa)
				row[i+2] = byte(bb * 255 / aa)
			} else {
				row[i] = 0
				row[i+1] = 0
				row[i+2] = 0
			}
			row[i+3] = byte(aa >> 8)
		}
		if _, e = f.Write(row); e != nil {
			return fail()
		}
	}
	return fmt.Sprintf("/proc/%d/fd/%d", os.Getpid(), f.Fd()), cleanup, nil
}
