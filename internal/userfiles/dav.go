package userfiles

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"syscall"
	"time"

	"golang.org/x/net/webdav"
)

const DAVPrefix = "/api/v1/webdav"
const propLimit = 1 << 20
const controlLimit = 64 << 10

func randomID() string {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b[:])
}

type davState struct {
	locks *davLocks
	props map[string][]webdav.Property
}

func newDAVState() *davState {
	return &davState{locks: newDAVLocks(), props: make(map[string][]webdav.Property)}
}
func (s *Store) loadProps() error {
	dir, e := s.root.Open(".")
	if e != nil {
		return e
	}
	fd, e := syscall.Openat(int(dir.Fd()), ".dav-properties", syscall.O_NOFOLLOW|syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	dir.Close()
	var f *os.File
	if e == nil {
		f = os.NewFile(uintptr(fd), ".dav-properties")
	}
	if errors.Is(e, fs.ErrNotExist) {
		return nil
	}
	if e != nil {
		return e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return e
	}
	if !st.Mode().IsRegular() || st.Size() > propLimit {
		return ErrBoundary
	}
	if e = json.NewDecoder(io.LimitReader(f, propLimit+1)).Decode(&s.dav.props); e != nil {
		return e
	}
	if s.dav.props == nil {
		s.dav.props = make(map[string][]webdav.Property)
	}
	return nil
}
func (s *Store) saveProps() error {
	b, e := json.Marshal(s.dav.props)
	if e != nil {
		return e
	}
	if len(b) > propLimit {
		return ErrFull
	}
	temp := ".upload-" + randomID()
	defer s.root.Remove(temp)
	f, e := s.root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	_, e = f.Write(b)
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	return s.root.Rename(temp, ".dav-properties")
}

func (l *davLocks) discovery(n string) []byte { return l.discoveryToken(n, "") }
func (l *davLocks) discoveryToken(n, only string) []byte {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.prune(now)
	var b bytes.Buffer
	esc := func(s string) { xml.EscapeText(&b, []byte(s)) }
	for t, v := range l.active {
		if only != "" && only != t {
			continue
		}
		root := path.Clean("/" + v.details.Root)
		if n != root && (v.details.ZeroDepth || !strings.HasPrefix(n, strings.TrimSuffix(root, "/")+"/")) {
			continue
		}
		b.WriteString(`<D:activelock xmlns:D="DAV:"><D:locktype><D:write/></D:locktype><D:lockscope>`)
		if v.shared {
			b.WriteString(`<D:shared/>`)
		} else {
			b.WriteString(`<D:exclusive/>`)
		}
		b.WriteString(`</D:lockscope><D:depth>`)
		if v.details.ZeroDepth {
			b.WriteString("0")
		} else {
			b.WriteString("infinity")
		}
		b.WriteString(`</D:depth><D:owner>`)
		b.WriteString(v.details.OwnerXML)
		b.WriteString(`</D:owner><D:timeout>`)
		fmt.Fprintf(&b, "Second-%d", max(0, int(time.Until(v.expires).Seconds())))
		b.WriteString(`</D:timeout><D:locktoken><D:href>`)
		esc(t)
		b.WriteString(`</D:href></D:locktoken><D:lockroot><D:href>`)
		esc((&url.URL{Path: DAVPrefix + root}).EscapedPath())
		b.WriteString(`</D:href></D:lockroot></D:activelock>`)
	}
	return b.Bytes()
}
func (s *Store) apiLock(n string) (func(), error) {
	t, e := s.dav.locks.Create(time.Now(), webdav.LockDetails{Root: "/" + n, Duration: time.Minute, ZeroDepth: true})
	if e != nil {
		return nil, ErrBusy
	}
	return func() { s.dav.locks.Unlock(time.Now(), t) }, nil
}

type davRequestState struct {
	s      *Store
	stages []*stage
	err    error
	body   *davBody
}
type stateKey struct{}

func requestState(ctx context.Context) *davRequestState {
	return ctx.Value(stateKey{}).(*davRequestState)
}

type davBody struct {
	io.ReadCloser
	n   int64
	err error
}

func (b *davBody) Read(p []byte) (int, error) {
	n, e := b.ReadCloser.Read(p)
	b.n += int64(n)
	if e != nil && e != io.EOF {
		b.err = e
	}
	return n, e
}

// Only control responses are buffered. File downloads remain streamed.
type davResponse struct {
	h    http.Header
	code int
	b    bytes.Buffer
	err  error
}

func (w *davResponse) Header() http.Header { return w.h }
func (w *davResponse) WriteHeader(c int) {
	if w.code == 0 {
		w.code = c
	}
}
func (w *davResponse) Write(b []byte) (int, error) {
	if w.code == 0 {
		w.code = 200
	}
	if w.b.Len()+len(b) > 4<<20 {
		w.err = ErrTooLarge
		return 0, w.err
	}
	return w.b.Write(b)
}
func (w *davResponse) send(dst http.ResponseWriter) {
	for k, v := range w.h {
		dst.Header()[k] = v
	}
	if w.code == 0 {
		w.code = 200
	}
	dst.WriteHeader(w.code)
	dst.Write(w.b.Bytes())
}

func (s *Store) DAVHandler() http.Handler {
	return http.HandlerFunc(s.serveDAV)
}
func (s *Store) serveDAV(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	if !sameOrigin(r) {
		fail(w, 403, "cross_origin")
		return
	}
	n := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if n != "" && !ValidPath(n) {
		fail(w, 400, "invalid_name")
		return
	}
	if e := s.init(); e != nil {
		httpFail(w, e)
		return
	}
	mutation := r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" && r.Method != "PROPFIND"
	if mutation {
		if !s.upload.TryLock() {
			httpFail(w, ErrBusy)
			return
		}
		defer s.upload.Unlock()
	}
	// The root is permanent. Root locks/properties are supported, not destruction.
	if n == "" && (r.Method == "DELETE" || r.Method == "MOVE" || r.Method == "PUT") {
		fail(w, 403, "protected_collection")
		return
	}
	// Reject recursive self-copies BEFORE a handler can remove the destination.
	if r.Method == "COPY" || r.Method == "MOVE" {
		u, e := url.Parse(r.Header.Get("Destination"))
		if e != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "" && u.Scheme != "https") || (u.Host != "" && u.Host != r.Host) || !strings.HasPrefix(u.Path, DAVPrefix+"/") {
			fail(w, 502, "invalid_destination")
			return
		}
		dn := strings.TrimSuffix(strings.TrimPrefix(u.Path, DAVPrefix+"/"), "/")
		if !ValidPath(dn) {
			fail(w, 400, "invalid_destination")
			return
		}
		if dn == n || strings.HasPrefix(dn, n+"/") || strings.HasPrefix(n, dn+"/") {
			fail(w, 403, "recursive_destination")
			return
		}
		if v := r.Header.Get("Overwrite"); v != "" && v != "T" && v != "F" {
			fail(w, 400, "invalid_overwrite")
			return
		}
	}
	if r.Method == "UNLOCK" {
		token := strings.Trim(r.Header.Get("Lock-Token"), "<>")
		s.dav.locks.mu.Lock()
		v, ok := s.dav.locks.active[token]
		s.dav.locks.mu.Unlock()
		if ok && v.details.Root != path.Clean("/"+n) {
			w.WriteHeader(409)
			return
		}
	}
	rc := http.NewResponseController(w)
	if mutation {
		_ = rc.SetReadDeadline(time.Now().Add(5 * time.Minute))
		defer rc.SetReadDeadline(time.Time{})
	}
	shared := false
	if r.Method == "PROPFIND" || r.Method == "PROPPATCH" || r.Method == "LOCK" {
		control, e := io.ReadAll(io.LimitReader(r.Body, controlLimit+1))
		if e != nil {
			httpFail(w, e)
			return
		}
		if len(control) > controlLimit {
			w.WriteHeader(413)
			return
		}
		if len(control) > 0 {
			if e = validateDAVXML(control); e != nil {
				fail(w, 400, "invalid_xml_namespace")
				return
			}
			if r.Method == "LOCK" {
				control, shared, e = lockScopeXML(control, "shared", "exclusive")
				if e != nil {
					fail(w, 400, "invalid_lock_xml")
					return
				}
			}
		}
		r = r.Clone(r.Context())
		r.Body = io.NopCloser(bytes.NewReader(control))
		r.ContentLength = int64(len(control))
	}
	body := &davBody{ReadCloser: r.Body}
	st := &davRequestState{s: s, body: body}
	r = r.Clone(context.WithValue(r.Context(), stateKey{}, st))
	r.Body = body
	if r.Method != "PUT" {
		r.Body = http.MaxBytesReader(w, r.Body, controlLimit)
	} else if r.ContentLength > s.maxFile {
		httpFail(w, ErrTooLarge)
		return
	}
	r.URL.Path = DAVPrefix + "/" + n
	if strings.HasSuffix(r.RequestURI, "/") && n != "" {
		r.URL.Path += "/"
	}
	r.URL.RawPath = ""
	// x/net's MOVE default differs from RFC 4918 section 10.6 (default T).
	if r.Method == "MOVE" && r.Header.Get("Overwrite") == "" {
		r.Header.Set("Overwrite", "T")
	}
	if mutation && (r.Method == "PUT" || r.Header.Get("If-Match") != "" || r.Header.Get("If-None-Match") != "" || r.Header.Get("If-Unmodified-Since") != "") && !s.preconditions(w, r, n) {
		return
	}
	var transaction *treeTransaction
	if mutation {
		var e error
		transaction, e = s.beginTree(r.Context())
		if e != nil {
			httpFail(w, e)
			return
		}
		defer transaction.abort()
		s = transaction.work
		st.s = s
	}
	if code, e := s.checkDAVIf(r, n); e != nil {
		w.WriteHeader(code)
		return
	}
	h := &webdav.Handler{Prefix: DAVPrefix, FileSystem: davFS{s}, LockSystem: requestLocks{davLocks: s.dav.locks, s: s, shared: shared, method: r.Method, verified: r.Header.Get("If") != ""}}
	defer func() {
		for _, f := range st.stages {
			f.abort()
		}
	}()
	if r.Method == "GET" || r.Method == "HEAD" {
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": path.Base(n)}))
		h.ServeHTTP(w, r)
		return
	}
	out := &davResponse{h: make(http.Header)}
	defer func() {
		if transaction != nil && !transaction.committed && r.Method == "LOCK" {
			token := strings.Trim(out.h.Get("Lock-Token"), "<>")
			if token != "" {
				s.dav.locks.Unlock(time.Now(), token)
			}
		}
	}()
	h.ServeHTTP(out, r)
	if st.err != nil {
		httpFail(w, st.err)
		return
	}
	if out.err != nil {
		httpFail(w, out.err)
		return
	}
	if out.code == 0 || out.code < 300 {
		if body.err != nil || (r.Method == "PUT" && r.ContentLength >= 0 && body.n != r.ContentLength) {
			httpFail(w, io.ErrUnexpectedEOF)
			return
		}
		if e := r.Context().Err(); e != nil {
			httpFail(w, e)
			return
		}
		// Validate aggregate growth before publishing ANY staged COPY/upload file.
		s.mu.Lock()
		v, e := s.view()
		growth := int64(0)
		count := s.entryCount
		if e == nil {
			for _, f := range st.stages {
				old, se := s.statTree(f.name)
				if se == nil {
					growth -= old.Size()
				} else if errors.Is(se, fs.ErrNotExist) {
					count++
				} else {
					e = se
					break
				}
				growth += f.count
			}
		}
		if e == nil && (v.Used+growth > s.budget || count > MaxFiles) {
			e = ErrFull
		}
		s.mu.Unlock()
		if e != nil {
			httpFail(w, e)
			return
		}
		for _, f := range st.stages {
			if e = f.finish(); e != nil {
				httpFail(w, e)
				return
			}
		}
		if r.Method == "PUT" {
			if stBefore := r.Context().Value(existingKey{}); stBefore == true {
				out.code = 204
			}
		}
	}
	if (out.code == 0 || out.code < 300) && transaction != nil {
		if e := transaction.commit(r.Context()); e != nil {
			httpFail(w, e)
			return
		}
	}
	if (r.Method == "DELETE" || r.Method == "MOVE") && (out.code == 0 || out.code < 300) {
		s.dav.locks.deleteUnder(n)
	}
	if r.Method == "PROPFIND" && out.code == 207 {
		out.b = *bytes.NewBuffer(patchSupportedLocks(out.b.Bytes()))
	}
	if r.Method == "LOCK" && (out.code == 0 || out.code < 300) {
		token := strings.Trim(out.h.Get("Lock-Token"), "<>")
		if token == "" {
			s.dav.locks.mu.Lock()
			for t := range s.dav.locks.active {
				if strings.Contains(r.Header.Get("If"), "<"+t+">") {
					token = t
					break
				}
			}
			s.dav.locks.mu.Unlock()
		}
		if token != "" {
			out.b.Reset()
			out.b.WriteString(xml.Header + `<D:prop xmlns:D="DAV:"><D:lockdiscovery>`)
			out.b.Write(s.dav.locks.discoveryToken(path.Clean("/"+n), token))
			out.b.WriteString(`</D:lockdiscovery></D:prop>`)
		}
	}
	out.send(w)
}

