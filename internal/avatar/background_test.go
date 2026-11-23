package avatar

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureBackground(t *testing.T, root, name string, img image.Image) string {
	t.Helper()
	p := filepath.Join(root, name)
	f, e := os.Create(p)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	if strings.HasSuffix(name, "jpg") {
		e = jpeg.Encode(f, img, nil)
	} else {
		e = png.Encode(f, img)
	}
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func TestBackgroundDecodeBudgetsAndSnapshotLifetime(t *testing.T) {
	root := t.TempDir()
	stage := t.TempDir()
	img := image.NewNRGBA(image.Rect(0, 0, 6, 4))
	img.SetNRGBA(0, 0, color.NRGBA{255, 0, 0, 128})
	fixtureBackground(t, root, "good.png", img)
	for i := 0; i < 5; i++ {
		p, release, e := ResolveBackground(root, stage, "/good.png")
		if e != nil {
			t.Fatal(e)
		}
		data, e := os.ReadFile(p)
		if e != nil {
			t.Fatal(e)
		}
		if string(data[:4]) != "BG01" || binary.LittleEndian.Uint32(data[4:8]) != 6 || binary.LittleEndian.Uint32(data[8:12]) != 4 || len(data) != 12+6*4*4 {
			t.Fatal("snapshot")
		}
		if !bytes.Equal(data[12:16], []byte{255, 0, 0, 128}) {
			t.Fatal("straight alpha", data[12:16])
		}
		if files, _ := os.ReadDir(stage); len(files) != 0 {
			t.Fatal("persistent snapshot")
		}
		release()
		if _, e := os.Open(p); e == nil {
			t.Fatal("descriptor leaked")
		}
	}
	fixtureBackground(t, root, "good.jpg", img)
	if _, e := readBackground(root, "/good.jpg"); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []string{"/absent.png", "/../good.png", "good.png", "/good.gif", "/good.png\\x", "/bad\n.png"} {
		if _, e := readBackground(root, bad); e == nil {
			t.Fatal("accepted", bad)
		}
	}
	os.WriteFile(filepath.Join(root, "bad.png"), []byte("not an image"), 0600)
	if _, e := readBackground(root, "/bad.png"); e == nil {
		t.Fatal("invalid accepted")
	}
	huge := image.NewGray(image.Rect(0, 0, 4097, 1))
	fixtureBackground(t, root, "wide.png", huge)
	if _, e := readBackground(root, "/wide.png"); e == nil {
		t.Fatal("dimension")
	}
	pixels := image.NewGray(image.Rect(0, 0, 2049, 2049))
	fixtureBackground(t, root, "pixels.png", pixels)
	if _, e := readBackground(root, "/pixels.png"); e == nil {
		t.Fatal("decoded budget")
	}
	f, _ := os.Create(filepath.Join(root, "bytes.png"))
	f.Truncate(MaxBackgroundBytes + 1)
	f.Close()
	if _, e := readBackground(root, "/bytes.png"); e == nil {
		t.Fatal("encoded budget")
	}
	fixtureBackground(t, root, "shrink.png", image.NewGray(image.Rect(0, 0, 4096, 2)))
	p, release, e := ResolveBackground(root, stage, "/shrink.png")
	if e != nil {
		t.Fatal(e)
	}
	defer release()
	data, _ := os.ReadFile(p)
	if binary.LittleEndian.Uint32(data[4:8]) != 2048 || binary.LittleEndian.Uint32(data[8:12]) != 1 {
		t.Fatal("texture budget")
	}
}
func TestBackgroundTranslations(t *testing.T) {
	for _, lang := range []string{"en", "de", "it", "vi"} {
		for _, key := range []string{"background", "background_help", "background_reset", "background_example", "fullscreen", "preview_only", "preview_exit", "background_invalid", "background_unavailable", "background_storage"} {
			if Text(lang, key) == "" {
				t.Fatal(lang, key)
			}
		}
	}
}
