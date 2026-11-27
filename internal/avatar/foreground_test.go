package avatar

import (
	"context"
	"image"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestForegroundStraightAlphaAndLifetime(t *testing.T) {
	root := t.TempDir()
	stage := t.TempDir()
	img := image.NewNRGBA(image.Rect(0, 0, 3, 1))
	img.SetNRGBA(0, 0, color.NRGBA{255, 0, 0, 255})
	img.SetNRGBA(1, 0, color.NRGBA{0, 255, 0, 128})
	fixtureBackground(t, root, "front.png", img)
	p, release, err := ResolveForeground(root, stage, "/front.png")
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil || len(b) != 24 {
		t.Fatal(err, len(b))
	}
	want := []byte{255, 0, 0, 255, 0, 255, 0, 128, 0, 0, 0, 0}
	for i, v := range want {
		if b[12+i] != v {
			t.Fatal("alpha lost", b[12:])
		}
	}
	release()
	if _, err = os.ReadFile(p); err == nil {
		t.Fatal("descriptor leaked")
	}
	for _, bad := range []string{"/missing.png", "relative.png", "/../front.png"} {
		if _, close, err := ResolveForeground(root, stage, bad); err == nil || !strings.HasPrefix(err.Error(), "foreground_") {
			close()
			t.Fatal(bad, err)
		}
	}
	if err := ValidateForeground(""); err != nil {
		t.Fatal(err)
	}
}

func TestControllerStagesBothForegroundCompositions(t *testing.T) {
	root := t.TempDir()
	img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	img.SetNRGBA(1, 1, color.NRGBA{20, 40, 60, 128})
	asset := fixtureBackground(t, root, "front.png", img)
	c := New(t.TempDir(), nil, func() bool { return false }, func() {})
	c.resolve = fakeAssets
	c.SetBackground(func() string { return asset })
	c.EnableTalkLayout(func() string { return asset })
	c.SetForeground(func() string { return asset }, func() string { return asset })
	c.SetTalkMode(true)
	paths := []string{}
	c.command = func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		for _, flag := range []string{"--background", "--fullscreen-background", "--foreground", "--fullscreen-foreground"} {
			found := false
			for i, arg := range args {
				if arg == flag {
					found = true
					p := args[i+1]
					if p == asset {
						t.Fatal("unvalidated caller path forwarded")
					}
					data, err := os.ReadFile(p)
					if err != nil || string(data[:4]) != "BG01" {
						t.Fatal(flag, err)
					}
					paths = append(paths, p)
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
	defer c.Stop()
	eventually(t, func() bool { return c.Snapshot().Ready })
	c.SetTalkMode(false)
	c.SetTalkMode(true)
	c.Stop()
	eventually(t, func() bool { return !c.Snapshot().Running })
	for _, p := range paths {
		if _, err := os.ReadFile(p); err == nil {
			t.Fatal("leaked snapshot")
		}
	}
	if files, _ := filepath.Glob(filepath.Join(c.dir, ".background-*")); len(files) != 0 {
		t.Fatal(files)
	}
}

func TestForegroundFailureLeavesSceneRunning(t *testing.T) {
	c := New(t.TempDir(), nil, func() bool { return false }, func() {})
	c.resolve = fakeAssets
	c.SetForeground(func() string { return "/absent.png" }, nil)
	c.command = func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		for _, arg := range args {
			if arg == "--foreground" {
				t.Fatal("invalid image forwarded")
			}
		}
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRendererHelper$")
		cmd.Env = []string{"VRM_TEST_HELPER=normal"}
		return cmd
	}
	c.Start()
	defer c.Stop()
	eventually(t, func() bool { return c.Snapshot().Ready })
	if s := c.Snapshot(); s.ForegroundNotice != "foreground_unavailable" || s.BackgroundNotice != "" {
		t.Fatal(s)
	}
	c.stats(c.generation, "AVATAR_FOREGROUND_ERROR")
	if c.Snapshot().ForegroundNotice != "foreground_invalid" {
		t.Fatal(c.Snapshot())
	}
}

func TestForegroundTranslations(t *testing.T) {
	for _, lang := range []string{"en", "de", "it", "vi"} {
		for _, key := range []string{"foreground", "fullscreen_foreground", "foreground_help", "foreground_reset", "foreground_example", "foreground_invalid", "foreground_unavailable", "foreground_storage"} {
			if Text(lang, key) == "" {
				t.Fatal(lang, key)
			}
		}
	}
}
