package avatar

import (
	"bytes"
	"context"
	"encoding/binary"
	"github.com/gen2brain/webp"
	"image"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func sceneFixture(t *testing.T, root string, lossless bool) []byte {
	t.Helper()
	frames := []image.Image{}
	for i := 0; i < 3; i++ {
		img := image.NewNRGBA(image.Rect(0, 0, 8, 4))
		for y := 0; y < 4; y++ {
			for x := 0; x < 8; x++ {
				a := uint8(0)
				if x >= 2 {
					a = 128
				}
				if x >= 5 {
					a = 255
				}
				img.SetNRGBA(x, y, color.NRGBA{uint8(220 - i*40), uint8(32 + i*40), 24, a})
			}
		}
		frames = append(frames, img)
	}
	var b bytes.Buffer
	if e := webp.EncodeAll(&b, &webp.WEBP{Image: frames, Delay: []int{80, 160, 240}, LoopCount: 2}, webp.Options{Lossless: lossless, Exact: true}); e != nil {
		t.Fatal(e)
	}
	data := b.Bytes()
	if e := os.WriteFile(filepath.Join(root, "loop.webp"), data, 0600); e != nil {
		t.Fatal(e)
	}
	return data
}
func TestAnimatedWebPAlphaTimingAndSnapshot(t *testing.T) {
	root, stage := t.TempDir(), t.TempDir()
	for _, lossless := range []bool{true, false} {
		data := sceneFixture(t, root, lossless)
		w, h, n, loops, delays, e := webPBudget(data)
		if e != nil || w != 8 || h != 4 || n != 3 || loops != 2 || delays[2] != 240 {
			t.Fatal(w, h, n, loops, delays, e)
		}
		scene, e := readScene(root, "/loop.webp")
		if e != nil {
			t.Fatal(e)
		}
		for i, img := range scene.frames {
			_, _, _, a := img.At(0, 0).RGBA()
			if a != 0 {
				t.Fatal("transparent", i, a)
			}
			r, g, _, a := img.At(3, 0).RGBA()
			if a < 127*257 || a > 129*257 || r == 0 || g == 0 {
				t.Fatal("partial alpha", i, r, g, a)
			}
		}
		p, release, e := ResolveForeground(root, stage, "/loop.webp")
		if e != nil {
			t.Fatal(e)
		}
		b, e := os.ReadFile(p)
		if e != nil {
			t.Fatal(e)
		}
		stride := 8*4*4 + 4
		if string(b[:4]) != "BG02" || len(b) != 24+3*stride || binary.LittleEndian.Uint32(b[16:20]) != 2 {
			t.Fatal("snapshot")
		}
		for i, d := range []uint32{80, 160, 240} {
			offset := 24 + i*stride
			if binary.LittleEndian.Uint32(b[offset:offset+4]) != d || b[offset+4+3] != 0 || b[offset+4+3*4+3] != 128 {
				t.Fatal("frame/alpha", i)
			}
			if lossless && (b[offset+4+3*4] < uint8(218-i*40) || b[offset+4+3*4] > uint8(222-i*40)) {
				t.Fatal("straight alpha lost", b[offset+4+3*4:offset+4+4*4])
			}
		}
		release()
		if _, e := os.Stat(p); e == nil {
			t.Fatal("descriptor leak")
		}
	}
	if entries, _ := os.ReadDir(stage); len(entries) != 0 {
		t.Fatal("persistent snapshots")
	}
}
func mutateChunk(data []byte, kind string, edit func([]byte)) []byte {
	b := append([]byte(nil), data...)
	for off := 12; off+8 <= len(b); {
		n := int(binary.LittleEndian.Uint32(b[off+4 : off+8]))
		if string(b[off:off+4]) == kind {
			edit(b[off+8 : off+8+n])
			return b
		}
		off += 8 + n + (n & 1)
	}
	panic("chunk not found")
}
func TestAnimatedBudgetsAndDamage(t *testing.T) {
	root := t.TempDir()
	data := sceneFixture(t, root, true)
	cases := map[string][]byte{
		"truncated": data[:len(data)-1],
		"canvas":    mutateChunk(data, "VP8X", func(b []byte) { b[4] = 0xff; b[5] = 0xff }),
		"fast":      mutateChunk(data, "ANMF", func(b []byte) { b[12] = 10; b[13] = 0; b[14] = 0 }),
		"long":      mutateChunk(data, "ANMF", func(b []byte) { b[12] = 0xff; b[13] = 0xff; b[14] = 0 }),
		"outside":   mutateChunk(data, "ANMF", func(b []byte) { b[0] = 100 }),
		"damage": mutateChunk(data, "ANMF", func(b []byte) {
			for i := 24; i < len(b); i++ {
				b[i] = 0xff
			}
		}),
	}
	for name, b := range cases {
		t.Run(name, func(t *testing.T) {
			os.WriteFile(filepath.Join(root, "bad.webp"), b, 0600)
			if _, e := readScene(root, "/bad.webp"); e == nil {
				t.Fatal("accepted")
			}
		})
	}
	// Budget inspection fails before any full decoder allocation.
	huge := mutateChunk(data, "VP8X", func(b []byte) { b[4] = 255; b[5] = 3; b[7] = 255; b[8] = 3 })
	frame := data[bytes.Index(data, []byte("ANMF")):]
	var many bytes.Buffer
	many.Write(huge[:bytes.Index(huge, []byte("ANMF"))])
	for i := 0; i < 20; i++ {
		many.Write(frame)
	}
	b := many.Bytes()
	binary.LittleEndian.PutUint32(b[4:8], uint32(len(b)-8))
	if _, _, _, _, _, e := webPBudget(b); e == nil {
		t.Fatal("decoded budget")
	}
	for _, v := range []string{"/x.webp", "/x.WEBP"} {
		if ValidateBackgroundPath(v) != nil {
			t.Fatal(v)
		}
	}
	if ValidateBackgroundPath("/../x.webp") == nil {
		t.Fatal("path")
	}
}
func TestAPNGExplicitlyUnsupported(t *testing.T) {
	root := t.TempDir()
	fixtureBackground(t, root, "plain.png", image.NewGray(image.Rect(0, 0, 2, 2)))
	b, _ := os.ReadFile(filepath.Join(root, "plain.png"))
	// acTL chunk before IDAT; no silent PNG first-frame fallback.
	ac := append([]byte{0, 0, 0, 8, 'a', 'c', 'T', 'L'}, make([]byte, 12)...)
	b = append(append(append([]byte(nil), b[:33]...), ac...), b[33:]...)
	os.WriteFile(filepath.Join(root, "animated.png"), b, 0600)
	if _, e := readScene(root, "/animated.png"); e == nil || e.Error() != "background_unsupported" {
		t.Fatal(e)
	}
	if e := foregroundError(errorsForTest("background_animation_limit")); !strings.HasPrefix(e.Error(), "foreground_") {
		t.Fatal(e)
	}
}
func errorsForTest(s string) error { return &sceneTestError{s} }

type sceneTestError struct{ s string }

func (e *sceneTestError) Error() string { return e.s }

func TestControllerAnimatedCompositionsAndCleanup(t *testing.T) {
	root := t.TempDir()
	sceneFixture(t, root, true)
	asset := filepath.Join(root, "loop.webp")
	c := New(t.TempDir(), nil, func() bool { return false }, func() {})
	c.resolve = fakeAssets
	c.SetBackground(func() string { return asset })
	c.EnableTalkLayout(func() string { return asset })
	c.SetForeground(func() string { return asset }, func() string { return asset })
	paths := []string{}
	c.command = func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		for _, flag := range []string{"--background", "--fullscreen-background", "--foreground", "--fullscreen-foreground"} {
			found := false
			for i, arg := range args {
				if arg == flag {
					p := args[i+1]
					data, e := os.ReadFile(p)
					if e != nil || string(data[:4]) != "BG02" || p == asset {
						t.Fatal(flag, e)
					}
					paths = append(paths, p)
					found = true
				}
			}
			if !found {
				t.Fatal("missing", flag)
			}
		}
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRendererHelper$")
		cmd.Env = []string{"VRM_TEST_HELPER=normal"}
		return cmd
	}
	c.Start()
	eventually(t, func() bool { return c.Snapshot().Ready })
	c.SetTalkMode(true)
	c.SetTalkMode(false)
	c.Stop()
	eventually(t, func() bool { return !c.Snapshot().Running })
	for _, p := range paths {
		if _, e := os.Stat(p); e == nil {
			t.Fatal("leaked", p)
		}
	}
}
func TestSceneErrorTranslations(t *testing.T) {
	for _, lang := range []string{"en", "de", "it", "vi"} {
		for _, key := range []string{"background_unsupported", "foreground_unsupported", "background_animation_limit", "foreground_animation_limit"} {
			v := Text(lang, key)
			if !strings.Contains(v, "WebP") || !strings.Contains(v, "APNG") {
				t.Fatal(lang, key, v)
			}
		}
	}
}
