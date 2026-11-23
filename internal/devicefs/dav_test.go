package devicefs

import (
	"bytes"
	"encoding/xml"
	"github.com/ygelfand/echolocal/internal/userfiles"
	"golang.org/x/sys/unix"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func davReq(t *testing.T, s *Service, m, n, b string, h map[string]string, want int) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(m, "https://device.example"+n, strings.NewReader(b))
	for k, v := range h {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	s.DAVHandler().ServeHTTP(w, r)
	if w.Code != want {
		t.Fatalf("%s %s: %d != %d: %s", m, n, w.Code, want, w.Body)
	}
	return w
}
func TestDeviceDAVUsesBrowserFilesystem(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "system/etc"), 0700)
	os.Mkdir(filepath.Join(root, "data"), 0700)
	os.WriteFile(filepath.Join(root, "system/etc/sample.txt"), []byte("synthetic"), 0600)
	os.Symlink("system", filepath.Join(root, "alias"))
	unix.Mkfifo(filepath.Join(root, "pipe"), 0600)
	s := NewService(root, "")
	w := davReq(t, s, "PROPFIND", "/", "", map[string]string{"Depth": "1"}, 207)
	var x struct{ XMLName xml.Name }
	if e := xml.Unmarshal(w.Body.Bytes(), &x); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"system/", "data/", "alias/", "pipe"} {
		if !strings.Contains(w.Body.String(), name) {
			t.Fatal("missing root entry", name, w.Body)
		}
	}
	w = davReq(t, s, "GET", "/alias/etc/sample.txt", "", nil, 200)
	if w.Body.String() != "synthetic" {
		t.Fatal(w.Body)
	}
	davReq(t, s, "PUT", "/data/from-dav", "dav bytes", nil, 201)
	w = transferRequest(s.Handler(), "GET", "/data/from-dav", nil, nil)
	if w.Code != 200 || w.Body.String() != "dav bytes" {
		t.Fatal(w.Code, w.Body)
	}
	w = transferRequest(s.Handler(), "PUT", "/data/from-api", []byte("api bytes"), nil)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body)
	}
	if b := davReq(t, s, "GET", "/data/from-api", "", nil, 200).Body.String(); b != "api bytes" {
		t.Fatal(b)
	}
	davReq(t, s, "GET", "/pipe", "", nil, 403)
}
func TestDeviceDAVWritesPropertiesCopyAndRestart(t *testing.T) {
	root := t.TempDir()
	meta := filepath.Join(t.TempDir(), "props.json")
	s := NewService(root, meta)
	davReq(t, s, "MKCOL", "/Folder/", "", nil, 201)
	davReq(t, s, "PUT", "/Folder/x", "old", nil, 201)
	patch := `<propertyupdate xmlns="DAV:"><set><prop><test xmlns="urn:test">value</test></prop></set></propertyupdate>`
	davReq(t, s, "PROPPATCH", "/Folder/x", patch, nil, 207)
	if b := davReq(t, s, "GET", "/Folder/x", "", nil, 200).Body.String(); b != "old" {
		t.Fatal("metadata truncated bytes", b)
	}
	davReq(t, s, "COPY", "/Folder", "", map[string]string{"Destination": "https://device.example" + userfiles.DAVPrefix + "/Copy"}, 201)
	davReq(t, s, "MOVE", "/Copy/x", "", map[string]string{"Destination": "https://device.example" + userfiles.DAVPrefix + "/moved"}, 201)
	s = NewService(root, meta)
	w := davReq(t, s, "PROPFIND", "/moved", "", map[string]string{"Depth": "0"}, 207)
	if !strings.Contains(w.Body.String(), "value") {
		t.Fatal(w.Body)
	}
	davReq(t, s, "PUT", "/moved", "new", nil, 204)
	davReq(t, s, "COPY", "/Folder/x", "", map[string]string{"Destination": "https://device.example" + userfiles.DAVPrefix + "/moved"}, 204)
	davReq(t, s, "MOVE", "/Folder/x", "", map[string]string{"Destination": "https://device.example" + userfiles.DAVPrefix + "/moved", "Overwrite": "T"}, 204)
	davReq(t, s, "PUT", "/moved", "new", nil, 204)
	davReq(t, s, "PUT", "/missing/child", "x", nil, 409)
	davReq(t, s, "MKCOL", "/missing/child/", "", nil, 409)
	if b := davReq(t, s, "GET", "/moved", "", nil, 200).Body.String(); b != "new" {
		t.Fatal(b)
	}
	davReq(t, s, "DELETE", "/Folder", "", nil, 204)
	davReq(t, s, "DELETE", "/Copy", "", nil, 204)
}

const devLock = `<D:lockinfo xmlns:D="DAV:"><D:lockscope><D:shared/></D:lockscope><D:locktype><D:write/></D:locktype></D:lockinfo>`

