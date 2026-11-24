package devicefs

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/ygelfand/libcountertop/pkg/runtime/webapi"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

func request(h http.Handler, method, target string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(method, "https://device.invalid"+target, nil))
	return w
}
func decode(t *testing.T, w *httptest.ResponseRecorder) Listing {
	t.Helper()
	if w.Code != 200 {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var listing Listing
	if err := json.Unmarshal(w.Body.Bytes(), &listing); err != nil {
		t.Fatal(err)
	}
	return listing
}
func TestRealMetadataAndExternalChanges(t *testing.T) {
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, "nested"), 0700)
	const payload = "SYNTHETIC PRIVATE CONTENT MUST NEVER BE RETURNED"
	os.WriteFile(filepath.Join(root, "state.json"), []byte(payload), 0600)
	os.Symlink("nested", filepath.Join(root, "alias"))
	if err := syscall.Mkfifo(filepath.Join(root, "pipe"), 0600); err != nil {
		t.Fatal(err)
	}
	h := handler(root)
	w := request(h, "GET", "/")
	v := decode(t, w)
	if v.Path != "/" || v.Parent != "" || len(v.Entries) != 4 {
		t.Fatal(v)
	}
	kinds := map[string]string{}
	for _, entry := range v.Entries {
		kinds[entry.Name] = entry.Kind
	}
	if kinds["nested"] != "directory" || kinds["state.json"] != "file" || kinds["alias"] != "symlink" || kinds["pipe"] != "special" {
		t.Fatal(kinds)
	}
	if strings.Contains(w.Body.String(), payload) {
		t.Fatal("file contents leaked")
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("metadata cached")
	}
	os.WriteFile(filepath.Join(root, "nested", "outside-api.txt"), []byte("x"), 0600)
	v = decode(t, request(h, "GET", "/?path=/nested"))
	if v.Parent != "/" || v.Path != "/nested" || len(v.Entries) != 1 || v.Entries[0].Name != "outside-api.txt" {
		t.Fatal(v)
	}
	os.Remove(filepath.Join(root, "nested", "outside-api.txt"))
	if len(decode(t, request(h, "GET", "/?path=/nested")).Entries) != 0 {
		t.Fatal("stale or virtual inventory")
	}
}
func TestListingsDoNotReadFilesOrWriteAndResolveDirectoryAliases(t *testing.T) {
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, "dir"), 0700)
	os.WriteFile(filepath.Join(root, "dir", "secret"), []byte("fixture"), 0600)
	os.Symlink("dir", filepath.Join(root, "internal-link"))
	os.Symlink(t.TempDir(), filepath.Join(root, "external-link"))
	syscall.Mkfifo(filepath.Join(root, "pipe"), 0600)
	h := handler(root)
	v := decode(t, request(h, "GET", "/?path=/internal-link"))
	if v.Path != "/dir" || len(v.Entries) != 1 {
		t.Fatal("alias did not navigate to its real directory", v)
	}
	for _, name := range []string{"/dir/secret", "/internal-link/child", "/external-link", "/pipe"} {
		w := request(h, "GET", "/?path="+url.QueryEscape(name))
		if w.Code < 400 {
			t.Errorf("opened %q: %d", name, w.Code)
		}
		if strings.Contains(w.Body.String(), "fixture") {
			t.Fatal("content returned")
		}
	}
	for _, method := range []string{"PUT", "POST", "DELETE", "PATCH", "HEAD"} {
		if w := request(h, method, "/?path=/dir"); w.Code != 405 {
			t.Fatal(method, w.Code)
		}
	}
	if w := request(h, "GET", "/dir/secret"); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if b, err := os.ReadFile(filepath.Join(root, "dir", "secret")); err != nil || string(b) != "fixture" {
		t.Fatal("source changed")
	}
}
func TestPathsAndPaging(t *testing.T) {
	root := t.TempDir()
	h := handler(root)
	for _, p := range []string{"relative", "/../x", "/dir/../x", "/./x", "/x\x00", "/x\\y", "/x\nz"} {
		if w := request(h, "GET", "/?path="+url.QueryEscape(p)); w.Code != 400 {
			t.Fatal(p, w.Code)
		}
	}
	for _, offset := range []string{"-1", "1", "invalid", "999999999999999999999"} {
		if w := request(h, "GET", "/?offset="+offset); w.Code != 400 {
			t.Fatal(offset, w.Code)
		}
	}
	// Non-ASCII, hidden names and spaces are real entries, not upload-name rules.
	os.Mkdir(filepath.Join(root, "dír name"), 0700)
	os.WriteFile(filepath.Join(root, "dír name", ".hidden"), nil, 0600)
	if v := decode(t, request(h, "GET", "/?path="+url.QueryEscape("/dír name"))); len(v.Entries) != 1 || v.Entries[0].Name != ".hidden" {
		t.Fatal(v)
	}
	for i := 0; i < 300; i++ {
		os.WriteFile(filepath.Join(root, fmt.Sprintf("f%03d", i)), nil, 0600)
	}
	first := decode(t, request(h, "GET", "/"))
	if len(first.Entries) != pageSize || first.NextOffset == nil || *first.NextOffset != pageSize {
		t.Fatal("page one", len(first.Entries), first.NextOffset)
	}
	second := decode(t, request(h, "GET", "/?offset=256"))
	if len(second.Entries) != 45 || second.NextOffset != nil {
		t.Fatal("page two", len(second.Entries))
	}
	seen := map[string]bool{}
	for _, v := range append(first.Entries, second.Entries...) {
		if seen[v.Name] {
			t.Fatal("duplicate", v.Name)
		}
		seen[v.Name] = true
	}
	if len(seen) != 301 {
		t.Fatal("missing entries", len(seen))
	}
	if w := request(h, "GET", "/?path=/missing"); w.Code != 404 {
		t.Fatal(w.Code)
	}
}

