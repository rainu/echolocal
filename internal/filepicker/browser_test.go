package filepicker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func fixture(t *testing.T) (string, *Browser) {
	t.Helper()
	root := t.TempDir()
	return root, New(Options{Locations: []Location{{"files", root}}, Extensions: []string{".vrm"}})
}
func write(t *testing.T, name string) {
	t.Helper()
	if err := os.WriteFile(name, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
}
func TestFilteringOrderingAndLargeDirectory(t *testing.T) {
	root, b := fixture(t)
	for i := 0; i < 300; i++ {
		write(t, filepath.Join(root, fmt.Sprintf("model-%03d.VRM", 299-i)))
	}
	long := strings.Repeat("long-", 45) + ".vrm"
	write(t, filepath.Join(root, long))
	write(t, filepath.Join(root, "unsupported.png"))
	write(t, filepath.Join(root, ".private.vrm"))
	if err := os.Mkdir(filepath.Join(root, "Z-folder"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(root, "pipe.vrm"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(
		filepath.Join(root, "model-000.VRM"),
		filepath.Join(root, "link.vrm"),
	); err != nil {
		t.Fatal(err)
	}
	if err := b.Navigate(root); err != nil {
		t.Fatal(err)
	}
	if len(b.Entries) != 302 || !b.Entries[0].Directory || b.Entries[1].Name != long ||
		b.Entries[2].Name != "model-000.VRM" {
		t.Fatal("filter/order/long names", len(b.Entries), b.Entries[:3])
	}
	b.Select(filepath.Join(root, long))
	if got, err := b.Confirm(); err != nil || got != filepath.Join(root, long) {
		t.Fatal(got, err)
	}
}
func TestInitialMissingParentEmptyAndDirectoryMode(t *testing.T) {
	root, b := fixture(t)
	b.options.Initial = filepath.Join(root, "deleted", "model.vrm")
	b.Start()
	if b.Current != root || b.Err == nil || b.Selected != "" {
		t.Fatal("missing initial", b)
	}
	requireNoError(t, b.Navigate(root))
	if len(b.Entries) != 0 || b.Err != nil {
		t.Fatal("empty folder")
	}
	b.Parent()
	if b.Current != "" {
		t.Fatal("escaped location")
	}
	b.Parent()
	if b.Current != "" {
		t.Fatal("virtual parent")
	}
	dir := filepath.Join(root, "folder")
	requireNoError(t, os.Mkdir(dir, 0700))
	write(t, filepath.Join(root, "a.vrm"))
	d := New(Options{Locations: []Location{{"files", root}}, Directory: true, Initial: dir})
	d.Start()
	if got, err := d.Confirm(); err != nil || got != dir {
		t.Fatal(got, err)
	}
	d.Parent()
	if len(d.Entries) != 1 || !d.Entries[0].Directory {
		t.Fatal("directory mode included files", d.Entries)
	}
}
func TestInitialFileAndRevalidation(t *testing.T) {
	root, b := fixture(t)
	name := filepath.Join(root, "test.vrm")
	write(t, name)
	b.options.Initial = name
	b.Start()
	if b.Selected != name {
		t.Fatal("lost initial file")
	}
	requireNoError(t, os.Remove(name))
	if _, err := b.Confirm(); err == nil {
		t.Fatal("accepted removed file")
	}
	write(t, name)
	b.Select(name)
	outside := filepath.Join(t.TempDir(), "secret.vrm")
	write(t, outside)
	requireNoError(t, os.Remove(name))
	requireNoError(t, os.Symlink(outside, name))
	if _, err := b.Confirm(); err == nil {
		t.Fatal("accepted replaced symlink")
	}
	b.Select(outside)
	if b.Selected != "" {
		t.Fatal("selected unlisted outside file")
	}
	for _, invalid := range []string{outside, root + "/../secret", root + "/.hidden", "relative", root + "/bad\nname"} {
		if b.Navigate(invalid) == nil {
			t.Fatal("navigated invalid/outside", invalid)
		}
	}
	// Reject a symlinked directory even when its target is inside the root.
	requireNoError(t, os.Mkdir(filepath.Join(root, "real"), 0700))
	requireNoError(t, os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "alias")))
	if b.Navigate(filepath.Join(root, "alias")) == nil {
		t.Fatal("followed directory symlink")
	}
	if b.Current != root {
		t.Fatal("failed navigation lost usable folder")
	}
}
func TestInitialOutsideDoesNotBrowseIt(t *testing.T) {
	_, b := fixture(t)
	b.options.Initial = filepath.Join(t.TempDir(), "secret.vrm")
	b.Start()
	if b.Current != "" || len(b.Entries) != 0 || b.Err == nil {
		t.Fatal("outside initial exposed")
	}
}
func TestPinnedRegularOpenRejectsSpecialReplacement(t *testing.T) {
	root, b := fixture(t)
	name := filepath.Join(root, "test.vrm")
	write(t, name)
	requireNoError(t, b.Navigate(root))
	b.Select(name)
	requireNoError(t, os.Remove(name))
	if err := syscall.Mkfifo(name, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Confirm(); err == nil {
		t.Fatal("opened FIFO")
	}
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
