package filesmedia

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestImageBoundsDecodeAndFailure(t *testing.T) {
	f, e := os.CreateTemp(t.TempDir(), "*.png")
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	img := image.NewNRGBA(image.Rect(0, 0, 400, 200))
	img.Set(1, 1, color.White)
	png.Encode(f, img)
	f.Seek(0, 0)
	small, e := Image(f, 192, 192)
	if e != nil || small.Bounds().Dx() != 192 || small.Bounds().Dy() != 96 {
		t.Fatal("aspect", e)
	}
	f.Truncate(0)
	f.Seek(0, 0)
	f.WriteString("broken image")
	f.Seek(0, 0)
	if _, e = Image(f, 192, 192); e == nil {
		t.Fatal("invalid accepted")
	}
	f.Truncate(MaxImageBytes + 1)
	f.Seek(0, 0)
	if _, e = Image(f, 192, 192); e == nil {
		t.Fatal("oversize accepted")
	}
}
func TestPinnedTransportRangeAndNoNetworkFallback(t *testing.T) {
	name := filepath.Join(t.TempDir(), "sample.mp4")
	os.WriteFile(name, []byte("0123456789"), 0600)
	f, _ := os.Open(name)
	tr, e := NewTransport(f, name)
	if e != nil {
		t.Fatal(e)
	}
	defer tr.Close()
	os.Remove(name)
	os.WriteFile(name, []byte("private replacement"), 0600)
	for raw, want := range map[string]string{"bytes=2-4": "234", "bytes=5-": "56789", "": "0123456789"} {
		r, _ := http.NewRequest("GET", URL, nil)
		r.Header.Set("Range", raw)
		res, e := tr.RoundTrip(r)
		if e != nil {
			t.Fatal(e)
		}
		data, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if string(data) != want {
			t.Fatal(raw, string(data))
		}
	}
	for _, url := range []string{"http://example.com/file", "file:///etc/passwd", URL + "?path=/etc/passwd", URL + "/../secret"} {
		r, _ := http.NewRequest("GET", url, nil)
		if _, e := tr.RoundTrip(r); e == nil {
			t.Fatal("fallback", url)
		}
	}
	for _, raw := range []string{"bytes=-1", "bytes=1-2,4-5", "bytes=2-bad"} {
		r, _ := http.NewRequest("GET", URL, nil)
		r.Header.Set("Range", raw)
		if _, e := tr.RoundTrip(r); e == nil {
			t.Fatal("bad range", raw)
		}
	}
	tr.Close()
	r, _ := http.NewRequest("GET", URL, nil)
	if _, e = tr.RoundTrip(r); e == nil {
		t.Fatal("closed")
	}
}
func TestCacheBoundNegativeFallbackAndCancellation(t *testing.T) {
	c := NewCache(nil)
	defer c.Close()
	for i := 0; i < 300; i++ {
		c.Get(strings.Repeat("x", i+1), "bad.png", "", func() (*os.File, error) { return nil, ErrPreview })
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		pending := len(c.pending)
		count := len(c.items)
		c.mu.Unlock()
		if pending == 0 {
			if count > 64 {
				t.Fatal("cache bound", count)
			}
			break
		}
		time.Sleep(time.Millisecond)
	}
	c.Close()
	if img := c.Get("after", "bad.png", "", nil); img != nil {
		t.Fatal("closed cache")
	}
	f, _ := os.CreateTemp(t.TempDir(), "video")
	defer f.Close()
	if _, e := Video(context.Background(), f, "/missing-decoder"); e == nil {
		t.Fatal("missing helper")
	}
}

// This exact test binary also runs on the real device with the installed helper.
func TestActualVideoThumbnail(t *testing.T) {
	name, helper := os.Getenv("FILES_TEST_VIDEO"), os.Getenv("FILES_TEST_HELPER")
	if name == "" || helper == "" {
		t.Skip("real native helper fixture not supplied")
	}
	f, e := os.Open(name)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	img, e := Video(context.Background(), f, helper)
	if e != nil || img == nil {
		t.Fatal("native first-frame preview", e)
	}
	if img.Rect.Dx() > 192 || img.Rect.Dy() > 192 || img.Rect.Dx() < 1 || img.Rect.Dy() < 1 {
		t.Fatal("thumbnail bounds")
	}
	low, high := 255, 0
	for _, v := range img.Pix {
		low = min(low, int(v))
		high = max(high, int(v))
	}
	if high-low < 32 {
		t.Fatal("empty native picture")
	}
	fmt.Printf("native thumbnail %dx%d; bounded local decoder PASS\n", img.Rect.Dx(), img.Rect.Dy())
}
