package userfiles

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"github.com/ygelfand/libcountertop/pkg/runtime/webapi"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
	"golang.org/x/net/webdav"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func davRequestFor(h http.Handler, method, p, b string, headers map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "https://device.example"+p, strings.NewReader(b))
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func mustDAV(t *testing.T, h http.Handler, m, p, b string, headers map[string]string, want int) *httptest.ResponseRecorder {
	t.Helper()
	w := davRequestFor(h, m, p, b, headers)
	if w.Code != want {
		t.Fatalf("%s %s: %d != %d: %s", m, p, w.Code, want, w.Body)
	}
	return w
}
func TestDAVDiscoveryAndSharedHierarchy(t *testing.T) {
	s := testStore(t)
	h := s.DAVHandler()
	w := mustDAV(t, h, "OPTIONS", "/", "", nil, 200)
	if w.Header().Get("DAV") != "1, 2" {
		t.Fatal(w.Header())
	}
	mustDAV(t, h, "MKCOL", "/Folder", "", nil, 201)
	mustDAV(t, h, "MKCOL", "/Folder/Sub", "", nil, 201)
	mustDAV(t, h, "PUT", "/Folder/Sub/a.txt", "hello", nil, 201)
	f, _, e := s.Open("Folder/Sub/a.txt")
	if e != nil {
		t.Fatal(e)
	}
	b, _ := io.ReadAll(f)
	f.Close()
	if string(b) != "hello" {
		t.Fatal(string(b))
	}
	if _, e = s.Upload("Folder/api.txt", 3, strings.NewReader("api")); e != nil {
		t.Fatal(e)
	}
	mustDAV(t, h, "GET", "/Folder/api.txt", "", nil, 200)
	w = mustDAV(t, h, "PROPFIND", "/", "", map[string]string{"Depth": "infinity"}, 207)
	var v struct{ XMLName xml.Name }
	if e = xml.Unmarshal(w.Body.Bytes(), &v); e != nil || !strings.Contains(w.Body.String(), "a.txt") {
		t.Fatal(e, w.Body)
	}
	v2, e := s.List()
	if e != nil || len(v2.Files) != 2 || v2.Used != 8 {
		t.Fatal(v2, e)
	}
	mustDAV(t, h, "DELETE", "/Folder", "", nil, 204)
	if v, e := s.List(); e != nil || len(v.Files) != 0 {
		t.Fatal(v, e)
	}
}
func TestDAVOverwritePreconditionsAndAtomicFailure(t *testing.T) {
	s := testStore(t)
	h := s.DAVHandler()
	mustDAV(t, h, "PUT", "/x", "old", nil, 201)
	w := mustDAV(t, h, "GET", "/x", "", nil, 200)
	etag := w.Header().Get("ETag")
	mustDAV(t, h, "PUT", "/x", "new", map[string]string{"If-None-Match": "*"}, 412)
	mustDAV(t, h, "PUT", "/x", "new", map[string]string{"If-None-Match": "W/" + etag}, 412)
	mustDAV(t, h, "PUT", "/x", "new", map[string]string{"If-Match": "\"wrong\""}, 412)
	mustDAV(t, h, "PUT", "/x", "new", map[string]string{"If-Match": etag}, 204)
	mustDAV(t, h, "GET", "/x", "", map[string]string{"If-None-Match": mustDAV(t, h, "HEAD", "/x", "", nil, 200).Header().Get("ETag")}, 304)
	if _, e := s.Upload("x", 3, strings.NewReader("api")); !errors.Is(e, ErrExists) {
		t.Fatal(e)
	}
	r := httptest.NewRequest("PUT", "https://device.example/x", strings.NewReader("bad"))
	r.ContentLength = 10
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code < 400 {
		t.Fatal(w.Code)
	}
	w = mustDAV(t, h, "GET", "/x", "", nil, 200)
	if w.Body.String() != "new" {
		t.Fatal("partial replacement", w.Body)
	}
	r = httptest.NewRequest("PUT", "https://device.example/x", brokenBody{})
	r.ContentLength = -1
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code < 400 {
		t.Fatal(w.Code)
	}
	if b := mustDAV(t, h, "GET", "/x", "", nil, 200).Body.String(); b != "new" {
		t.Fatal(b)
	}
}

type brokenBody struct{}