type existingKey struct{}

func (s *Store) preconditions(w http.ResponseWriter, r *http.Request, n string) bool {
	s.mu.Lock()
	fi, e := s.statTree(n)
	s.mu.Unlock()
	exists := e == nil
	etag := ""
	if exists {
		etag = fmt.Sprintf(`"%x%x"`, fi.ModTime().UnixNano(), fi.Size())
	}
	match := func(v string, weak bool) bool {
		for _, x := range strings.Split(v, ",") {
			x = strings.TrimSpace(x)
			if weak {
				x = strings.TrimPrefix(x, "W/")
			}
			if x == etag && etag != "" {
				return true
			}
		}
		return false
	}
	if v := r.Header.Get("If-Match"); v != "" && ((v == "*" && !exists) || (v != "*" && !match(v, false))) {
		w.WriteHeader(412)
		return false
	}
	if v := r.Header.Get("If-None-Match"); v != "" && ((v == "*" && exists) || match(v, true)) {
		w.WriteHeader(412)
		return false
	}
	if v := r.Header.Get("If-Unmodified-Since"); v != "" && exists && r.Header.Get("If-Match") == "" {
		if t, e := http.ParseTime(v); e == nil && fi.ModTime().Truncate(time.Second).After(t) {
			w.WriteHeader(412)
			return false
		}
	}
	if exists {
		*r = *r.WithContext(context.WithValue(r.Context(), existingKey{}, true))
	}
	return true
}

