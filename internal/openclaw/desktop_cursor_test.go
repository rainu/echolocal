package openclaw

import (
	"context"
	"encoding/binary"
	"image"
	"net"
	"testing"
	"time"
)

func TestRFBCursorNegotiation(t *testing.T) {
	for _, offer := range []bool{false, true} {
		name := "legacy"
		if offer {
			name = "rich-cursor"
		}
		t.Run(name, func(t *testing.T) {
			server, client := net.Pipe()
			defer client.Close()
			client.SetDeadline(time.Now().Add(5 * time.Second))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				defer server.Close()
				done <- serveDesktop(ctx, server, "12345678", 4, 2, func(context.Context) (image.Image, error) {
					return image.NewRGBA(image.Rect(0, 0, 4, 2)), nil
				}, func() {})
			}()
			clientRFBHandshake(t, client, "12345678", 4, 2)
			if offer {
				client.Write([]byte{2, 0, 0, 2, 0, 0, 0, 0, 255, 255, 255, 17})
			}
			read := func() {
				t.Helper()
				client.Write(updateRequest())
				header, err := rfbRead(client, 16)
				if err != nil {
					t.Fatal(err)
				}
				want := uint16(1)
				if offer {
					want = 2
				}
				if binary.BigEndian.Uint16(header[2:]) != want || binary.BigEndian.Uint32(header[12:]) != 0 {
					t.Fatal("frame/cursor rectangle negotiation")
				}
				if _, err = rfbRead(client, 4*2*4); err != nil {
					t.Fatal(err)
				}
				if !offer {
					return
				}
				cursor, err := rfbRead(client, 12)
				if err != nil {
					t.Fatal(err)
				}
				if binary.BigEndian.Uint16(cursor) != 0 || binary.BigEndian.Uint16(cursor[2:]) != 0 ||
					binary.BigEndian.Uint16(cursor[4:]) != 16 || binary.BigEndian.Uint16(cursor[6:]) != 18 ||
					int32(binary.BigEndian.Uint32(cursor[8:])) != -239 {
					t.Fatal("RichCursor rectangle/hotspot")
				}
				pixels, err := rfbRead(client, 16*18*4)
				if err != nil {
					t.Fatal(err)
				}
				// White center in the arrow; black outline. Both must remain visible on
				// opposite backgrounds, rather than advertising an invisible cursor.
				white := (2*16 + 1) * 4
				if pixels[white] != 255 || pixels[white+1] != 255 || pixels[white+2] != 255 ||
					pixels[0] != 0 || pixels[1] != 0 || pixels[2] != 0 {
					t.Fatal("cursor contrast")
				}
				mask, err := rfbRead(client, 2*18)
				if err != nil {
					t.Fatal(err)
				}
				if mask[0] != 0x80 || mask[1] != 0 || mask[2] != 0xc0 || mask[4] != 0xe0 {
					t.Fatal("cursor alpha mask must be MSB-first, padded per row")
				}
			}
			read()
			if offer {
				// A new encoding offer after a prior frame must resend the cursor even on
				// a static screen; it must not be swallowed by unchanged-frame suppression.
				client.Write([]byte{2, 0, 0, 1, 255, 255, 255, 17})
				read()
			}
			client.Close()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("cursor stream did not close")
			}
		})
	}
}

func TestRFBCursorPixelLayout(t *testing.T) {
	for _, tc := range []struct {
		name   string
		format []byte
		white  []byte
	}{
		{"rgb32-le", rfbFormat, []byte{255, 255, 255, 0}},
		{"rgb565-be", []byte{16, 16, 1, 1, 0, 31, 0, 63, 0, 31, 11, 5, 0, 0, 0, 0}, []byte{255, 255}},
		{"rgb332", []byte{8, 8, 0, 1, 0, 7, 0, 7, 0, 3, 5, 2, 0, 0, 0, 0}, []byte{255}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := rfbCursor(tc.format)
			bpp := int(tc.format[0]) / 8
			if len(b) != 12+16*18*bpp+2*18 {
				t.Fatal("cursor wire size")
			}
			offset := 12 + (2*16+1)*bpp
			for i, w := range tc.white {
				if b[offset+i] != w {
					t.Fatal("cursor did not honor negotiated pixel layout")
				}
			}
		})
	}
}
