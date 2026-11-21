package userfiles

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// Handler must ONLY be mounted inside the owner-authenticated HTTPS Pages API.
// No Node/peer/anonymous route may expose this handler.
func (s *Store) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// Cookie auth is SameSite=Strict. Additionally reject cross-origin writes,
		// including authenticated requests from a different origin.
		if r.Method != "GET" && r.Method != "HEAD" {
			if origin := r.Header.Get("Origin"); origin != "" {
				u, err := url.Parse(origin)
				if err != nil || u.Scheme != "https" || u.Host != r.Host {
					fail(w, 403, "cross_origin")
					return
				}
			}
			if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
				fail(w, 403, "cross_origin")
				return
			}
		}
		path := r.URL.Path
		if path == "" || path == "/" {
			if r.Method != "GET" {
				fail(w, 405, "method_not_allowed")
				return
			}
			v, err := s.List()
			if err != nil {
				httpFail(w, err)
				return
			}
			reply(w, 200, v)
			return
		}
		name := strings.TrimPrefix(path, "/")
		if !ValidName(name) {
			fail(w, 400, "invalid_name")
			return
		}
		switch r.Method {
		case "GET", "HEAD":
			f, m, err := s.Open(name)
			if err != nil {
				httpFail(w, err)
				return
			}
			defer f.Close()
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
			w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
			http.ServeContent(w, r, name, m.Modified, f)
		case "PUT":
			if r.ContentLength < 0 {
				fail(w, 411, "length_required")
				return
			}
			// A hard deadline prevents an admitted but stalled upload monopolizing
			// the single transfer slot indefinitely.
			rc := http.NewResponseController(w)
			_ = rc.SetReadDeadline(time.Now().Add(5 * time.Minute))
			defer rc.SetReadDeadline(time.Time{})
			m, err := s.Upload(name, r.ContentLength, r.Body)
			if err != nil {
				httpFail(w, err)
				return
			}
			reply(w, 201, m)
		case "DELETE":
			if err := s.Delete(name); err != nil {
				httpFail(w, err)
				return
			}
			w.WriteHeader(204)
		default:
			fail(w, 405, "method_not_allowed")
		}
	})
}
func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, code string) {
	reply(w, status, map[string]string{"error": code})
}
func httpFail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, io.ErrUnexpectedEOF):
		fail(w, 400, "incomplete_upload")
	case errors.Is(err, ErrName):
		fail(w, 400, "invalid_name")
	case errors.Is(err, ErrExists):
		fail(w, 409, "already_exists")
	case errors.Is(err, ErrTooLarge):
		fail(w, 413, "file_too_large")
	case errors.Is(err, ErrFull), errors.Is(err, syscall.ENOSPC), errors.Is(err, syscall.EDQUOT):
		fail(w, 507, "storage_full")
	case errors.Is(err, ErrBusy):
		fail(w, 423, "transfer_busy")
	case errors.Is(err, fs.ErrNotExist):
		fail(w, 404, "not_found")
	case errors.Is(err, ErrBoundary), errors.Is(err, syscall.ELOOP):
		fail(w, 409, "unsafe_entry")
	default:
		fail(w, 500, "transfer_failed")
	}
}