func (brokenBody) Read(p []byte) (int, error) { copy(p, "bad"); return 3, io.ErrUnexpectedEOF }
func (brokenBody) Close() error               { return nil }
func TestDAVCopyMovePropertiesAndRestart(t *testing.T) {
	s := testStore(t)
	h := s.DAVHandler()
	mustDAV(t, h, "MKCOL", "/A", "", nil, 201)
	mustDAV(t, h, "PUT", "/A/x", "source", nil, 201)
	patch := `<D:propertyupdate xmlns:D="DAV:" xmlns:T="urn:test"><D:set><D:prop><T:label>ä &amp; value</T:label></D:prop></D:set></D:propertyupdate>`
	mustDAV(t, h, "PROPPATCH", "/A/x", patch, nil, 207)
	dest := func(p string) map[string]string {
		return map[string]string{"Destination": "https://device.example" + DAVPrefix + p}
	}
	mustDAV(t, h, "COPY", "/A", "", dest("/B"), 201)
	w := mustDAV(t, h, "PROPFIND", "/B/x", `<propfind xmlns="DAV:"><allprop/></propfind>`, map[string]string{"Depth": "0"}, 207)
	if !strings.Contains(w.Body.String(), "value") {
		t.Fatal(w.Body)
	}
	mustDAV(t, h, "MOVE", "/B", "", dest("/C"), 201)
	mustDAV(t, h, "GET", "/B/x", "", nil, 404)
	mustDAV(t, h, "PUT", "/target", "old", nil, 201)
	mustDAV(t, h, "MOVE", "/C/x", "", dest("/target"), 204)
	if b := mustDAV(t, h, "GET", "/target", "", nil, 200).Body.String(); b != "source" {
		t.Fatal(b)
	}
	next := New(s.path)
	next.free = s.free
	defer next.rootCleanup()
	w = mustDAV(t, next.DAVHandler(), "PROPFIND", "/target", "", map[string]string{"Depth": "0"}, 207)
	if !strings.Contains(w.Body.String(), "value") {
		t.Fatal("properties lost", w.Body)
	}
}
func (s *Store) rootCleanup() {
	if s.root != nil {
		s.root.Close()
	}
}
func TestDAVCopyFailurePreservesEntireDestination(t *testing.T) {
	s := testStore(t)
	h := s.DAVHandler()
	mustDAV(t, h, "MKCOL", "/A", "", nil, 201)
	mustDAV(t, h, "PUT", "/A/x", strings.Repeat("x", 60), nil, 201)
	mustDAV(t, h, "PUT", "/target", "old", nil, 201)
	mustDAV(t, h, "COPY", "/A", "", map[string]string{"Destination": "https://device.example" + DAVPrefix + "/target"}, 507)
	if b := mustDAV(t, h, "GET", "/target", "", nil, 200).Body.String(); b != "old" {
		t.Fatal("failed COPY deleted destination", b)
	}
	mustDAV(t, h, "COPY", "/A", "", map[string]string{"Destination": "https://device.example" + DAVPrefix + "/A/nested"}, 403)
}

const lockBody = `<D:lockinfo xmlns:D="DAV:"><D:lockscope><D:exclusive/></D:lockscope><D:locktype><D:write/></D:locktype><D:owner>test</D:owner></D:lockinfo>`