type davFS struct{ s *Store }

func cleanDAV(n string) (string, error) {
	n = strings.TrimSuffix(strings.TrimPrefix(n, "/"), "/")
	if n == "" {
		return ".", nil
	}
	if !ValidPath(n) {
		return "", os.ErrPermission
	}
	return n, nil
}
func (d davFS) Stat(ctx context.Context, n string) (os.FileInfo, error) {
	n, e := cleanDAV(n)
	if e != nil {
		return nil, e
	}
	for _, f := range requestState(ctx).stages {
		if f.name == n {
			return f.File.Stat()
		}
	}
	d.s.mu.Lock()
	defer d.s.mu.Unlock()
	return d.s.statTree(n)
}
func (d davFS) OpenFile(ctx context.Context, n string, flags int, perm os.FileMode) (webdav.File, error) {
	n, e := cleanDAV(n)
	if e != nil {
		return nil, e
	}
	st := requestState(ctx)
	if flags&(os.O_TRUNC|os.O_CREATE) != 0 {
		if n == "." {
			return nil, os.ErrPermission
		}
		f, e := d.s.newStage(ctx, n, true)
		if e != nil {
			if !errors.Is(e, fs.ErrNotExist) {
				st.err = e
			}
			return nil, e
		}
		st.stages = append(st.stages, f)
		return &davFile{File: f.File, stage: f, s: d.s, name: n, ctx: ctx}, nil
	}
	d.s.mu.Lock()
	defer d.s.mu.Unlock()
	f, e := d.s.openTree(n)
	if e != nil {
		return nil, e
	}
	return &davFile{File: f, s: d.s, name: n, ctx: ctx}, nil
}
func (d davFS) Mkdir(ctx context.Context, n string, perm os.FileMode) error {
	n, e := cleanDAV(n)
	if e != nil || n == "." {
		return os.ErrPermission
	}
	d.s.mu.Lock()
	defer d.s.mu.Unlock()
	if e = ctx.Err(); e != nil {
		return e
	}
	p, _, e := d.s.parent(n)
	if e != nil {
		return e
	}
	p.Close()
	if _, e = d.s.view(); e != nil {
		return e
	}
	if d.s.entryCount >= MaxFiles {
		requestState(ctx).err = ErrFull
		return ErrFull
	}
	return d.s.root.Mkdir(n, 0700)
}
func (d davFS) RemoveAll(ctx context.Context, n string) error {
	n, e := cleanDAV(n)
	if e != nil || n == "." {
		return os.ErrPermission
	}
	d.s.mu.Lock()
	defer d.s.mu.Unlock()
	// Preflight the complete subtree before deleting anything (no symlink walks).
	f, e := d.s.openTree(n)
	if errors.Is(e, fs.ErrNotExist) {
		return nil
	}
	if e != nil {
		return e
	}
	fi, e := f.Stat()
	f.Close()
	if e != nil {
		return e
	}
	if fi.IsDir() {
		if e = d.s.scan(n, func(string, os.FileInfo) error { return nil }); e != nil {
			return e
		}
	}
	if e = d.s.removeTree(ctx, n); e != nil {
		return e
	}
	for k := range d.s.dav.props {
		if k == n || strings.HasPrefix(k, n+"/") {
			delete(d.s.dav.props, k)
		}
	}
	return d.s.saveProps()
}
func (d davFS) Rename(ctx context.Context, a, b string) error {
	a, e := cleanDAV(a)
	if e != nil || a == "." {
		return os.ErrPermission
	}
	b, e = cleanDAV(b)
	if e != nil || b == "." {
		return os.ErrPermission
	}
	d.s.mu.Lock()
	defer d.s.mu.Unlock()
	if e = ctx.Err(); e != nil {
		return e
	}
	p, _, e := d.s.parent(b)
	if e != nil {
		return e
	}
	p.Close()
	f, e := d.s.openTree(a)
	if e != nil {
		return e
	}
	f.Close()
	if e = d.s.root.Rename(a, b); e != nil {
		return e
	}
	moved := map[string][]webdav.Property{}
	for k, v := range d.s.dav.props {
		if k == a || strings.HasPrefix(k, a+"/") {
			moved[b+strings.TrimPrefix(k, a)] = v
			delete(d.s.dav.props, k)
		}
	}
	for k, v := range moved {
		d.s.dav.props[k] = v
	}
	return d.s.saveProps()
}

