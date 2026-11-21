package userfiles

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ygelfand/libcountertop/pkg/runtime/webapi"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

func davRequestFor(h http.Handler, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "https://device.example"+path, strings.NewReader(body))
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestDAVSharedStorageAndMetadata(t *testing.T) {
	s := testStore(t)
	h := s.DAVHandler()
	if _, err := s.Upload("api first.bin", 3, strings.NewReader("api")); err != nil {
		t.Fatal(err)
	}
	w := davRequestFor(h, "PROPFIND", "/", "", map[string]string{"Depth": "1"})
	if w.Code != 207 {
		t.Fatal(w.Code, w.Body)
	}
	var out davMultistatus
	if err := xml.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err, w.Body)
	}
	if len(out.Responses) != 2 || out.Responses[1].Href != DAVPrefix+"/api%20first.bin" {
		t.Fatal(out, w.Body)
	}
	if !strings.Contains(w.Body.String(), "<collection") || !strings.Contains(w.Body.String(), ">3<") {
		t.Fatal(w.Body)
	}
	w = davRequestFor(h, "GET", "/api%20first.bin", "", nil)
	if w.Code != 200 || w.Body.String() != "api" {
		t.Fatal(w.Code, w.Body)
	}
	w = davRequestFor(h, "PUT", "/from-dav.bin", "dav", nil)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body)
	}
	w = davRequestFor(s.Handler(), "GET", "/from-dav.bin", "", nil)
	if w.Code != 200 || w.Body.String() != "dav" {
		t.Fatal(w.Code, w.Body)
	}
	w = davRequestFor(h, "GET", "/from-dav.bin", "", map[string]string{"Range": "bytes=1-2"})
	if w.Code != 206 || w.Body.String() != "av" {
		t.Fatal(w.Code, w.Body)
	}
	w = davRequestFor(h, "HEAD", "/from-dav.bin", "", nil)
	if w.Code != 200 || w.Body.Len() != 0 || w.Header().Get("Content-Length") != "3" {
		t.Fatal(w.Code, w.Body, w.Header())
	}
	w = davRequestFor(h, "DELETE", "/from-dav.bin", "", nil)
	if w.Code != 204 {
		t.Fatal(w.Code)
	}
	if _, _, err := s.Open("from-dav.bin"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}

func TestDAVPropertiesDepthAndBoundaries(t *testing.T) {
	s := testStore(t)
	h := s.DAVHandler()
	s.Upload("x", 1, strings.NewReader("x"))
	for _, depth := range []string{"", "infinity", "2", "-1"} {
		w := davRequestFor(h, "PROPFIND", "/", "", map[string]string{"Depth": depth})
		if w.Code != 400 && w.Code != 403 {
			t.Fatal(depth, w.Code)
		}
	}
	query := "<propfind xmlns=\"DAV:\"><prop><getcontentlength/><alien xmlns=\"urn:test\"/></prop></propfind>"
	w := davRequestFor(h, "PROPFIND", "/x", query, map[string]string{"Depth": "0"})
	if w.Code != 207 || !strings.Contains(w.Body.String(), "404 Not Found") || !strings.Contains(w.Body.String(), "urn:test") {
		t.Fatal(w.Code, w.Body)
	}
	for _, q := range []string{"<broken", "<propfind/>", "<propfind xmlns=\"DAV:\"><allprop/><propname/></propfind>", strings.Repeat("x", 32769)} {
		w := davRequestFor(h, "PROPFIND", "/", q, map[string]string{"Depth": "0"})
		if w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
	for _, path := range []string{"/../secret", "/a/b", "/.upload-secret", "/a%2fb", "/a%5cb", "/a%252fb"} {
		w := davRequestFor(h, "PUT", path, "x", nil)
		if w.Code != 400 {
			t.Fatal(path, w.Code)
		}
	}
	// Nothing in the adapter can traverse a hostile store entry.
	if err := os.Symlink(filepath.Join(t.TempDir(), "private"), filepath.Join(s.path, "link")); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"GET", "DELETE", "PROPFIND"} {
		w := davRequestFor(h, method, "/link", "", map[string]string{"Depth": "0"})
		if w.Code != 409 {
			t.Fatal(method, w.Code, w.Body)
		}
	}
}

