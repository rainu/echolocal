package devicefs

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ygelfand/libcountertop/pkg/runtime/webapi"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
	"golang.org/x/sys/unix"
)

func transferRequest(h http.Handler, method, name string, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "https://device.invalid/file?path="+url.QueryEscape(name), bytes.NewReader(body))
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func TestDownloadsAcrossRealFilesystem(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "system", "etc"), 0700)
	// Synthetic configuration fixture, never an actual device credential.
	data := []byte{0, 255, 1, 2, 42}
	os.WriteFile(filepath.Join(root, "system", "etc", "sample.conf"), data, 0600)
	os.Symlink("system/etc/sample.conf", filepath.Join(root, "alias"))
	h := handler(root)
	for _, name := range []string{"/system/etc/sample.conf", "/alias"} {
		w := transferRequest(h, "GET", name, nil, nil)
		if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), data) {
			t.Fatal(name, w.Code)
		}
		if !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment") || w.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatal("unsafe headers")
		}
	}
	w := transferRequest(h, "GET", "/system/etc/sample.conf", nil, map[string]string{"Range": "bytes=1-3"})
	if w.Code != 206 || !bytes.Equal(w.Body.Bytes(), data[1:4]) {
		t.Fatal("range", w.Code)
	}
	w = transferRequest(h, "HEAD", "/system/etc/sample.conf", nil, nil)
	if w.Code != 200 || w.Body.Len() != 0 || w.Header().Get("Content-Length") != "5" {
		t.Fatal("head", w)
	}
	// Download size is independent of the upload limit; use a sparse fixture
	// and a one-byte range instead of allocating a large response in RAM.
	f, e := os.Create(filepath.Join(root, "large.bin"))
	if e != nil {
		t.Fatal(e)
	}
	f.Truncate(MaxUploadBytes + 1)
	f.Close()
	w = transferRequest(h, "GET", "/large.bin", nil, map[string]string{"Range": "bytes=134217728-134217728"})
	if w.Code != 206 || w.Body.Len() != 1 {
		t.Fatal("large download restricted", w.Code)
	}
	unix.Mkfifo(filepath.Join(root, "pipe"), 0600)
	for _, name := range []string{"/pipe", "/system", "/missing"} {
		if w := transferRequest(h, "GET", name, nil, nil); w.Code < 400 {
			t.Fatal("non-file", name, w.Code)
		}
	}
}
func TestUploadToChosenDirectoryAndUnicodeNames(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"one", "two"} {
		os.Mkdir(filepath.Join(root, d), 0700)
	}
	os.Symlink("two", filepath.Join(root, "alias"))
	h := handler(root)
	for _, target := range []string{"/one/.ä & file.bin", "/two/sample.bin", "/alias/alias-write.bin"} {
		payload := []byte(target)
		w := transferRequest(h, "PUT", target, payload, nil)
		if w.Code != 201 {
			t.Fatalf("%s: %d %s", target, w.Code, w.Body.String())
		}
		got := transferRequest(h, "GET", target, nil, nil)
		if !bytes.Equal(got.Body.Bytes(), payload) {
			t.Fatal("bytes changed")
		}
	}
	if _, err := os.Stat(filepath.Join(root, "two", "alias-write.bin")); err != nil {
		t.Fatal("symlink did not resolve to actual directory")
	}
	if w := transferRequest(h, "PUT", "/one/.ä & file.bin", []byte("replace"), nil); w.Code != 409 {
		t.Fatal("overwrite", w.Code)
	}
	before, _ := os.ReadFile(filepath.Join(root, "one", ".ä & file.bin"))
	if string(before) != "/one/.ä & file.bin" {
		t.Fatal("existing bytes changed")
	}
	// Existing symlinks are collisions, never replacement targets.
	os.Symlink("../one/.ä & file.bin", filepath.Join(root, "two", "link"))
	if w := transferRequest(h, "PUT", "/two/link", []byte("bad"), nil); w.Code != 409 {
		t.Fatal("followed upload final symlink")
	}
	for _, folder := range []string{"one", "two"} {
		entries, _ := os.ReadDir(filepath.Join(root, folder))
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".echolocal-upload-") {
				t.Fatal("temporary file leaked")
			}
		}
	}
}
func TestUploadLimitsOriginAndIncompleteCleanup(t *testing.T) {
	root := t.TempDir()
	h := handler(root)
	for _, header := range []map[string]string{{"Origin": "https://foreign.invalid"}, {"Sec-Fetch-Site": "cross-site"}} {
		if w := transferRequest(h, "PUT", "/denied", []byte("x"), header); w.Code != 403 {
			t.Fatal(w.Code)
		}
	}
	for _, tc := range []struct {
		length int64
		body   string
		status int
	}{{-1, "", 411}, {MaxUploadBytes + 1, "", 413}, {3, "x", 400}, {0, "x", 400}} {
		r := httptest.NewRequest("PUT", "https://device.invalid/file?path=/partial", strings.NewReader(tc.body))
		r.ContentLength = tc.length
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatal(tc, w.Code, w.Body.String())
		}
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatal("partial or rejected upload left files", entries)
	}
	// Errors from read-only filesystems and exhausted space remain actionable.
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{{unix.EROFS, 403, "read_only_filesystem"}, {unix.EACCES, 403, "permission_denied"}, {unix.ENOSPC, 507, "storage_full"}, {unix.EDQUOT, 507, "storage_full"}, {errFull, 507, "storage_full"}} {
		w := httptest.NewRecorder()
		transferFailure(w, tc.err)
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) {
			t.Fatal(tc, w.Code)
		}
	}
}