func TestDAVLocksAcrossAPIAndProtocol(t *testing.T) {
	s := testStore(t)
	h := s.DAVHandler()
	mustDAV(t, h, "PUT", "/x", "old", nil, 201)
	w := mustDAV(t, h, "LOCK", "/x", lockBody, map[string]string{"Depth": "0", "Timeout": "Second-60"}, 200)
	token := w.Header().Get("Lock-Token")
	if !strings.HasPrefix(token, "<opaquelocktoken:") {
		t.Fatal(token)
	}
	mustDAV(t, h, "PUT", "/x", "bad", nil, 423)
	if e := s.Delete("x"); !errors.Is(e, ErrBusy) {
		t.Fatal("API bypassed lock", e)
	}
	w = mustDAV(t, h, "PROPFIND", "/x", `<propfind xmlns="DAV:"><prop><lockdiscovery/></prop></propfind>`, map[string]string{"Depth": "0"}, 207)
	if !strings.Contains(w.Body.String(), "activelock") {
		t.Fatal(w.Body)
	}
	mustDAV(t, h, "PUT", "/x", "new", map[string]string{"If": "(" + token + ")"}, 204)
	mustDAV(t, h, "LOCK", "/x", "", map[string]string{"If": "(" + token + ")", "Timeout": "Second-120"}, 200)
	mustDAV(t, h, "UNLOCK", "/x", "", map[string]string{"Lock-Token": token}, 204)
	if e := s.Delete("x"); e != nil {
		t.Fatal(e)
	}
	mustDAV(t, h, "MKCOL", "/Dir", "", nil, 201)
	w = mustDAV(t, h, "LOCK", "/Dir", lockBody, nil, 200)
	token = w.Header().Get("Lock-Token")
	if _, e := s.Upload("Dir/new", 0, strings.NewReader("")); !errors.Is(e, ErrBusy) {
		t.Fatal("API bypassed ancestor lock", e)
	}
	mustDAV(t, h, "UNLOCK", "/Dir", "", map[string]string{"Lock-Token": token}, 204)
	mustDAV(t, h, "LOCK", "/new", lockBody, nil, 201)
	mustDAV(t, h, "GET", "/new", "", nil, 200)
}
func TestDAVQuotaNamesAndBoundaries(t *testing.T) {
	s := testStore(t)
	h := s.DAVHandler()
	for _, n := range []string{"/.upload-private", "/.dav-properties", "/a/../x", "/a%5cb", "/a//b"} {
		w := davRequestFor(h, "PUT", n, "x", nil)
		if w.Code < 400 {
			t.Fatal(n, w.Code)
		}
	}
	for _, n := range []string{"/.client-temp", "/Grüße%20%26%20world.txt"} {
		mustDAV(t, h, "PUT", n, "x", nil, 201)
	}
	mustDAV(t, h, "PUT", "/too-big", strings.Repeat("x", 65), nil, 413)
	s.free = func() (int64, error) { return 10, nil }
	mustDAV(t, h, "PUT", "/full", "x", nil, 507)
	s.free = func() (int64, error) { return 1000, nil }
	outside := filepath.Join(t.TempDir(), "private")
	os.WriteFile(outside, []byte("private"), 0600)
	os.Symlink(outside, filepath.Join(s.path, "alias"))
	if w := davRequestFor(h, "GET", "/alias", "", nil); w.Code < 400 {
		t.Fatal(w.Code)
	}
	if b, _ := os.ReadFile(outside); string(b) != "private" {
		t.Fatal("outside touched")
	}
}
func TestDAVCancelledTransactionsAndReadAvailability(t *testing.T) {
	s := testStore(t)
	h := s.DAVHandler()
	mustDAV(t, h, "PUT", "/x", "old", nil, 201)
	g := gated{make(chan struct{}), make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	r := httptest.NewRequest("PUT", "https://device.example/x", io.NopCloser(g)).WithContext(ctx)
	r.ContentLength = 0
	done := make(chan int)
	go func() { w := httptest.NewRecorder(); h.ServeHTTP(w, r); done <- w.Code }()
	<-g.entered
	if v, e := s.List(); e != nil || len(v.Files) != 1 {
		t.Fatal(v, e)
	}
	if b := mustDAV(t, h, "GET", "/x", "", nil, 200).Body.String(); b != "old" {
		t.Fatal(b)
	}
	mustDAV(t, h, "PUT", "/busy", "x", nil, 423)
	cancel()
	close(g.release)
	if code := <-done; code < 400 {
		t.Fatal(code)
	}
	if b := mustDAV(t, h, "GET", "/x", "", nil, 200).Body.String(); b != "old" {
		t.Fatal(b)
	}
}
func TestDAVCrashJournalRecovery(t *testing.T) {
	for _, published := range []bool{false, true} {
		t.Run(fmtBool(published), func(t *testing.T) {
			s := testStore(t)
			s.Upload("old", 1, strings.NewReader("a"))
			base, temp, previous := journalNames(s.path)
			parent, e := os.OpenRoot(filepath.Dir(s.path))
			if e != nil {
				t.Fatal(e)
			}
			defer parent.Close()
			parent.Mkdir(temp, 0700)
			parent.WriteFile(temp+"/new", []byte("b"), 0600)
			if e = parent.Rename(base, previous); e != nil {
				t.Fatal(e)
			}
			if published {
				if e = parent.Rename(temp, base); e != nil {
					t.Fatal(e)
				}
			}
			next := New(s.path)
			defer next.rootCleanup()
			v, e := next.List()
			if e != nil {
				t.Fatal(e)
			}
			want := "old"
			if published {
				want = "new"
			}
			if len(v.Files) != 1 || v.Files[0].Name != want {
				t.Fatal(v)
			}
		})
	}
}
func fmtBool(v bool) string {
	if v {
		return "published"
	}
	return "unpublished"
}

type davOwnerSettings struct{ v schema.Frontend }

func (m *davOwnerSettings) Read() schema.Frontend                 { return m.v }
func (m *davOwnerSettings) Update(f func(*schema.Frontend)) error { f(&m.v); return nil }
func TestDAVOwnerRouter(t *testing.T) {
	s := testStore(t)
	m := &davOwnerSettings{schema.Frontend{Password: "synthetic-test", WebDAV: true}}
	a := webapi.New(webapi.Options{Settings: m, Pages: map[string]http.Handler{"webdav": s.DAVHandler(), "files": s.Handler()}})
	srv := httptest.NewTLSServer(a.Handler())
	defer srv.Close()
	cl := srv.Client()
	do := func(method, p, b string, auth bool) *http.Response {
		t.Helper()
		r, _ := http.NewRequest(method, srv.URL+p, bytes.NewBufferString(b))
		if auth {
			r.SetBasicAuth("owner", "synthetic-test")
		}
		r.Header.Set("Depth", "1")
		resp, e := cl.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { resp.Body.Close() })
		return resp
	}
	if r := do("OPTIONS", DAVPrefix+"/", "", false); r.StatusCode != 401 {
		t.Fatal(r.StatusCode)
	}
	if r := do("OPTIONS", DAVPrefix+"/", "", true); r.Header.Get("DAV") != "1, 2" {
		t.Fatal(r.StatusCode, r.Header)
	}
	if r := do("PUT", DAVPrefix+"/roundtrip", "real HTTPS", true); r.StatusCode != 201 {
		t.Fatal(r.StatusCode)
	}
	r := do("GET", "/api/v1/files/roundtrip", "", true)
	b, _ := io.ReadAll(r.Body)
	if string(b) != "real HTTPS" {
		t.Fatal(r.StatusCode, string(b))
	}
	if e := a.SetWebDAV(false); e != nil {
		t.Fatal(e)
	}
	if r := do("GET", DAVPrefix+"/roundtrip", "", true); r.StatusCode != 404 {
		t.Fatal(r.StatusCode)
	}
	if r := do("GET", "/api/v1/files/roundtrip", "", true); r.StatusCode != 200 {
		t.Fatal(r.StatusCode)
	}
}
func TestDAVLockExpiry(t *testing.T) {
	l := newDAVLocks()
	now := time.Now()
	token, e := l.Create(now, webdav.LockDetails{Root: "/x", Duration: time.Millisecond, ZeroDepth: true})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = l.Refresh(now.Add(time.Second), token, time.Minute); e == nil {
		t.Fatal("expired token revived")
	}
}