type davFile struct {
	*os.File
	stage *stage
	s     *Store
	name  string
	ctx   context.Context
}

// Do not inherit os.File's zero-copy methods: they would bypass quota/error
// tracking in Write/Read when io.Copy detects ReaderFrom or WriterTo.
func (f *davFile) ReadFrom(r io.Reader) (int64, error) {
	return io.CopyBuffer(struct{ io.Writer }{f}, r, make([]byte, 64<<10))
}
func (f *davFile) WriteTo(w io.Writer) (int64, error) {
	return io.CopyBuffer(w, struct{ io.Reader }{f}, make([]byte, 64<<10))
}
func (f *davFile) Write(p []byte) (int, error) {
	if f.stage == nil {
		return 0, os.ErrPermission
	}
	n, e := f.stage.Write(p)
	if e != nil {
		requestState(f.ctx).err = e
	}
	return n, e
}
func (f *davFile) Read(p []byte) (int, error) {
	n, e := f.File.Read(p)
	if e != nil && e != io.EOF {
		requestState(f.ctx).err = e
	}
	return n, e
}
func (f *davFile) Close() error {
	if f.stage != nil {
		return f.stage.Close()
	}
	return f.File.Close()
}
func (f *davFile) Readdir(count int) ([]os.FileInfo, error) {
	var out []os.FileInfo
	for {
		es, e := f.File.Readdir(1)
		if e != nil {
			if e == io.EOF && (count <= 0 || len(out) > 0) {
				return out, nil
			}
			return out, e
		}
		for _, st := range es {
			if strings.HasPrefix(st.Name(), ".upload-") || strings.HasPrefix(st.Name(), ".dav-") {
				continue
			}
			if !validComponent(st.Name()) || (!st.IsDir() && !st.Mode().IsRegular()) {
				return nil, ErrBoundary
			}
			out = append(out, st)
		}
		if count > 0 && len(out) >= count {
			return out, nil
		}
	}
}
func (f *davFile) DeadProps() (map[xml.Name]webdav.Property, error) {
	f.s.mu.Lock()
	defer f.s.mu.Unlock()
	m := map[xml.Name]webdav.Property{}
	for _, p := range f.s.dav.props[f.name] {
		p.InnerXML = bytes.Clone(p.InnerXML)
		m[p.XMLName] = p
	}
	pn := xml.Name{Space: "DAV:", Local: "lockdiscovery"}
	m[pn] = webdav.Property{XMLName: pn, InnerXML: f.s.dav.locks.discovery(path.Clean("/" + f.name))}
	return m, nil
}
func (f *davFile) Patch(ps []webdav.Proppatch) ([]webdav.Propstat, error) {
	f.s.mu.Lock()
	defer f.s.mu.Unlock()
	if e := f.ctx.Err(); e != nil {
		return nil, e
	}
	m := map[xml.Name]webdav.Property{}
	for _, p := range f.s.dav.props[f.name] {
		m[p.XMLName] = p
	}
	status := webdav.Propstat{Status: 200}
	for _, patch := range ps {
		for _, p := range patch.Props {
			if p.XMLName.Space == "DAV:" && (p.XMLName.Local == "lockdiscovery" || p.XMLName.Local == "supportedlock") {
				continue
			}
			status.Props = append(status.Props, webdav.Property{XMLName: p.XMLName})
			if patch.Remove {
				delete(m, p.XMLName)
			} else {
				m[p.XMLName] = p
			}
		}
	}
	next := make([]webdav.Property, 0, len(m))
	for _, p := range m {
		next = append(next, p)
	}
	old := f.s.dav.props[f.name]
	f.s.dav.props[f.name] = next
	if e := f.s.saveProps(); e != nil {
		f.s.dav.props[f.name] = old
		requestState(f.ctx).err = e
		return nil, e
	}
	return []webdav.Propstat{status}, nil
}