func TestDAVUnsupportedAndSharedTransferRules(t *testing.T) {
	s := testStore(t)
	h := s.DAVHandler()
	for _, method := range []string{"MKCOL", "MOVE", "COPY", "LOCK", "UNLOCK", "PROPPATCH", "POST"} {
		w := davRequestFor(h, method, "/x", "", nil)
		if w.Code != 405 || w.Header().Get("DAV") != "" || strings.Contains(w.Header().Get("Allow"), method) {
			t.Fatal(method, w.Code, w.Header())
		}
	}
	for _, method := range []string{"PUT", "DELETE"} {
		if w := davRequestFor(h, method, "/", "", nil); w.Code != 405 {
			t.Fatal(method, w.Code)
		}
	}
	if w := davRequestFor(h, "PUT", "/x", "original", nil); w.Code != 201 {
		t.Fatal(w.Code)
	}
	if w := davRequestFor(h, "PUT", "/x", "changed", nil); w.Code != 409 {
		t.Fatal(w.Code)
	}
	if w := davRequestFor(h, "PUT", "/large", strings.Repeat("x", 65), nil); w.Code != 413 {
		t.Fatal(w.Code)
	}
	s.budget = 8
	if w := davRequestFor(h, "PUT", "/full", "x", nil); w.Code != 507 {
		t.Fatal(w.Code)
	}
	s.budget = 100
	if w := davRequestFor(h, "PUT", "/x", "", map[string]string{"If": "(<token>)"}); w.Code != 412 {
		t.Fatal(w.Code)
	}
	for _, method := range []string{"PROPFIND", "PUT", "DELETE"} {
		w := davRequestFor(h, method, "/x", "", map[string]string{"Origin": "https://evil.example", "Depth": "0"})
		if w.Code != 403 {
			t.Fatal(method, w.Code)
		}
	}
	// Existing API and DAV share the exact upload admission mutex.
	s.upload.Lock()
	if w := davRequestFor(h, "PUT", "/busy", "x", nil); w.Code != 423 {
		t.Fatal(w.Code)
	}
	s.upload.Unlock()
	r := httptest.NewRequest("PUT", "https://device.example/partial", strings.NewReader("short"))
	r.ContentLength = 12
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatal(w.Code, w.Body)
	}
	entries, _ := os.ReadDir(s.path)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".upload-") || e.Name() == "partial" {
			t.Fatal("partial published", e.Name())
		}
	}
}

func TestDAVCancelledUploadIsNeverPublished(t *testing.T) {
	s := testStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	body := &cancelReader{cancel: cancel}
	_, err := s.UploadContext(ctx, "cancelled", 1, body)
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	v, err := s.List()
	if err != nil || len(v.Files) != 0 {
		t.Fatal(v, err)
	}
	es, _ := os.ReadDir(s.path)
	if len(es) != 0 {
		t.Fatal("staging survived", es)
	}
}

type cancelReader struct{ cancel context.CancelFunc }

func (r *cancelReader) Read(p []byte) (int, error) { r.cancel(); copy(p, "x"); return 1, io.EOF }

func TestDAVOwnerRouter(t *testing.T) {
	s := testStore(t)
	m := &davOwnerSettings{schema.Frontend{Password: "not-a-real-secret", WebDAV: true}}
	a := webapi.New(webapi.Options{Settings: m, Pages: map[string]http.Handler{"webdav": s.DAVHandler(), "files": s.Handler()}})
	server := httptest.NewTLSServer(a.Handler())
	defer server.Close()
	client := server.Client()
	do := func(method, path string, data []byte, auth bool) *http.Response {
		req, _ := http.NewRequest(method, server.URL+path, bytes.NewReader(data))
		req.Header.Set("Depth", "1")
		if auth {
			req.SetBasicAuth("owner", "not-a-real-secret")
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { resp.Body.Close() })
		return resp
	}
	if r := do("PROPFIND", DAVPrefix+"/", nil, false); r.StatusCode != 401 {
		t.Fatal(r.StatusCode)
	}
	if r := do("PUT", DAVPrefix+"/roundtrip", []byte("real HTTPS"), true); r.StatusCode != 201 {
		t.Fatal(r.StatusCode)
	}
	r := do("GET", "/api/v1/files/roundtrip", nil, true)
	b, _ := io.ReadAll(r.Body)
	if r.StatusCode != 200 || string(b) != "real HTTPS" {
		t.Fatal(r.StatusCode, string(b))
	}
	if err := a.SetWebDAV(false); err != nil {
		t.Fatal(err)
	}
	if r := do("GET", DAVPrefix+"/roundtrip", nil, true); r.StatusCode != 404 {
		t.Fatal(r.StatusCode)
	}
	if r := do("GET", "/api/v1/files/roundtrip", nil, true); r.StatusCode != 200 {
		t.Fatal(r.StatusCode)
	}
}

type davOwnerSettings struct{ v schema.Frontend }

func (m *davOwnerSettings) Read() schema.Frontend                  { return m.v }
func (m *davOwnerSettings) Update(fn func(*schema.Frontend)) error { fn(&m.v); return nil }