func TestDAVConformanceRegressions(t *testing.T) {
	s := testStore(t)
	h := s.DAVHandler()
	mustDAV(t, h, "PUT", "/missing/child", "x", nil, 409)
	mustDAV(t, h, "PROPFIND", "/", `<D:propfind xmlns:D="DAV:"><D:prop><p:x xmlns:p=""/></D:prop></D:propfind>`, nil, 400)
	mustDAV(t, h, "PROPFIND", "/", `<D:propfind xmlns:D="DAV:"><D:prop><unknown:x/></D:prop></D:propfind>`, nil, 400)
	mustDAV(t, h, "PUT", "/x", "intact", nil, 201)
	mustDAV(t, h, "PROPPATCH", "/x", `<propertyupdate xmlns="DAV:"><set><prop><p xmlns="urn:test">value</p></prop></set></propertyupdate>`, nil, 207)
	if b := mustDAV(t, h, "GET", "/x", "", nil, 200).Body.String(); b != "intact" {
		t.Fatal("property update truncated data", b)
	}
	shared := strings.ReplaceAll(lockBody, "exclusive", "shared")
	one := mustDAV(t, h, "LOCK", "/x", shared, map[string]string{"Depth": "0"}, 200).Header().Get("Lock-Token")
	two := mustDAV(t, h, "LOCK", "/x", shared, map[string]string{"Depth": "0"}, 200).Header().Get("Lock-Token")
	mustDAV(t, h, "PUT", "/x", "bad", nil, 423)
	mustDAV(t, h, "PUT", "/x", "bad", map[string]string{"If": "(" + one + ` ["wrong"])`}, 412)
	etag := mustDAV(t, h, "HEAD", "/x", "", nil, 200).Header().Get("ETag")
	mustDAV(t, h, "PUT", "/x", "valid", map[string]string{"If": "(" + one + " [" + etag + "])"}, 204)
	mustDAV(t, h, "UNLOCK", "/other", "", map[string]string{"Lock-Token": one}, 409)
	mustDAV(t, h, "UNLOCK", "/x", "", map[string]string{"Lock-Token": one}, 204)
	mustDAV(t, h, "PUT", "/x", "also valid", map[string]string{"If": "(" + two + ")"}, 204)
	mustDAV(t, h, "UNLOCK", "/x", "", map[string]string{"Lock-Token": two}, 204)
}

