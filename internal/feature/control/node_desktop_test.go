//go:build !board_biscuit

package control

import (
	"encoding/binary"
	"testing"
)

func TestDesktopCaptureHeadersAndBounds(t *testing.T) {
	for _, header := range []int{12, 16} {
		b := make([]byte, header+8)
		binary.LittleEndian.PutUint32(b, 2)
		binary.LittleEndian.PutUint32(b[4:], 1)
		binary.LittleEndian.PutUint32(b[8:], 1)
		b[header] = 23
		img, err := decodeDesktopCapture(b)
		if err != nil {
			t.Fatal(err)
		}
		r, _, _, _ := img.At(0, 0).RGBA()
		if r>>8 != 23 {
			t.Fatal("wrong header offset")
		}
	}
	for _, b := range [][]byte{nil, make([]byte, 16)} {
		if _, err := decodeDesktopCapture(b); err == nil {
			t.Fatal("invalid capture accepted")
		}
	}
}