func TestDeviceDAVAliasLocksAlsoProtectBrowserUploads(t *testing.T) {
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, "data"), 0700)
	os.Symlink("data", filepath.Join(root, "alias"))
	s := NewService(root, "")
	w := davReq(t, s, "LOCK", "/alias", devLock, map[string]string{"Depth": "infinity"}, 200)
	token := w.Header().Get("Lock-Token")
	w = transferRequest(s.Handler(), "PUT", "/data/new", []byte("blocked"), nil)
	if w.Code != 423 {
		t.Fatal("browser bypassed lock", w.Code, w.Body)
	}
	davReq(t, s, "PUT", "/data/new", "blocked", nil, 423)
	davReq(t, s, "PUT", "/alias/new", "allowed", map[string]string{"If": "(" + token + ")"}, 201)
	davReq(t, s, "UNLOCK", "/data", "", map[string]string{"Lock-Token": token}, 204)
}
func TestDeviceDAVFailedReplacePreservesOldFile(t *testing.T) {
	root := t.TempDir()
	s := NewService(root, "")
	davReq(t, s, "PUT", "/x", "old", nil, 201)
	r := httptest.NewRequest("PUT", "https://device.example/x", strings.NewReader("short"))
	r.ContentLength = 100
	w := httptest.NewRecorder()
	s.DAVHandler().ServeHTTP(w, r)
	if w.Code < 400 {
		t.Fatal(w.Code)
	}
	if b := davReq(t, s, "GET", "/x", "", nil, 200).Body.String(); b != "old" {
		t.Fatal(b)
	}
	// A failed COPY must undo its destination removal, not destroy the old file.
	unix.Mkfifo(filepath.Join(root, "pipe"), 0600)
	w = davReq(t, s, "COPY", "/pipe", "", map[string]string{"Destination": "https://device.example" + userfiles.DAVPrefix + "/x"}, 403)
	if b := davReq(t, s, "GET", "/x", "", nil, 200).Body.String(); b != "old" {
		t.Fatal(b)
	}
	files, _ := os.ReadDir(root)
	for _, f := range files {
		if strings.HasPrefix(f.Name(), ".echolocal-") {
			t.Fatal("staging leaked", f.Name())
		}
	}
}
func TestDeviceDAVBoundsAndSpecialFiles(t *testing.T) {
	root := t.TempDir()
	s := NewService(root, "")
	outside := filepath.Join(t.TempDir(), "outside")
	os.WriteFile(outside, []byte("private fixture"), 0600)
	os.Symlink(outside, filepath.Join(root, "outside"))
	for n, code := range map[string]int{"/../outside": 400, "/.echolocal-upload-x": 403, "/a%5cb": 400} {
		w := davReq(t, s, "PUT", n, "x", nil, code)
		_ = w
	}
	davReq(t, s, "GET", "/outside", "", nil, 403)
	davReq(t, s, "PROPFIND", "/", "", map[string]string{"Depth": "infinity"}, 403)
	davReq(t, s, "DELETE", "/", "", nil, 403)
	davReq(t, s, "COPY", "/", "", map[string]string{"Destination": "https://device.example" + userfiles.DAVPrefix + "/copy"}, 403)
}
func TestDeviceDAVOptimizedCopyDoesNotBypassAccounting(t *testing.T) {
	root := t.TempDir()
	s := NewService(root, "")
	b := bytes.Repeat([]byte{1, 2, 3, 4}, 32768)
	r := httptest.NewRequest("PUT", "https://device.example/source", bytes.NewReader(b))
	w := httptest.NewRecorder()
	s.DAVHandler().ServeHTTP(w, r)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body)
	}
	davReq(t, s, "COPY", "/source", "", map[string]string{"Destination": "https://device.example" + userfiles.DAVPrefix + "/copy"}, 201)
	r = httptest.NewRequest("GET", "https://device.example/copy", nil)
	w = httptest.NewRecorder()
	s.DAVHandler().ServeHTTP(w, r)
	got, _ := io.ReadAll(w.Result().Body)
	if !bytes.Equal(got, b) {
		t.Fatal("copy bytes changed")
	}
	_ = http.MethodGet
}

func TestDeviceDAVDefaultHTTPSPortAndHostnameCase(t *testing.T) {
	s := NewService(t.TempDir(), "")
	davReq(t, s, "PUT", "/source", "bytes", nil, 201)
	davReq(t, s, "COPY", "/source", "", map[string]string{"Destination": "https://DEVICE.example:443" + userfiles.DAVPrefix + "/target"}, 201)
}

type stalledDownload struct {
	h              http.Header
	start, release chan struct{}
	once           sync.Once
}

func (w *stalledDownload) Header() http.Header { return w.h }
func (w *stalledDownload) WriteHeader(int)     {}
func (w *stalledDownload) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.start) })
	<-w.release
	return len(p), nil
}
func TestDeviceDAVOpenDownloadsDoNotBlockWriters(t *testing.T) {
	for _, dav := range []bool{false, true} {
		root := t.TempDir()
		os.WriteFile(filepath.Join(root, "download"), bytes.Repeat([]byte("a"), 65536), 0600)
		s := NewService(root, "")
		w := &stalledDownload{h: http.Header{}, start: make(chan struct{}), release: make(chan struct{})}
		done := make(chan struct{})
		uri := "https://device.example/file?path=/download"
		handler := s.Handler()
		if dav {
			uri = "https://device.example/download"
			handler = s.DAVHandler()
		}
		go func() { defer close(done); handler.ServeHTTP(w, httptest.NewRequest("GET", uri, nil)) }()
		<-w.start
		wrote := make(chan int)
		go func() {
			r := httptest.NewRequest("PUT", "https://device.example/new", strings.NewReader("new"))
			out := httptest.NewRecorder()
			s.DAVHandler().ServeHTTP(out, r)
			wrote <- out.Code
		}()
		select {
		case code := <-wrote:
			if code != 201 {
				t.Error(code)
			}
		case <-time.After(2 * time.Second):
			t.Error("download blocks unrelated writer")
		}
		close(w.release)
		<-done
	}
}
