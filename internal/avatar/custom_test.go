package avatar

import (
	"bytes"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCustomSnapshotPinsBytesAndCleansUp(t *testing.T) {
	root, dir := t.TempDir(), t.TempDir()
	original := glb(`{"extensions":{"VRMC_vrm":{"specVersion":"1.0","meta":{"name":"Custom","authors":["Artist"]}}}}`)
	source := filepath.Join(root, "custom.vrm")
	os.WriteFile(source, original, 0600)
	for i := 0; i < 20; i++ {
		a, p, m, release, err := ResolveCustom(root, dir, "/custom.vrm")
		if err != nil || a.Name != "Custom" || m != "" {
			t.Fatal(a, m, err)
		}
		os.WriteFile(source, []byte("replacement"), 0600)
		b, err := os.ReadFile(p)
		if err != nil || !bytes.Equal(b, original) {
			t.Fatal("snapshot changed", err)
		}
		entries, _ := os.ReadDir(dir)
		if len(entries) != 0 {
			t.Fatal("named staging file leaked", entries)
		}
		release()
		if _, err = os.Stat(p); !os.IsNotExist(err) {
			t.Fatal("snapshot descriptor leaked", err)
		}
		os.WriteFile(source, original, 0600)
	}
}
func TestCustomFailuresDoNotPublishOrFallback(t *testing.T) {
	root, dir := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(root, "invalid.vrm"), []byte("not a model"), 0600)
	os.WriteFile(filepath.Join(root, "external.vrm"), glb(`{"extensions":{"VRMC_vrm":{}},"images":[{"uri":"https://example.invalid/image"}]}`), 0600)
	os.WriteFile(filepath.Join(root, "unsupported.vrm"), glb(`{"extensions":{"VRMC_vrm":{"specVersion":"2.0"}}}`), 0600)
	os.Mkdir(filepath.Join(root, "folder.vrm"), 0700)
	unix.Mkfifo(filepath.Join(root, "pipe.vrm"), 0600)
	os.Symlink("/dev/null", filepath.Join(root, "outside.vrm"))
	f, _ := os.Create(filepath.Join(root, "large.vrm"))
	f.Truncate(MaxModelBytes)
	f.Close()
	for _, name := range []string{"missing", "invalid", "external", "unsupported", "folder", "pipe", "outside", "large"} {
		t.Run(name, func(t *testing.T) {
			_, p, m, release, err := ResolveCustom(root, dir, "/"+name+".vrm")
			release()
			if err == nil || p != "" || m != "" {
				t.Fatal("failure returned assets", p, m, err)
			}
			entries, _ := os.ReadDir(dir)
			if len(entries) > 0 {
				t.Fatal("leaked files")
			}
		})
	}
}
func TestCustomPathValidation(t *testing.T) {
	for _, s := range []string{"relative.vrm", "/a/../b.vrm", "/a/./b.vrm", "/bad\n.vrm", "/bad\x00.vrm", "/bad.glb", "/" + strings.Repeat("a", 252) + ".vrm"} {
		if ValidatePath(s) == nil {
			t.Errorf("accepted %q", s)
		}
	}
	for _, s := range []string{"", "/model.vrm", "/data/my avatar.VRM", "/data/模型.vrm"} {
		if err := ValidatePath(s); err != nil {
			t.Fatal(s, err)
		}
	}
}
func TestCustomMessagesCoverSupportedLanguages(t *testing.T) {
	for _, lang := range []string{"en", "de", "it", "vi"} {
		for _, key := range []string{"title", "path", "model", "default", "help", "invalid_path", "custom_unavailable", "custom_invalid", "custom_storage", "model_invalid", "starting", "selection"} {
			if Text(lang, key) == "" {
				t.Fatal(lang, key)
			}
		}
	}
}