type memory struct{ v schema.Frontend }

func (m *memory) Read() schema.Frontend                 { return m.v }
func (m *memory) Update(f func(*schema.Frontend)) error { f(&m.v); return nil }
func TestOwnerOnlyMount(t *testing.T) {
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, "directory"), 0700)
	a := webapi.New(webapi.Options{Settings: &memory{schema.Frontend{Password: "synthetic-password"}}, Pages: map[string]http.Handler{"filesystem": handler(root)}})
	for _, authenticated := range []bool{false, true} {
		r := httptest.NewRequest("GET", "https://device.invalid/api/v1/filesystem?path=/", nil)
		if authenticated {
			r.SetBasicAuth("owner", "synthetic-password")
		}
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		if authenticated {
			decode(t, w)
		} else if w.Code != 401 {
			t.Fatal("anonymous metadata", w.Code)
		}
	}
}

func TestGlobalDirectoriesFirstAlphabeticalAcrossPages(t *testing.T) {
	root := t.TempDir()
	// Deliberately create files before directories and in reverse name order.
	for i := 299; i >= 0; i-- {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("item-%03d", i)), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"z-file.txt", "apple.txt", "APPLE.txt", "A-file.txt"} {
		os.WriteFile(filepath.Join(root, name), nil, 0600)
	}
	for _, name := range []string{"Zulu", "Beta", "alpha"} {
		os.Mkdir(filepath.Join(root, name), 0700)
	}
	os.Symlink("Zulu", filepath.Join(root, "aardvark-link"))
	h := handler(root)
	first := decode(t, request(h, "GET", "/"))
	expected := []string{"aardvark-link", "alpha", "Beta", "Zulu", "A-file.txt", "APPLE.txt", "apple.txt", "item-000"}
	for i, name := range expected {
		if first.Entries[i].Name != name {
			t.Fatalf("position %d: got %q want %q", i, first.Entries[i].Name, name)
		}
	}
	if len(first.Entries) != 256 || first.Entries[255].Name != "item-248" || first.NextOffset == nil || *first.NextOffset != 256 {
		t.Fatal("incorrect globally sorted first page")
	}
	if first.Entries[0].TargetKind != "directory" {
		t.Fatal("directory alias not classified as folder")
	}
	second := decode(t, request(h, "GET", "/?offset=256"))
	if len(second.Entries) != 52 || second.Entries[0].Name != "item-249" || second.Entries[51].Name != "z-file.txt" || second.NextOffset != nil {
		t.Fatal("second page is not the continuation of the globally sorted directory")
	}
}
