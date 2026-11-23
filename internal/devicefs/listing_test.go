package devicefs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSharedDirectoryReaderMatchesHTTPPolicy(t *testing.T) {
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, "folder"), 0700)
	os.WriteFile(filepath.Join(root, "image.png"), []byte("metadata only"), 0600)
	got, e := ListDirectory(context.Background(), root, "/", 0)
	if e != nil || len(got.Entries) != 2 || got.Entries[0].Kind != "directory" {
		t.Fatal(got, e)
	}
	for _, name := range []string{"../", "/../x", "/bad\\name", "/bad\nname"} {
		if _, e := ListDirectory(context.Background(), root, name, 0); !errors.Is(e, errListingPath) {
			t.Fatal(name, e)
		}
	}
	if _, e := ListDirectory(context.Background(), root, "/", 1); !errors.Is(e, errListingOffset) {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := ListDirectory(ctx, root, "/", 0); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
