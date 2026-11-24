package userfiles

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/ygelfand/libcountertop/pkg/runtime/webapi"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s := New(filepath.Join(t.TempDir(), "files"))
	s.maxFile = 64
	s.budget = 100
	s.reserve = 10
	s.free = func() (int64, error) { return 1000, nil }
	t.Cleanup(func() {
		if s.root != nil {
			s.root.Close()
		}
	})
	return s
}
func TestRoundTripAndRestart(t *testing.T) {
	s := testStore(t)
	data := []byte{0, 1, 255, 42}
	m, err := s.Upload("asset.vrm", int64(len(data)), bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if m.Size != 4 {
		t.Fatal(m)
	}
	v, err := s.List()
	if err != nil || len(v.Files) != 1 || v.Used != 4 || v.Available != 96 {
		t.Fatal(v, err)
	}
	f, _, err := s.Open("asset.vrm")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(f)
	f.Close()
	if !bytes.Equal(got, data) {
		t.Fatal("bytes changed")
	}
	next := New(s.path)
	defer func() {
		if next.root != nil {
			next.root.Close()
		}
	}()
	v, err = next.List()
	if err != nil || len(v.Files) != 1 {
		t.Fatal(v, err)
	}
	if err := next.Delete("asset.vrm"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Open("asset.vrm"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}
func TestInvalidNames(t *testing.T) {
	s := testStore(t)
	for _, name := range []string{"", "..", "../state.json", "/etc/passwd", "a\\b", "a\x00b", "a\nb", ".upload-x", ".dav-properties", "a//b", "a/./b", "a/../b", strings.Repeat("x", 256)} {
		if ValidName(name) {
			t.Errorf("accepted %q", name)
		}
		if _, err := s.Upload(name, 0, strings.NewReader("")); !errors.Is(err, ErrName) {
			t.Errorf("%q: %v", name, err)
		}
		if _, _, err := s.Open(name); !errors.Is(err, ErrName) {
			t.Fatal(name, err)
		}
		if err := s.Delete(name); !errors.Is(err, ErrName) {
			t.Fatal(name, err)
		}
	}
}
func TestNoOverwrite(t *testing.T) {
	s := testStore(t)
	s.Upload("x", 3, strings.NewReader("old"))
	if _, err := s.Upload("x", 3, strings.NewReader("new")); !errors.Is(err, ErrExists) {
		t.Fatal(err)
	}
	f, _, _ := s.Open("x")
	defer f.Close()
	b, _ := io.ReadAll(f)
	if string(b) != "old" {
		t.Fatal(string(b))
	}
}
func TestLimits(t *testing.T) {
	t.Run("per-file", func(t *testing.T) {
		s := testStore(t)
		_, err := s.Upload("x", 65, strings.NewReader(""))
		if !errors.Is(err, ErrTooLarge) {
			t.Fatal(err)
		}
	})
	t.Run("budget", func(t *testing.T) {
		s := testStore(t)
		s.Upload("a", 60, strings.NewReader(strings.Repeat("x", 60)))
		_, err := s.Upload("b", 41, strings.NewReader(""))
		if !errors.Is(err, ErrFull) {
			t.Fatal(err)
		}
	})
	t.Run("reserve", func(t *testing.T) {
		s := testStore(t)
		s.free = func() (int64, error) { return 15, nil }
		_, err := s.Upload("x", 6, strings.NewReader(""))
		if !errors.Is(err, ErrFull) {
			t.Fatal(err)
		}
	})
	t.Run("count", func(t *testing.T) {
		s := testStore(t)
		s.init()
		for i := range MaxFiles {
			s.root.WriteFile(fmtName(i), nil, 0600)
		}
		_, err := s.Upload("next", 0, strings.NewReader(""))
		if !errors.Is(err, ErrFull) {
			t.Fatal(err)
		}
	})
}
func fmtName(n int) string { return "file-" + string(rune('a'+n/26)) + string(rune('a'+n%26)) }

type broken struct{}

func (broken) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func TestFailedUploadsLeaveNoActiveOrPartialFiles(t *testing.T) {
	for _, tc := range []struct {
		name string
		size int64
		body io.Reader
	}{
		{"short", 10, strings.NewReader("x")}, {"long", 1, strings.NewReader("xx")}, {"broken", 10, broken{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testStore(t)
			if _, err := s.Upload("x", tc.size, tc.body); err == nil {
				t.Fatal("accepted")
			}
			v, err := s.List()
			if err != nil || len(v.Files) != 0 {
				t.Fatal(v, err)
			}
			es, _ := s.entries()
			if len(es) != 0 {
				t.Fatal("temporary file leaked")
			}
		})
	}
}
func TestCrashLeftoversAndSymlinks(t *testing.T) {
	s := testStore(t)
	os.MkdirAll(s.path, 0700)
	os.WriteFile(filepath.Join(s.path, ".upload-stale"), []byte("partial"), 0600)
	if v, err := s.List(); err != nil || len(v.Files) != 0 {
		t.Fatal(v, err)
	}
	if es, _ := s.entries(); len(es) != 0 {
		t.Fatal(es)
	}
	outside := filepath.Join(t.TempDir(), "private")
	os.WriteFile(outside, []byte("secret"), 0600)
	os.Symlink(outside, filepath.Join(s.path, "link"))
	if _, _, err := s.Open("link"); err == nil {
		t.Fatal("followed symlink")
	}
	if err := s.Delete("link"); !errors.Is(err, ErrBoundary) {
		t.Fatal(err)
	}
	if _, err := s.List(); !errors.Is(err, ErrBoundary) {
		t.Fatal(err)
	}
	if _, err := s.Upload("link", 1, strings.NewReader("x")); !errors.Is(err, ErrExists) {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(outside)
	if string(b) != "secret" {
		t.Fatal("outside changed")
	}
}
func TestDirectoryAndFIFORejected(t *testing.T) {
	s := testStore(t)
	s.init()
	os.Mkdir(filepath.Join(s.path, "dir"), 0700)
	syscall.Mkfifo(filepath.Join(s.path, "pipe"), 0600)
	for _, name := range []string{"dir", "pipe"} {
		if _, _, err := s.Open(name); !errors.Is(err, ErrBoundary) {
			t.Fatal(name, err)
		}
	}
}
func TestRootSymlinkRejected(t *testing.T) {
	p := filepath.Join(t.TempDir(), "link")
	os.Symlink(t.TempDir(), p)
	s := New(p)
	if _, err := s.List(); !errors.Is(err, ErrBoundary) {
		t.Fatal(err)
	}
}

type gated struct{ entered, release chan struct{} }

func (g gated) Read(p []byte) (int, error) { close(g.entered); <-g.release; return 0, io.EOF }
func TestSingleUploadAndResponsiveList(t *testing.T) {
	s := testStore(t)
	g := gated{make(chan struct{}), make(chan struct{})}
	done := make(chan error)
	go func() { _, err := s.Upload("first", 0, g); done <- err }()
	<-g.entered
	if _, err := s.Upload("second", 0, strings.NewReader("")); !errors.Is(err, ErrBusy) {
		t.Fatal(err)
	}
	if v, err := s.List(); err != nil || len(v.Files) != 0 {
		t.Fatal(v, err)
	}
	close(g.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
func TestReserveRecheckedBeforePublication(t *testing.T) {
	s := testStore(t)
	calls := 0
	s.free = func() (int64, error) {
		calls++
		if calls > 1 {
			return 0, nil
		}
		return 1000, nil
	}
	if _, err := s.Upload("x", 1, strings.NewReader("x")); !errors.Is(err, ErrFull) {
		t.Fatal(err)
	}
	es, _ := s.entries()
	if len(es) != 0 {
		t.Fatal(es)
	}
}
func TestDiskFullMapped(t *testing.T) {
	w := httptest.NewRecorder()
	httpFail(w, syscall.ENOSPC)
	if w.Code != 507 {
		t.Fatal(w.Code)
	}
}

type memory struct{ v schema.Frontend }

func (m *memory) Read() schema.Frontend                 { return m.v }
func (m *memory) Update(f func(*schema.Frontend)) error { f(&m.v); return nil }
func TestOwnerHTTPSMount(t *testing.T) {
	s := testStore(t)
	a := webapi.New(webapi.Options{Settings: &memory{schema.Frontend{Password: "synthetic-password"}}, Pages: map[string]http.Handler{"files": s.Handler()}})
	h := a.Handler()
	request := func(method, path, body string, auth bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "https://device.invalid/api/v1/files"+path, strings.NewReader(body))
		if auth {
			r.SetBasicAuth("owner", "synthetic-password")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	for _, method := range []string{"GET", "PUT", "DELETE"} {
		if w := request(method, "/a", "x", false); w.Code != 401 {
			t.Fatal(method, w.Code)
		}
	}
	if w := request("PUT", "/a", "abc", true); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request("GET", "", "", true); w.Code != 200 || !strings.Contains(w.Body.String(), "\"name\":\"a\"") {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request("GET", "/a", "", true); w.Code != 200 || w.Body.String() != "abc" || !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment") {
		t.Fatal(w)
	}
	if w := request("DELETE", "/a", "", true); w.Code != 204 {
		t.Fatal(w.Code, w.Body.String())
	}
	r := httptest.NewRequest("PUT", "https://device.invalid/api/v1/files/x", strings.NewReader("x"))
	r.SetBasicAuth("owner", "synthetic-password")
	r.Header.Set("Origin", "https://other.invalid")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
	for _, path := range []string{"/..%2Fstate.json", "/a%5Cb", "/.upload-x"} {
		if w := request("GET", path, "", true); w.Code < 400 {
			t.Fatal(path, w.Code)
		}
	}
}

func TestInternalSymlinkAlsoRejected(t *testing.T) {
	s := testStore(t)
	s.Upload("real", 1, strings.NewReader("x"))
	os.Symlink("real", filepath.Join(s.path, "alias"))
	if _, _, err := s.Open("alias"); err == nil {
		t.Fatal("internal symlink followed")
	}
}
func TestHTTPTransferErrors(t *testing.T) {
	s := testStore(t)
	for _, tc := range []struct {
		method, path, body string
		length             int64
		code               int
	}{
		{"PUT", "/too-big", "", 65, 413}, {"PUT", "/unknown", "", -1, 411},
		{"PUT", "/short", "x", 2, 400}, {"GET", "/missing", "", 0, 404},
		{"POST", "/x", "", 0, 405}, {"GET", "/a%2fb", "", 0, 404},
	} {
		r := httptest.NewRequest(tc.method, "https://device.invalid"+tc.path, strings.NewReader(tc.body))
		r.ContentLength = tc.length
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != tc.code {
			t.Errorf("%s %s: got %d want %d (%s)", tc.method, tc.path, w.Code, tc.code, w.Body.String())
		}
	}
}

func TestOSReservedBlocksAreNotDoubleCounted(t *testing.T) {
	for _, tc := range []struct{ available, free, reserve, want int64 }{
		{110, 200, 128, 72}, {110, 200, 64, 110}, {10, 100, 128, -28}, {40, 128, 128, 0},
	} {
		if got := reserveCapacity(tc.available, tc.free, tc.reserve); got != tc.want {
			t.Errorf("got %d want %d", got, tc.want)
		}
	}
}
