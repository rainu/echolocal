package avatar

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVideoSnapshotPinnedAndBackgroundOnly(t *testing.T) {
	root := t.TempDir()
	name := filepath.Join(root, "loop.MP4")
	data := []byte("\x00\x00\x00\x18ftypisom0123456789012345")
	if err := os.WriteFile(name, data, 0600); err != nil {
		t.Fatal(err)
	}
	old := probeBackgroundVideo
	defer func() { probeBackgroundVideo = old }()
	calls := 0
	probeBackgroundVideo = func(p string) error {
		calls++
		got, e := os.ReadFile(p)
		if e != nil || string(got) != string(data) {
			t.Fatal("probe snapshot")
		}
		return nil
	}
	if err := ValidateBackground(name); err != nil {
		t.Fatal(err)
	}
	p, release, err := ResolveBackground("/", filepath.Join(root, "staged"), name)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	os.WriteFile(name, []byte("changed source"), 0600)
	got, e := os.ReadFile(p)
	if e != nil || string(got) != string(data) {
		t.Fatal("source changed pinned bytes")
	}
	entries, _ := os.ReadDir(filepath.Join(root, "staged"))
	if len(entries) != 0 {
		t.Fatal("snapshot not unlinked")
	}
	if ValidateForeground(name) == nil {
		t.Fatal("video foreground accepted")
	}
	if _, done, err := ResolveForeground("/", root, name); err == nil {
		done()
		t.Fatal("video foreground resolved")
	}
	if calls != 2 {
		t.Fatal("unexpected probe", calls)
	}
	release()
	if _, err = os.ReadFile(p); err == nil {
		t.Fatal("descriptor retained")
	}
}
func TestVideoRejectsSignatureSizeAndProbe(t *testing.T) {
	root := t.TempDir()
	name := filepath.Join(root, "bad.mp4")
	old := probeBackgroundVideo
	defer func() { probeBackgroundVideo = old }()
	calls := 0
	probeBackgroundVideo = func(string) error { calls++; return errors.New("background_video_unsupported") }
	for _, data := range [][]byte{nil, []byte("not a video at all"), []byte("\x00\x00\x00\x18ftypisom0123456789012345")} {
		os.WriteFile(name, data, 0600)
		if ValidateBackground(name) == nil {
			t.Fatal("bad video accepted")
		}
	}
	if calls != 1 {
		t.Fatal("invalid header reached probe")
	}
	f, _ := os.Create(name)
	f.Truncate(MaxBackgroundVideoBytes + 1)
	f.Close()
	if ValidateBackground(name) == nil || calls != 1 {
		t.Fatal("oversize reached probe")
	}
	for _, name := range []string{"relative.mp4", "/a/../b.mp4", "/a/b.mp4\n"} {
		if ValidateBackgroundPath(name) == nil {
			t.Fatal("bad path")
		}
	}
	for _, lang := range []string{"en", "de", "it", "vi"} {
		if !strings.Contains(Text(lang, "background_help"), "MP4") {
			t.Fatal("missing help", lang)
		}
	}
}
