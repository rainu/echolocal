package filesmedia

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Transport serves exactly one pinned inode to an in-process HTTP client.
// It has NO listener and NO network fallback. Playlists/external references
// cannot turn local media into network requests or arbitrary file reads.
type Transport struct {
	mu   sync.Mutex
	file *os.File
	size int64
	kind string
}

const URL = "http://files.invalid/media"

func NewTransport(f *os.File, name string) (*Transport, error) {
	st, e := f.Stat()
	if e != nil || !st.Mode().IsRegular() {
		return nil, ErrPreview
	}
	kind := mime.TypeByExtension(strings.ToLower(filepath.Ext(name)))
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mp3":
		kind = "audio/mpeg"
	case ".m4a":
		kind = "audio/mp4"
	case ".mkv":
		kind = "video/x-matroska"
	case ".wav":
		kind = "audio/wav"
	case ".flac":
		kind = "audio/flac"
	}
	return &Transport{file: f, size: st.Size(), kind: kind}, nil
}
func (t *Transport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.file == nil {
		return nil
	}
	e := t.file.Close()
	t.file = nil
	return e
}
func (t *Transport) RoundTrip(r *http.Request) (*http.Response, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if r.URL.String() != URL || r.Method != "GET" || t.file == nil || r.Context().Err() != nil {
		return nil, errors.New("local_media_unavailable")
	}
	start, end := int64(0), t.size-1
	status := 200
	if raw := r.Header.Get("Range"); raw != "" {
		if !strings.HasPrefix(raw, "bytes=") || strings.Contains(raw, ",") {
			return nil, ErrPreview
		}
		a, b, ok := strings.Cut(strings.TrimPrefix(raw, "bytes="), "-")
		if !ok {
			return nil, ErrPreview
		}
		var e error
		start, e = strconv.ParseInt(a, 10, 64)
		if e != nil {
			return nil, ErrPreview
		}
		if b != "" {
			end, e = strconv.ParseInt(b, 10, 64)
			if e != nil {
				return nil, ErrPreview
			}
			end = min(end, t.size-1)
		}
		status = 206
	}
	h := http.Header{"Content-Type": {t.kind}, "Accept-Ranges": {"bytes"}}
	if start < 0 || start >= t.size || end < start {
		return &http.Response{StatusCode: 416, Header: h, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	}
	n := end - start + 1
	h.Set("Content-Length", strconv.FormatInt(n, 10))
	if status == 206 {
		h.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, t.size))
	}
	return &http.Response{StatusCode: status, Header: h, Body: io.NopCloser(io.NewSectionReader(t.file, start, n)), ContentLength: n, Request: r}, nil
}