type blockedBody struct {
	started chan struct{}
	release chan struct{}
}

func (b *blockedBody) Read(p []byte) (int, error) {
	close(b.started)
	<-b.release
	p[0] = 'x'
	return 1, io.EOF
}
func TestConcurrentUploadAndAtomicCollision(t *testing.T) {
	root := t.TempDir()
	h := handler(root)
	body := &blockedBody{make(chan struct{}), make(chan struct{})}
	r := httptest.NewRequest("PUT", "https://device.invalid/file?path=/raced", body)
	r.ContentLength = 1
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { w := httptest.NewRecorder(); h.ServeHTTP(w, r); done <- w }()
	<-body.started
	if w := transferRequest(h, "PUT", "/other", []byte("x"), nil); w.Code != 423 {
		t.Fatal("parallel upload", w.Code)
	}
	// Race the final publication: the external file must win without replacement.
	os.WriteFile(filepath.Join(root, "raced"), []byte("external"), 0600)
	close(body.release)
	if w := <-done; w.Code != 409 {
		t.Fatal("publication overwrote collision", w.Code, w.Body.String())
	}
	b, _ := os.ReadFile(filepath.Join(root, "raced"))
	if string(b) != "external" {
		t.Fatal("external file changed")
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 1 {
		t.Fatal("failed transfer leaked partial", entries)
	}
}
func TestResolvedPathsStayInsideTrustedTestRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "file"), []byte("outside"), 0600)
	os.Symlink(outside, filepath.Join(root, "external"))
	h := handler(root)
	for _, method := range []string{"GET", "PUT"} {
		if w := transferRequest(h, method, "/external/file", []byte("x"), nil); w.Code != 403 {
			t.Fatal(method, w.Code)
		}
	}
	os.Mkdir(filepath.Join(root, "original"), 0700)
	os.Symlink("original", filepath.Join(root, "alias"))
	canonical, err := resolve(root, "/alias")
	if err != nil {
		t.Fatal(err)
	}
	d, err := openDirectory(root, canonical)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	os.Rename(filepath.Join(root, "original"), filepath.Join(root, "moved"))
	os.Symlink(outside, filepath.Join(root, "original"))
	_, err = upload(d, "pinned", 1, strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(root, "moved", "pinned")); err != nil {
		t.Fatal("lost pinned directory")
	}
	if _, err = os.Stat(filepath.Join(outside, "pinned")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("redirected pinned upload")
	}
	if _, err = openDirectory(root, canonical); err == nil {
		t.Fatal("followed a swapped component")
	}
}

func TestOwnerFileTransferMount(t *testing.T) {
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, "system"), 0700)
	os.WriteFile(filepath.Join(root, "system", "fixture.conf"), []byte("synthetic"), 0600)
	a := webapi.New(webapi.Options{Settings: &memory{schema.Frontend{Password: "synthetic-password"}}, Pages: map[string]http.Handler{"filesystem": handler(root)}})
	for _, method := range []string{"GET", "HEAD", "PUT"} {
		r := httptest.NewRequest(method, "https://device.invalid/api/v1/filesystem/file?path=/system/fixture.conf", strings.NewReader("x"))
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatal("unauthorized transfer", method, w.Code)
		}
	}
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{{"GET", "/system/fixture.conf", "", 200}, {"HEAD", "/system/fixture.conf", "", 200}, {"PUT", "/system/new.conf", "created", 201}} {
		r := httptest.NewRequest(tc.method, "https://device.invalid/api/v1/filesystem/file?path="+url.QueryEscape(tc.path), strings.NewReader(tc.body))
		r.SetBasicAuth("owner", "synthetic-password")
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatal("owner transfer", tc.method, w.Code, w.Body.String())
		}
	}
}
