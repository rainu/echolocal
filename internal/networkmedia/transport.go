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
	"sync/atomic"
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
func (t *Transport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Hostname() != smburl.Host {
		return t.Base.RoundTrip(r)
	}
	if r.URL.Scheme != "https" || r.URL.Host != smburl.Host || r.Method != http.MethodGet || r.URL.RawQuery != "" || r.URL.User != nil || !smburl.Path(strings.TrimPrefix(r.URL.Path, "/")) {
		return nil, t.fail(smburl.ErrURL.Error())
	}
	cfg, e := t.Store.snapshot()
	if e != nil {
		return nil, t.fail(playbackDiagnostic("config", "config", "INVALID_SOURCE_SETTINGS"))
	}
	if !cfg.Enabled {
		return nil, t.fail(playbackDiagnostic("disabled", "config", "SOURCE_DISABLED"))
	}
	select {
	case t.slots <- struct{}{}:
	case <-r.Context().Done():
		return nil, r.Context().Err()
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	release := func() { cancel(); <-t.slots }
	deadline, _ := ctx.Deadline()
	conn, e := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)))
	if e != nil {
		e = deadlineError(ctx, deadline, e)
		release()
		if r.Context().Err() != nil {
			return nil, r.Context().Err()
		}
		return nil, t.fail(category(e, "connect"))
	}
	_ = conn.SetDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	closeConn := sync.OnceFunc(func() { stop(); _ = conn.Close(); release() })
	failed := func(e error, stage string) (*http.Response, error) {
		e = deadlineError(ctx, deadline, e)
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
	var fi os.FileInfo
	for i := range parts {
		fi, e = share.Lstat(strings.Join(parts[:i+1], "/"))
		if e != nil {
			return failed(e, "metadata")
		}
		if fi.Mode()&(os.ModeSymlink|os.ModeIrregular|os.ModeDevice|os.ModeNamedPipe|os.ModeSocket) != 0 {
			closeConn()
			return nil, t.fail(playbackDiagnostic("link", "metadata", "LINK_OR_SPECIAL_FILE"))
		}
	}
	// Lstat already obtained size and attributes from SMB CREATE, without a
	// large QUERY_INFO request. The pinned client's File.Stat ignores a reduced
	// credit grant for that query, which strict SMB servers can disconnect.
	// Stable regular files are required; concurrent path/content changes remain
	// unsupported, just as they are for range playback in general.
	if !fi.Mode().IsRegular() {
		closeConn()
		return nil, t.fail(playbackDiagnostic("folder", "metadata", "NOT_A_REGULAR_FILE"))
	}
	f, e := share.Open(name)
	if e != nil {
		return failed(e, "open")
	}
	if strings.ReplaceAll(f.Name(), "\\", "/") != name {
		closeConn()
		return nil, t.fail(playbackDiagnostic("path", "open", "FILE_PATH_MISMATCH"))
	}
	size := fi.Size()
	from, to, e := byteRange(r.Header.Get("Range"), size)
	if e != nil {
		kind, code := "range", "RANGE_NOT_SATISFIABLE"
		if size <= 0 {
			kind, code = "empty", "EMPTY_VIDEO_FILE"
		}
		t.fail(playbackDiagnostic(kind, "stat", code))
		closeConn()
		return &http.Response{StatusCode: 416, Header: http.Header{"Content-Range": {fmt.Sprintf("bytes */%d", size)}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	}
	header := http.Header{"Accept-Ranges": {"bytes"}, "Content-Length": {strconv.FormatInt(to-from+1, 10)}, "Content-Type": {mime.TypeByExtension(strings.ToLower(path.Ext(name)))}}
	status := 200
	if r.Header.Get("Range") != "" {
		status = 206
		header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", from, to, size))
	}
	body := &readBody{remaining: to - from + 1, Reader: io.NewSectionReader(f, from, to-from+1), close: closeConn, failed: func(e error) {
		if r.Context().Err() == nil {
			t.fail(category(deadlineError(ctx, deadline, e), "read"))
		}
	}}
	return &http.Response{StatusCode: status, Header: header, ContentLength: to - from + 1, Body: body, Request: r}, nil
}

type readBody struct {
	remaining int64
	closed    atomic.Bool
	io.Reader
	close  func()
	failed func(error)
}

func (b *readBody) Read(p []byte) (int, error) {
	n, e := b.Reader.Read(p)
	b.remaining -= int64(n)
	if e == io.EOF && b.remaining > 0 {
		e = io.ErrUnexpectedEOF
	}
	if e != nil && e != io.EOF && !b.closed.Load() {
		b.failed(e)
	}
	return n, e
}
func (b *readBody) Close() error { b.closed.Store(true); b.close(); return nil }
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
