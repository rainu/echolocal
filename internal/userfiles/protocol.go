package userfiles

import (
	"bytes"
	"context"
	"encoding/xml"
	"golang.org/x/net/webdav"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

// Protocol reuses the tested RFC4918 conditions, XML and shared/exclusive lock
// implementation with another trusted filesystem. It has no managed-store quota
// or whole-tree snapshot: the supplied backend owns storage and atomic writes.
type Protocol struct {
	FS        webdav.FileSystem
	Canonical func(string) (string, error)
	Begin     func(*http.Request) (*http.Request, func(bool) error, error)
	Error     func(context.Context) error
	Failure   func(http.ResponseWriter, error)
	locks     *davLocks
}

func NewProtocol(fs webdav.FileSystem) *Protocol { return &Protocol{FS: fs, locks: newDAVLocks()} }
func (p *Protocol) canonical(n string) (string, error) {
	if p.Canonical != nil {
		return p.Canonical(n)
	}
	return path.Clean("/" + n), nil
}
func (p *Protocol) Acquire(n string) (func(), error) {
	n, e := p.canonical(n)
	if e != nil {
		return nil, e
	}
	s := &Store{dav: &davState{locks: p.locks}}
	return s.apiLock(strings.TrimPrefix(n, "/"))
}
func (p *Protocol) Discovery(n string) []byte {
	n, e := p.canonical(n)
	if e != nil {
		return nil
	}
	return p.locks.discovery(n)
}
func (p *Protocol) failure(w http.ResponseWriter, e error) {
	if p.Failure != nil {
		p.Failure(w, e)
	} else {
		httpFail(w, e)
	}
}
func (p *Protocol) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	if !sameOrigin(r) {
		w.WriteHeader(403)
		return
	}
	n := strings.TrimPrefix(r.URL.Path, "/")
	if strings.ContainsAny(n, "\x00\\") || len(n) > 4096 {
		w.WriteHeader(400)
		return
	}
	for _, c := range strings.Split(n, "/") {
		if c == "." || c == ".." {
			w.WriteHeader(400)
			return
		}
	}
	mutation := r.Method != "GET" && r.Method != "HEAD" && r.Method != "PROPFIND" && r.Method != "OPTIONS"
	if n == "" && (r.Method == "PUT" || r.Method == "DELETE" || r.Method == "MOVE") {
		w.WriteHeader(403)
		return
	}
	if r.Method == "PROPFIND" && (r.Header.Get("Depth") == "" || r.Header.Get("Depth") == "infinity") {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(403)
		io.WriteString(w, `<D:error xmlns:D="DAV:"><D:propfind-finite-depth/></D:error>`)
		return
	}
	if r.Method == "COPY" || r.Method == "MOVE" {
		u, e := url.Parse(r.Header.Get("Destination"))
		if e != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "" && u.Scheme != "https") || (u.Host != "" && !sameHTTPSAuthority(u.Host, r.Host)) || !strings.HasPrefix(u.Path, DAVPrefix+"/") {
			w.WriteHeader(502)
			return
		}
		dn := strings.TrimPrefix(u.Path, DAVPrefix+"/")
		a, e := p.canonical(n)
		if e != nil {
			p.failure(w, e)
			return
		}
		b, e := p.canonical(dn)
		if e != nil {
			p.failure(w, e)
			return
		}
		if a == "/" || b == "/" || a == b || strings.HasPrefix(b, a+"/") || strings.HasPrefix(a, b+"/") {
			w.WriteHeader(403)
			return
		}
		if v := r.Header.Get("Overwrite"); v != "" && v != "T" && v != "F" {
			w.WriteHeader(400)
			return
		}
	}
	if r.Method == "MOVE" && r.Header.Get("Overwrite") == "" {
		r = r.Clone(r.Context())
		r.Header.Set("Overwrite", "T")
	}
	var finish func(bool) error
	if p.Begin != nil {
		var e error
		r, finish, e = p.Begin(r)
		if e != nil {
			p.failure(w, e)
			return
		}
		defer func() {
			if finish != nil {
				finish(false)
			}
		}()
	}
	shared := false
	if r.Method == "PROPFIND" || r.Method == "PROPPATCH" || r.Method == "LOCK" {
		b, e := io.ReadAll(io.LimitReader(r.Body, controlLimit+1))
		if e != nil {
			p.failure(w, e)
			return
		}
		if len(b) > controlLimit {
			w.WriteHeader(413)
			return
		}
		if len(b) > 0 {
			if e = validateDAVXML(b); e != nil {
				w.WriteHeader(400)
				return
			}
			if r.Method == "LOCK" {
				b, shared, e = lockScopeXML(b, "shared", "exclusive")
				if e != nil {
					w.WriteHeader(400)
					return
				}
			}
		}
		r = r.Clone(r.Context())
		r.Body = io.NopCloser(bytes.NewReader(b))
		r.ContentLength = int64(len(b))
	}
	body := &davBody{ReadCloser: r.Body}
	r = r.Clone(r.Context())
	r.Body = body
	s := &Store{dav: &davState{locks: p.locks}, statHook: func(name string) (os.FileInfo, error) {
		return p.FS.Stat(r.Context(), "/"+strings.TrimPrefix(name, "/"))
	}, canonicalHook: p.canonical}
	canonical, e := p.canonical(n)
	if e != nil {
		p.failure(w, e)
		return
	}
	if r.Method == "UNLOCK" {
		t := strings.Trim(r.Header.Get("Lock-Token"), "<>")
		p.locks.mu.Lock()
		v, ok := p.locks.active[t]
		p.locks.mu.Unlock()
		if ok && v.details.Root != canonical {
			w.WriteHeader(409)
			return
		}
	}
	if mutation && (r.Method == "PUT" || r.Header.Get("If-Match") != "" || r.Header.Get("If-None-Match") != "" || r.Header.Get("If-Unmodified-Since") != "") && !s.preconditions(w, r, n) {
		return
	}
	if code, e := s.checkDAVIf(r, n); e != nil {
		w.WriteHeader(code)
		return
	}
	// The complete If header was evaluated above. The standard handler's own
	// per-list/two-path shortcut must not reject equivalent tagged authorities.
	if r.Header.Get("If") != "" && mutation && r.Method != "LOCK" && r.Method != "UNLOCK" {
		r.Header.Set("If", "(Not <DAV:no-lock>)")
	}
	if r.Method == "COPY" || r.Method == "MOVE" {
		u, _ := url.Parse(r.Header.Get("Destination"))
		if u != nil && u.Host != "" {
			u.Host = r.Host
			r.Header.Set("Destination", u.String())
		}
	}
	r.URL.Path = DAVPrefix + "/" + n
	r.URL.RawPath = ""
	h := &webdav.Handler{Prefix: DAVPrefix, FileSystem: p.FS, LockSystem: requestLocks{davLocks: p.locks, s: s, shared: shared, method: r.Method, verified: r.Header.Get("If") != ""}}
	rc := http.NewResponseController(w)
	if mutation {
		_ = rc.SetReadDeadline(time.Now().Add(5 * time.Minute))
		defer rc.SetReadDeadline(time.Time{})
	}
	if r.Method == "GET" || r.Method == "HEAD" {
		if fi, e := p.FS.Stat(r.Context(), "/"+n); e == nil && !fi.IsDir() && !fi.Mode().IsRegular() {
			w.WriteHeader(403)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": path.Base(n)}))
		h.ServeHTTP(w, r)
		return
	}
	out := &davResponse{h: make(http.Header)}
	committed := false
	defer func() {
		if !committed && r.Method == "LOCK" {
			if t := strings.Trim(out.h.Get("Lock-Token"), "<>"); t != "" {
				p.locks.Unlock(time.Now(), t)
			}
		}
	}()
	h.ServeHTTP(out, r)
	if p.Error != nil {
		if e := p.Error(r.Context()); e != nil {
			p.failure(w, e)
			return
		}
	}
	if out.err != nil {
		p.failure(w, out.err)
		return
	}
	success := out.code == 0 || out.code < 300
	if success {
		if body.err != nil || (r.Method == "PUT" && r.ContentLength >= 0 && body.n != r.ContentLength) {
			p.failure(w, io.ErrUnexpectedEOF)
			return
		}
		if e := r.Context().Err(); e != nil {
			p.failure(w, e)
			return
		}
		if finish != nil {
			f := finish
			finish = nil
			if e := f(true); e != nil {
				p.failure(w, e)
				return
			}
		}
		committed = true
		if r.Method == "PUT" && r.Context().Value(existingKey{}) == true {
			out.code = 204
		}
		if r.Method == "DELETE" || r.Method == "MOVE" {
			p.locks.deleteUnder(canonical)
		}
	}
	if r.Method == "PROPFIND" && out.code == 207 {
		out.b = *bytes.NewBuffer(patchSupportedLocks(out.b.Bytes()))
	}
	if success && r.Method == "LOCK" {
		token := strings.Trim(out.h.Get("Lock-Token"), "<>")
		if token == "" {
			p.locks.mu.Lock()
			for t := range p.locks.active {
				if strings.Contains(r.Header.Get("If"), "<"+t+">") {
					token = t
					break
				}
			}
			p.locks.mu.Unlock()
		}
		if token != "" {
			out.b.Reset()
			out.b.WriteString(xml.Header + `<D:prop xmlns:D="DAV:"><D:lockdiscovery>`)
			out.b.Write(p.locks.discoveryToken(canonical, token))
			out.b.WriteString(`</D:lockdiscovery></D:prop>`)
		}
	}
	out.send(w)
}

func sameHTTPSAuthority(a, b string) bool {
	x := url.URL{Scheme: "https", Host: a}
	y := url.URL{Scheme: "https", Host: b}
	px, py := x.Port(), y.Port()
	if px == "" {
		px = "443"
	}
	if py == "" {
		py = "443"
	}
	return strings.EqualFold(x.Hostname(), y.Hostname()) && px == py
}
