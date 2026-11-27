package filepicker

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFilesMetadataSortHiddenAndPrivatePolicy(t *testing.T) {
	root := t.TempDir()
	for name, data := range map[string]string{"large.png": "12345", "small.txt": "1", ".visible.txt": "2", ".env": "secret", "id_rsa": "secret", "identity.pem": "secret"} {
		write(t, filepath.Join(root, name))
		os.WriteFile(filepath.Join(root, name), []byte(data), 0600)
	}
	os.Mkdir(filepath.Join(root, ".ssh"), 0700)
	os.Mkdir(filepath.Join(root, "folder"), 0700)
	now := time.Now().Add(-time.Hour)
	os.Chtimes(filepath.Join(root, "large.png"), now, now)
	b := New(Options{Locations: []Location{{"root", root}}, ShowHidden: true, Metadata: true, Sort: "size", Descending: true})
	requireNoError(t, b.Navigate(root))
	if len(b.Entries) != 4 || b.Entries[0].Name != "folder" || b.Entries[1].Name != "large.png" || b.Entries[1].Size != 5 {
		t.Fatalf("metadata/order: %+v", b.Entries)
	}
	for _, key := range []string{"name", "modified", "type"} {
		Sort(b.Entries, key, false)
		first := b.Entries[1]
		Sort(b.Entries, key, true)
		if b.Entries[len(b.Entries)-1].Path != first.Path {
			t.Fatal("sort direction", key)
		}
	}
	for _, name := range []string{".env", "id_rsa", "identity.pem", ".ssh"} {
		if f, e := b.Open(filepath.Join(root, name)); e == nil {
			f.Close()
			t.Fatal("private open", name)
		}
	}
	if e := b.Navigate(filepath.Join(root, ".ssh")); e == nil {
		t.Fatal("private directory")
	}
	picker := New(Options{Locations: []Location{{"root", root}}, Extensions: []string{".txt"}})
	requireNoError(t, picker.Navigate(root))
	if len(picker.Entries) != 2 {
		t.Fatal("picker hidden/filter changed", picker.Entries)
	}
	if f, e := picker.Open(filepath.Join(root, ".visible.txt")); e == nil {
		f.Close()
		t.Fatal("hidden direct-open")
	}
}
func TestPinnedOpenAndSameSharedRoots(t *testing.T) {
	root, b := fixture(t)
	name := filepath.Join(root, "asset.vrm")
	write(t, name)
	f, e := b.Open(name)
	requireNoError(t, e)
	defer f.Close()
	os.Remove(name)
	os.Symlink("/etc/passwd", name)
	if _, e = b.Open(name); e == nil {
		t.Fatal("replacement escaped")
	}
	st, e := f.Stat()
	if e != nil || !st.Mode().IsRegular() {
		t.Fatal("lost pinned file")
	}
	roots := UserLocations(func(en, de string) string { return en })
	if len(roots) != 3 || roots[0].Path == filepath.Dir(roots[0].Path) {
		t.Fatal("roots")
	}
}

func TestHiddenDirectoryAndFileOpenRetainLeadingDot(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".album")
	requireNoError(t, os.Mkdir(dir, 0700))
	name := filepath.Join(dir, ".picture.png")
	write(t, name)
	b := New(Options{Locations: []Location{{Label: "files", Path: root}}, ShowHidden: true})
	requireNoError(t, b.Navigate(dir))
	if len(b.Entries) != 1 || b.Entries[0].Path != name {
		t.Fatal("hidden name rewritten")
	}
	f, e := b.Open(name)
	requireNoError(t, e)
	f.Close()
	b.Parent()
	if b.Current != root {
		t.Fatal("hidden parent")
	}
}
