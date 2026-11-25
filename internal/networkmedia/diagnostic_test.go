package networkmedia

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http/httptest"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	smb2 "github.com/cloudsoda/go-smb2"
	"github.com/ygelfand/libcountertop/pkg/media/localvideo"
	"github.com/ygelfand/libcountertop/pkg/media/urlplayer"
	"github.com/ygelfand/libcountertop/pkg/say"
)

func TestSMBDiagnosticReasonsAndPrivacy(t *testing.T) {
	before := say.Chosen()
	say.Use("en")
	t.Cleanup(func() { say.Use(before) })
	const secret = "private-user:private-password@private-host/private-file"
	tests := []struct {
		err         error
		stage, want string
	}{
		{&smb2.ResponseError{Code: 0xc0000022}, "open", "access denied"},
		{&smb2.ResponseError{Code: 0xc0000033}, "metadata", "path is invalid"},
		{&smb2.ResponseError{Code: 0xc00000ba}, "open", "folder"},
		{&smb2.ResponseError{Code: 0xc0000043}, "open", "locked"},
		{&smb2.ResponseError{Code: 0xc0000034}, "mount", "share is unavailable"},
		{&smb2.ResponseError{Code: 0xc000006d}, "auth", "sign-in failed"},
		{&smb2.ResponseError{Code: 0xc000035c}, "read", "no longer valid"},
		{&smb2.ResponseError{Code: 0xc00000bb}, "metadata", "rejected this operation"},
		{&smb2.TransportError{Err: &net.OpError{Op: "read " + secret, Err: syscall.ECONNRESET}}, "read", "CONNECTION_RESET"},
		{&os.PathError{Op: secret, Path: secret, Err: &smb2.TransportError{Err: context.DeadlineExceeded}}, "open", "TIMEOUT"},
		{&smb2.TransportError{Err: &net.DNSError{Name: secret, Server: secret, Err: secret}}, "connect", "DNS_LOOKUP_FAILED"},
		{&smb2.TransportError{Err: syscall.ECONNREFUSED}, "connect", "ECONNREFUSED"},
		{io.EOF, "auth", "CONNECTION_CLOSED"},
		{io.ErrUnexpectedEOF, "read", "UNEXPECTED_EOF"},
		{&smb2.InvalidResponseError{Message: secret}, "read", "INVALID_SMB_RESPONSE"},
		{&smb2.InternalError{Message: secret}, "metadata", "SMB_CLIENT_ERROR"},
		{&os.PathError{Op: secret, Path: secret, Err: &smb2.ResponseError{Code: 0xdeadbeef}}, "metadata", "NTSTATUS 0xDEADBEEF"},
		{errors.New(secret), secret, "exact cause could not be determined"},
	}
	for _, tc := range tests {
		got := category(tc.err, tc.stage)
		if !strings.Contains(got, tc.want) || !strings.Contains(got, "Step:") || !strings.Contains(got, "code:") {
			t.Errorf("missing diagnosis for %T at %q: %s", tc.err, tc.stage, got)
		}
		for _, word := range []string{"private-user", "private-password", "private-host", "private-file"} {
			if strings.Contains(got, word) {
				t.Fatal("diagnostic leaked request data")
			}
		}
	}
	got := category(&smb2.ResponseError{Code: 0xc000000d}, "metadata")
	if !strings.Contains(got, "STATUS_INVALID_PARAMETER (0xC000000D)") || !strings.Contains(got, "read path metadata") {
		t.Fatal("lost actual operation/status", got)
	}
}
func TestSMBDiagnosticTranslations(t *testing.T) {
	before := say.Chosen()
	t.Cleanup(func() { say.Use(before) })
	for _, lang := range []string{"en", "de", "it", "vi"} {
		for key := range errorMessages["en"] {
			if errorMessages[lang][key] == "" {
				t.Errorf("missing diagnostic translation %s/%s", lang, key)
			}
		}
		say.Use(lang)
		got := category(&smb2.ResponseError{Code: 0xc0000022}, "open")
		if !strings.Contains(got, errorMessages[lang]["denied"]) || !strings.Contains(got, errorMessages[lang]["stage_open"]) || !strings.Contains(got, "0xC0000022") || strings.Contains(got, "%!") {
			t.Fatal("broken localized diagnostic", lang)
		}
	}
}