func TestDAVCollectionDeleteHonorsChildLock(t *testing.T) {
	s := testStore(t)
	h := s.DAVHandler()
	mustDAV(t, h, "MKCOL", "/parent", "", nil, 201)
	mustDAV(t, h, "PUT", "/parent/x", "data", nil, 201)
	token := mustDAV(t, h, "LOCK", "/parent/x", lockBody, map[string]string{"Depth": "0"}, 200).Header().Get("Lock-Token")
	mustDAV(t, h, "DELETE", "/parent", "", nil, 423)
	mustDAV(t, h, "UNLOCK", "/parent/x", "", map[string]string{"Lock-Token": token}, 204)
	mustDAV(t, h, "DELETE", "/parent", "", nil, 204)
}

func TestDAVOpenDownloadSurvivesPublication(t *testing.T) {
	s := testStore(t)
	h := s.DAVHandler()
	mustDAV(t, h, "PUT", "/x", "old data", nil, 201)
	f, _, e := s.Open("x")
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	mustDAV(t, h, "PUT", "/x", "new data", nil, 204)
	b, e := io.ReadAll(f)
	if e != nil || string(b) != "old data" {
		t.Fatal(string(b), e)
	}
	if b := mustDAV(t, h, "GET", "/x", "", nil, 200).Body.String(); b != "new data" {
		t.Fatal(b)
	}
}

func TestDAVTaggedConditionsAndLockDeletion(t *testing.T) {
	s := testStore(t)
	h := s.DAVHandler()
	mustDAV(t, h, "PUT", "/source", "source", nil, 201)
	mustDAV(t, h, "PUT", "/dest", "dest", nil, 201)
	source := mustDAV(t, h, "LOCK", "/source", lockBody, map[string]string{"Depth": "0"}, 200).Header().Get("Lock-Token")
	dest := mustDAV(t, h, "LOCK", "/dest", lockBody, map[string]string{"Depth": "0"}, 200).Header().Get("Lock-Token")
	hdr := map[string]string{"Destination": "https://device.example" + DAVPrefix + "/dest", "If": "<https://device.example" + DAVPrefix + "/source> (" + source + ") <https://device.example" + DAVPrefix + "/dest> (" + dest + ")"}
	mustDAV(t, h, "MOVE", "/source", "", hdr, 204)
	if b := mustDAV(t, h, "GET", "/dest", "", nil, 200).Body.String(); b != "source" {
		t.Fatal(b)
	}
	mustDAV(t, h, "PUT", "/source", "new source", nil, 201) // source lock did not move
	mustDAV(t, h, "DELETE", "/dest", "", map[string]string{"If": "(" + dest + ")"}, 204)
	mustDAV(t, h, "PUT", "/dest", "new dest", nil, 201) // delete removed destination lock
	w := mustDAV(t, h, "PROPFIND", "/source", "", map[string]string{"Depth": "0"}, 207)
	if strings.Count(w.Body.String(), "<D:supportedlock") != 1 || !strings.Contains(w.Body.String(), "<D:shared/>") {
		t.Fatal(w.Body)
	}
}

func TestDAVContentTypeMatchesMetadata(t *testing.T) {
	s := testStore(t)
	h := s.DAVHandler()
	mustDAV(t, h, "PUT", "/note.txt", "text", nil, 201)
	w := mustDAV(t, h, "GET", "/note.txt", "", nil, 200)
	ctype := w.Header().Get("Content-Type")
	if !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment") {
		t.Fatal(w.Header())
	}
	p := mustDAV(t, h, "PROPFIND", "/note.txt", `<propfind xmlns="DAV:"><prop><getcontenttype/></prop></propfind>`, map[string]string{"Depth": "0"}, 207)
	if !strings.Contains(p.Body.String(), ctype) {
		t.Fatal("GET metadata content-type mismatch", ctype, p.Body)
	}
	mustDAV(t, h, "PUT", "/note.txt/child", "data", nil, 409)
	mustDAV(t, h, "MKCOL", "/note.txt/dir", "", nil, 409)
}
