package openclaw

import (
	"bytes"
	"context"
	"crypto/des"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"image"
	"image/jpeg"
	"io"
	"math/bits"
	"time"

	"golang.org/x/image/draw"
)

type desktopWire interface {
	io.Reader
	io.Writer
}
type rfbEvent struct {
	kind byte
	data []byte
}

var rfbFormat = []byte{32, 24, 0, 1, 0, 255, 0, 255, 0, 255, 16, 8, 0, 0, 0, 0}

func vncResponse(password string, challenge []byte) []byte {
	key := make([]byte, 8)
	copy(key, password)
	for i := range key {
		key[i] = bits.Reverse8(key[i])
	}
	block, _ := des.NewCipher(key)
	out := make([]byte, 16)
	block.Encrypt(out[:8], challenge[:8])
	block.Encrypt(out[8:], challenge[8:])
	return out
}
func rfbRead(r io.Reader, n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := io.ReadFull(r, b)
	return b, err
}
func rfbHandshake(p desktopWire, password string, w, h int) error {
	if _, err := p.Write([]byte("RFB 003.008\n")); err != nil {
		return err
	}
	b, err := rfbRead(p, 12)
	if err != nil {
		return err
	}
	if string(b) != "RFB 003.008\n" {
		return errors.New("rfb_version")
	}
	if _, err = p.Write([]byte{1, 2}); err != nil {
		return err
	}
	b, err = rfbRead(p, 1)
	if err != nil {
		return err
	}
	if b[0] != 2 {
		return errors.New("rfb_auth")
	}
	challenge := make([]byte, 16)
	if _, err = rand.Read(challenge); err != nil {
		return err
	}
	if _, err = p.Write(challenge); err != nil {
		return err
	}
	b, err = rfbRead(p, 16)
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare(b, vncResponse(password, challenge)) != 1 {
		p.Write([]byte{0, 0, 0, 1, 0, 0, 0, 0})
		return errors.New("rfb_auth")
	}
	if _, err = p.Write([]byte{0, 0, 0, 0}); err != nil {
		return err
	}
	if _, err = rfbRead(p, 1); err != nil {
		return err
	}
	header := make([]byte, 24)
	binary.BigEndian.PutUint16(header, uint16(w))
	binary.BigEndian.PutUint16(header[2:], uint16(h))
	copy(header[4:], rfbFormat)
	name := []byte("EchoLocal (view only)")
	binary.BigEndian.PutUint32(header[20:], uint32(len(name)))
	_, err = p.Write(append(header, name...))
	return err
}
func readRFBEvents(ctx context.Context, p desktopWire, out chan<- rfbEvent) error {
	send := func(e rfbEvent) error {
		select {
		case out <- e:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	for {
		kind, err := rfbRead(p, 1)
		if err != nil {
			return err
		}
		var b []byte
		switch kind[0] {
		case 0:
			b, err = rfbRead(p, 19)
		case 2:
			b, err = rfbRead(p, 3)
			if err != nil {
				return err
			}
			n := int(binary.BigEndian.Uint16(b[1:]))
			if n > 256 {
				return errors.New("rfb_encodings")
			}
			b, err = rfbRead(p, n*4)
		case 3:
			b, err = rfbRead(p, 9)
		case 4:
			_, err = rfbRead(p, 7) // Never inject keys.
		case 5:
			_, err = rfbRead(p, 5) // Never inject pointer events.
		case 6:
			b, err = rfbRead(p, 7)
			if err != nil {
				return err
			}
			n := binary.BigEndian.Uint32(b[3:])
			if n > 65536 {
				return errors.New("rfb_clipboard")
			}
			_, err = io.CopyN(io.Discard, p, int64(n)) // Never read/write local clipboard.
		default:
			return errors.New("rfb_message")
		}
		if err != nil {
			return err
		}
		if kind[0] == 0 || kind[0] == 2 || kind[0] == 3 {
			if err = send(rfbEvent{kind[0], b}); err != nil {
				return err
			}
		}
	}
}

func validRFBFormat(f []byte) bool {
	if len(f) != 16 || (f[0] != 8 && f[0] != 16 && f[0] != 32) || f[1] == 0 || f[1] > f[0] || f[2] > 1 || f[3] != 1 {
		return false
	}
	var used uint64
	for i := 0; i < 3; i++ {
		maximum := uint64(binary.BigEndian.Uint16(f[4+i*2:]))
		shift := f[10+i]
		if maximum == 0 || maximum&(maximum+1) != 0 || shift >= f[0] {
			return false
		}
		mask := maximum << shift
		if mask>>f[0] != 0 || used&mask != 0 {
			return false
		}
		used |= mask
	}
	return true
}
func rfbRaw(img *image.RGBA, f []byte) []byte {
	bpp := int(f[0]) / 8
	out := make([]byte, img.Bounds().Dx()*img.Bounds().Dy()*bpp)
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			off := img.PixOffset(x, y)
			var v uint32
			for c := 0; c < 3; c++ {
				v |= uint32(uint64(img.Pix[off+c])*uint64(binary.BigEndian.Uint16(f[4+c*2:]))/255) << f[10+c]
			}
			at := (y*img.Bounds().Dx() + x) * bpp
			for j := 0; j < bpp; j++ {
				k := j
				if f[2] == 1 {
					k = bpp - 1 - j
				}
				out[at+k] = byte(v >> uint(j*8))
			}
		}
	}
	return out
}
func compactLength(n int) []byte {
	b := []byte{byte(n & 127)}
	n >>= 7
	if n > 0 {
		b[0] |= 128
		b = append(b, byte(n&127))
		n >>= 7
		if n > 0 {
			b[1] |= 128
			b = append(b, byte(n))
		}
	}
	return b
}
func serveDesktop(ctx context.Context, p desktopWire, password string, w, h int, capture func(context.Context) (image.Image, error), frame func()) error {
	if err := rfbHandshake(p, password, w, h); err != nil {
		return err
	}
	if ready, ok := p.(interface{ ready() }); ok {
		ready.ready()
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	events := make(chan rfbEvent, 8)
	failed := make(chan error, 1)
	go func() { failed <- readRFBEvents(ctx, p, events) }()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	idle := time.NewTimer(45 * time.Second)
	defer idle.Stop()
	format := append([]byte(nil), rfbFormat...)
	tight, pending, force := false, false, false
	var previous [32]byte
	lastUpdate := time.Time{}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-failed:
			return err
		case <-idle.C:
			return errors.New("rfb_idle")
		case e := <-events:
			idle.Reset(45 * time.Second)
			switch e.kind {
			case 0:
				if !validRFBFormat(e.data[3:]) {
					return errors.New("rfb_pixel_format")
				}
				format = append(format[:0], e.data[3:]...)
				force = true
			case 2:
				tight = false
				for i := 0; i < len(e.data); i += 4 {
					if binary.BigEndian.Uint32(e.data[i:]) == 7 {
						tight = true
					}
				}
			case 3:
				x, y := int(binary.BigEndian.Uint16(e.data[1:])), int(binary.BigEndian.Uint16(e.data[3:]))
				rw, rh := int(binary.BigEndian.Uint16(e.data[5:])), int(binary.BigEndian.Uint16(e.data[7:]))
				if x > w || y > h || rw > w-x || rh > h-y {
					return errors.New("rfb_rectangle")
				}
				pending = true
				force = force || e.data[0] == 0
			}
		case <-ticker.C:
			if !pending {
				continue
			}
			shotCtx, done := context.WithTimeout(ctx, 2*time.Second)
			src, err := capture(shotCtx)
			done()
			if err != nil {
				return errors.New("capture_unavailable")
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			img := image.NewRGBA(image.Rect(0, 0, w, h))
			draw.ApproxBiLinear.Scale(img, img.Bounds(), src, src.Bounds(), draw.Src, nil)
			hash := sha256.Sum256(img.Pix)
			if !force && hash == previous {
				// A held incremental request is not an idle viewer. Empty updates keep
				// the standard request/response loop alive even on a static display.
				if time.Since(lastUpdate) >= time.Second {
					if _, err = p.Write([]byte{0, 0, 0, 0}); err != nil {
						return err
					}
					pending = false
					lastUpdate = time.Now()
				}
				continue
			}
			header := make([]byte, 16)
			header[3] = 1
			binary.BigEndian.PutUint16(header[8:], uint16(w))
			binary.BigEndian.PutUint16(header[10:], uint16(h))
			var payload []byte
			if tight && format[0] == 32 && format[1] == 24 {
				var b bytes.Buffer
				if err = jpeg.Encode(&b, img, &jpeg.Options{Quality: 75}); err != nil {
					return err
				}
				if b.Len() > 2*1024*1024 {
					return errors.New("frame_limit")
				}
				binary.BigEndian.PutUint32(header[12:], 7)
				payload = append([]byte{0x90}, compactLength(b.Len())...)
				payload = append(payload, b.Bytes()...)
			} else {
				payload = rfbRaw(img, format)
			}
			if _, err = p.Write(header); err != nil {
				return err
			}
			if _, err = p.Write(payload); err != nil {
				return err
			}
			lastUpdate = time.Now()
			previous = hash
			pending = false
			force = false
			frame()
		}
	}
}
