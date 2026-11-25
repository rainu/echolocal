package networkmedia

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	smb2 "github.com/cloudsoda/go-smb2"
	"github.com/ygelfand/libcountertop/pkg/media/smburl"
)

type Transport struct {
	Store *Store
	Base  http.RoundTripper
	slots chan struct{}
	mu    sync.Mutex
	last  string
}

func NewTransport(s *Store, base http.RoundTripper) *Transport {
	if base == nil {
		base = http.DefaultTransport
	}
	return &Transport{Store: s, Base: base, slots: make(chan struct{}, 4)}
}
func (t *Transport) Error() string { t.mu.Lock(); defer t.mu.Unlock(); return t.last }
func (t *Transport) ClearError()   { t.mu.Lock(); t.last = ""; t.mu.Unlock() }
func (t *Transport) fail(message string) error {
	t.mu.Lock()
	t.last = message
	t.mu.Unlock()
	return errors.New(message)
}
func category(e error, stage string) string {
	if stage == "mount" {
		return "SMB share unavailable or access denied. Check the configured share name and read-only account permissions."
	}
	if stage == "auth" {
		return "SMB sign-in or signing negotiation failed. Check the account, password, domain, and SMB 2/3 policy."
	}
	var r *smb2.ResponseError
	if errors.As(e, &r) {
		switch r.Code {
		case 0xc000006d, 0xc000006a, 0xc0000064, 0xc0000072, 0xc0000234:
			return "SMB sign-in failed. Check the account, password, domain, and server NTLM policy."
		case 0xc0000022:
			return "SMB access denied. Check the read-only account permissions and signing policy."
		case 0xc0000034, 0xc000003a, 0xc000000f:
			return "Network video not found. Check the path within the configured share."
		case 0xc00000cc, 0xc00000be:
			return "SMB share unavailable. Check the configured share name."
		}
	}
	if errors.Is(e, os.ErrNotExist) {
		return "Network video not found. Check the path within the configured share."
	}
	if stage == "connect" {
		return "SMB server unavailable or incompatible. Check connectivity and SMB 2/3 support, then retry."
	}
	return "SMB read interrupted. Restore connectivity and load the video again; playback does not resume automatically."
}
func (t *Transport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Hostname() != smburl.Host {
		return t.Base.RoundTrip(r)
	}
	if r.URL.Scheme != "https" || r.URL.Host != smburl.Host || r.Method != http.MethodGet || r.URL.RawQuery != "" || r.URL.User != nil || !smburl.Path(strings.TrimPrefix(r.URL.Path, "/")) {
		return nil, t.fail(smburl.ErrURL.Error())
	}
	cfg, e := t.Store.snapshot()
	if e != nil {
		return nil, e
	}
	if !cfg.Enabled {
		return nil, t.fail("Configure and enable the SMB media source first")
	}
	select {
	case t.slots <- struct{}{}:
	case <-r.Context().Done():
		return nil, r.Context().Err()
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	release := func() { cancel(); <-t.slots }
	conn, e := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)))
	if e != nil {
		release()
		if r.Context().Err() != nil {
			return nil, r.Context().Err()
		}
		return nil, t.fail(category(e, "connect"))
	}
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	closeConn := sync.OnceFunc(func() { stop(); _ = conn.Close(); release() })
	failed := func(e error, stage string) (*http.Response, error) {
		closeConn()
		if r.Context().Err() != nil {
			return nil, r.Context().Err()
		}
		return nil, t.fail(category(e, stage))
	}
	d := smb2.Dialer{MaxCreditBalance: 16, Negotiator: smb2.Negotiator{RequireMessageSigning: true}, Initiator: &smb2.NTLMInitiator{User: cfg.Username, Password: cfg.Password, Domain: cfg.Domain}}
	session, e := d.DialConn(ctx, conn, cfg.Host)
	if e != nil {
		return failed(e, "auth")
	}
	share, e := session.WithContext(ctx).Mount(cfg.Share)
	if e != nil {
		return failed(e, "mount")
	}
	share = share.WithContext(ctx)
	name := strings.TrimPrefix(r.URL.Path, "/")
	// Reject server-advertised reparse points in every component. Never resolve
	// DFS referrals or connect to a second share/server. The server must itself
	// enforce its share boundary (including server-side Unix symlinks).
	parts := strings.Split(name, "/")
	for i := range parts {
		fi, e := share.Lstat(strings.Join(parts[:i+1], "/"))
		if e != nil {
			return failed(e, "open")
		}
		if fi.Mode()&(os.ModeSymlink|os.ModeIrregular|os.ModeDevice|os.ModeNamedPipe|os.ModeSocket) != 0 {
			closeConn()
			return nil, t.fail("SMB links and special files are not supported. Select a regular file within the share.")
		}
	}
	f, e := share.Open(name)
	if e != nil {
		return failed(e, "open")
	}
	fi, e := f.Stat()
	if e != nil {
		return failed(e, "open")
	}
	if !fi.Mode().IsRegular() || strings.ReplaceAll(f.Name(), "\\", "/") != name {
		closeConn()
		return nil, t.fail("Select a regular video file within the configured SMB share")
	}
	size := fi.Size()
	from, to, e := byteRange(r.Header.Get("Range"), size)
	if e != nil {
		closeConn()
		return &http.Response{StatusCode: 416, Header: http.Header{"Content-Range": {fmt.Sprintf("bytes */%d", size)}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	}
	header := http.Header{"Accept-Ranges": {"bytes"}, "Content-Length": {strconv.FormatInt(to-from+1, 10)}, "Content-Type": {mime.TypeByExtension(strings.ToLower(path.Ext(name)))}}
	status := 200
	if r.Header.Get("Range") != "" {
		status = 206
		header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", from, to, size))
	}
	body := &readBody{Reader: io.NewSectionReader(f, from, to-from+1), close: closeConn, failed: func(e error) {
		if r.Context().Err() == nil {
			t.fail(category(e, "read"))
		}
	}}
	return &http.Response{StatusCode: status, Header: header, ContentLength: to - from + 1, Body: body, Request: r}, nil
}

type readBody struct {
	io.Reader
	close  func()
	failed func(error)
}

func (b *readBody) Read(p []byte) (int, error) {
	n, e := b.Reader.Read(p)
	if e != nil && e != io.EOF {
		b.failed(e)
	}
	return n, e
}
func (b *readBody) Close() error { b.close(); return nil }
func byteRange(raw string, size int64) (int64, int64, error) {
	if size <= 0 {
		return 0, 0, errors.New("empty")
	}
	if raw == "" {
		return 0, size - 1, nil
	}
	if !strings.HasPrefix(raw, "bytes=") || strings.Contains(raw, ",") {
		return 0, 0, errors.New("range")
	}
	a, b, ok := strings.Cut(strings.TrimPrefix(raw, "bytes="), "-")
	if !ok || a == "" {
		return 0, 0, errors.New("range")
	}
	from, e := strconv.ParseInt(a, 10, 64)
	if e != nil || from < 0 || from >= size {
		return 0, 0, errors.New("range")
	}
	to := size - 1
	if b != "" {
		to, e = strconv.ParseInt(b, 10, 64)
		if e != nil || to < from {
			return 0, 0, errors.New("range")
		}
		to = min(to, size-1)
	}
	return from, to, nil
}