func TestSMBBodyReportsPrematureEOFNotNormalEOF(t *testing.T) {
	for _, tc := range []struct {
		content   string
		expected  int64
		truncated bool
	}{{"video", 5, false}, {"vid", 5, true}} {
		var fault error
		b := &readBody{Reader: strings.NewReader(tc.content), remaining: tc.expected, close: func() {}, failed: func(e error) { fault = e }}
		data, e := io.ReadAll(b)
		if string(data) != tc.content || errors.Is(e, io.ErrUnexpectedEOF) != tc.truncated || errors.Is(fault, io.ErrUnexpectedEOF) != tc.truncated {
			t.Fatal("incorrect transfer termination classification")
		}
	}
}

type closingReader struct{ started, closed chan struct{} }

func (r closingReader) Read([]byte) (int, error) {
	close(r.started)
	<-r.closed
	return 0, net.ErrClosed
}
func TestSMBIntentionalCloseDoesNotPoisonPlaybackError(t *testing.T) {
	started, closed := make(chan struct{}), make(chan struct{})
	fault := make(chan error, 1)
	done := make(chan struct{})
	b := &readBody{Reader: closingReader{started, closed}, remaining: 10, close: func() { close(closed) }, failed: func(e error) { fault <- e }}
	go func() { _, _ = b.Read(make([]byte, 10)); close(done) }()
	<-started
	_ = b.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("close blocked")
	}
	select {
	case <-fault:
		t.Fatal("intentional body close recorded a playback failure")
	default:
	}
}
func TestSMBDeadlineBeforeContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if !errors.Is(deadlineError(ctx, time.Now().Add(-time.Millisecond), net.ErrClosed), context.DeadlineExceeded) {
		t.Fatal("socket deadline misclassified")
	}
	if !errors.Is(deadlineError(ctx, time.Now().Add(time.Second), syscall.ECONNREFUSED), syscall.ECONNREFUSED) {
		t.Fatal("ordinary error hidden")
	}
}

type diagnosticPlayer struct{ localvideo.Player }

func (diagnosticPlayer) Snapshot() urlplayer.Snapshot {
	return urlplayer.Snapshot{State: "error", Error: "private-password@private-host"}
}
func TestSMBDiagnosticReachesPlaybackHTTP(t *testing.T) {
	s := fixture(t)
	tr := NewTransport(s, nil)
	p := Wrap(diagnosticPlayer{}, s, tr)
	p.network = true
	message := category(&os.PathError{Path: "private-file", Err: &smb2.ResponseError{Code: 0xc000000d}}, "metadata")
	tr.fail(message)
	w := httptest.NewRecorder()
	localvideo.New(p).ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "STATUS_INVALID_PARAMETER") || !strings.Contains(w.Body.String(), "0xC000000D") {
		t.Fatal("diagnostic missing from playback API", w.Code)
	}
	if strings.Contains(w.Body.String(), "private-") {
		t.Fatal("playback API leaked underlying diagnostic")
	}
}

func TestSMBActualConnectionResetRetainsCause(t *testing.T) {
	listener, e := net.ListenTCP("tcp", &net.TCPAddr{IP: net.ParseIP("127.0.0.1")})
	if e != nil {
		t.Fatal(e)
	}
	defer listener.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		c, e := listener.AcceptTCP()
		if e != nil {
			return
		}
		defer c.Close()
		_ = c.SetDeadline(time.Now().Add(2 * time.Second))
		_ = c.SetLinger(0)
		_, _ = io.ReadFull(c, make([]byte, 4))
	}()
	s := fixture(t)
	pw := "synthetic-password"
	if e = s.Save(Update{Settings: Settings{Enabled: true, Host: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port, Share: "videos", Username: "reader"}, Password: &pw}); e != nil {
		t.Fatal(e)
	}
	tr := NewTransport(s, nil)
	req := httptest.NewRequest("GET", "https://smb.media.invalid/sample.mp4", nil)
	if _, e = tr.RoundTrip(req); e == nil {
		t.Fatal("reset connection succeeded")
	}
	if !strings.Contains(tr.Error(), "CONNECTION_RESET") {
		t.Fatal("wrapped socket cause lost", tr.Error())
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("test server did not close")
	}
}
