package networkmedia

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestProbeValidationAndHTTPPrivacy(t *testing.T) {
	s := fixture(t)
	for _, u := range []Update{{}, {Settings: Settings{Host: "user:secret@host", Port: 445, Share: "videos", Username: "reader"}}} {
		if r := s.Test(context.Background(), u); r.OK || r.Code != "test_invalid" {
			t.Fatal(r)
		}
	}
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"POST", "/test", `{"host":"bad-secret@host","password":"do-not-echo"}`, 200},
		{"POST", "/test", `{"extra":"do-not-echo"}`, 400},
		{"POST", "/test", `{} {}`, 400},
		{"GET", "/test", "", 405},
		{"POST", "/unknown", "", 404},
	} {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
		if w.Code != tc.status || strings.Contains(w.Body.String(), "do-not-echo") || strings.Contains(w.Body.String(), "bad-secret") {
			t.Fatal("unsafe test response", w.Code)
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("test response cacheable")
		}
	}
	if _, e := os.Stat(s.path); !os.IsNotExist(e) {
		t.Fatal("test created configuration")
	}
}

func TestProbeCancellationAndSingleFlight(t *testing.T) {
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		c, e := listener.Accept()
		if e == nil {
			accepted <- c
		}
	}()
	addr := listener.Addr().(*net.TCPAddr)
	s := fixture(t)
	pw := "synthetic-password"
	u := Update{Settings: Settings{Host: "127.0.0.1", Port: addr.Port, Share: "videos", Username: "reader"}, Password: &pw}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan TestResult, 1)
	go func() { done <- s.Test(ctx, u) }()
	var c net.Conn
	select {
	case c = <-accepted:
		defer c.Close()
	case <-time.After(time.Second):
		t.Fatal("probe did not connect")
	}
	if r := s.Test(context.Background(), u); r.Code != "test_busy" {
		t.Fatal("unbounded probe concurrency", r.Code)
	}
	cancel()
	select {
	case r := <-done:
		if r.Code != "test_cancelled" {
			t.Fatal(r.Code)
		}
	case <-time.After(time.Second):
		t.Fatal("cancel did not release socket")
	}
	if len(s.testSlots) != 0 {
		t.Fatal("probe slot leaked")
	}
	go func() {
		c, e := listener.Accept()
		if e == nil {
			defer c.Close()
			time.Sleep(time.Second)
		}
	}()
	timed, stop := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer stop()
	if r := s.Test(timed, u); r.Code != "test_timeout" {
		t.Fatal("deadline category", r.Code)
	}
}

func TestLiveSMBProbeDraftAndRetainedPassword(t *testing.T) {
	name := os.Getenv("ECHOLOCAL_SMB_TEST_CONFIG")
	if name == "" {
		t.Skip("isolated SMB fixture required")
	}
	b, e := os.ReadFile(name)
	if e != nil {
		t.Fatal("fixture config unavailable")
	}
	var cfg saved
	if json.Unmarshal(b, &cfg) != nil {
		t.Fatal("fixture config malformed")
	}
	s := fixture(t)
	changed := 0
	s.Changed = func() { changed++ }
	u := Update{Settings: cfg.Settings, Password: &cfg.Password}
	u.Enabled = false
	if r := s.Test(context.Background(), u); !r.OK {
		t.Fatal("draft probe", r.Code)
	}
	if changed != 0 {
		t.Fatal("probe interrupted playback")
	}
	if _, e := os.Stat(s.path); !os.IsNotExist(e) {
		t.Fatal("draft persisted")
	}
	u.Enabled = true
	if e = s.Save(u); e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(s.path)
	changed = 0
	u.Password = nil
	if r := s.Test(context.Background(), u); !r.OK {
		t.Fatal("retained-password probe", r.Code)
	}
	wrong := "wrong-synthetic-password"
	u.Password = &wrong
	if r := s.Test(context.Background(), u); r.Code != "test_auth" {
		t.Fatal("wrong-password category", r.Code)
	}
	u.Password = nil
	u.Share = "missing-fixture"
	if r := s.Test(context.Background(), u); r.Code != "test_share" {
		t.Fatal("missing-share category", r.Code)
	}
	u.Settings = cfg.Settings
	u.Port++
	if r := s.Test(context.Background(), u); r.Code != "test_connect" {
		t.Fatal("offline category", r.Code)
	}
	after, _ := os.ReadFile(s.path)
	if !bytes.Equal(before, after) || changed != 0 {
		t.Fatal("probe changed saved settings or stopped media")
	}
}
