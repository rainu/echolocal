package devicefs

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"unicode/utf8"

	"github.com/ygelfand/echolocal/internal/userfiles"
	"golang.org/x/net/webdav"
	"golang.org/x/sys/unix"
)

var errChanged = errors.New("destination_changed")

type Service struct {
	root, metadata string
	uploads        sync.Mutex
	namespace      sync.RWMutex
	protocol       *userfiles.Protocol
	props          map[string][]webdav.Property
	initErr        error
}

func NewService(root, metadata string) *Service {
	s := &Service{root: root, metadata: metadata, props: map[string][]webdav.Property{}}
	if metadata != "" {
		b, e := os.ReadFile(metadata)
		if e == nil {
			if len(b) > 1<<20 {
				s.initErr = errFull
			} else {
				s.initErr = json.Unmarshal(b, &s.props)
			}
		} else if !errors.Is(e, fs.ErrNotExist) {
			s.initErr = e
		}
	}
	if s.props == nil {
		s.props = map[string][]webdav.Property{}
	}
	p := userfiles.NewProtocol(deviceDAV{s})
	s.protocol = p
	p.Canonical = s.canonical
	p.Begin = s.begin
	p.Error = func(c context.Context) error { return txn(c).err }
	p.Failure = davFailure
	return s
}
func (s *Service) DAVHandler() http.Handler { return s.protocol }

type namespaceReleaseKey struct{}

func releaseNamespace(r *http.Request) {
	if f, ok := r.Context().Value(namespaceReleaseKey{}).(func()); ok {
		f()
	}
}

func (s *Service) Handler() http.Handler {
	h := handlerShared(s.root, &s.uploads, s.protocol.Acquire)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" || r.Method == "HEAD" {
			s.namespace.RLock()
			var once sync.Once
			release := func() { once.Do(s.namespace.RUnlock) }
			defer release()
			r = r.WithContext(context.WithValue(r.Context(), namespaceReleaseKey{}, release))
		}
		h.ServeHTTP(w, r)
	})
}
func validDevicePath(n string) bool {
	if len(n) > 4096 || !utf8.ValidString(n) || strings.ContainsAny(n, "\x00\\") {
		return false
	}
	for _, c := range strings.Split(n, "/") {
		if c == "." || c == ".." || strings.HasPrefix(c, ".echolocal-upload-") || strings.HasPrefix(c, ".echolocal-dav-") {
			return false
		}
		for _, r := range c {
			if r < 32 || r == 127 {
				return false
			}
		}
	}
	return true
}
func (s *Service) canonical(n string) (string, error) {
	n = "/" + strings.TrimPrefix(n, "/")
	if !validDevicePath(n) {
		return "", unix.EPERM
	}
	n = path.Clean(n)
	tail := []string{}
	current := n
	for {
		v, e := resolve(s.root, current)
		if e == nil {
			for i := len(tail) - 1; i >= 0; i-- {
				v = path.Join(v, tail[i])
			}
			return v, nil
		}
		if !errors.Is(e, fs.ErrNotExist) && !errors.Is(e, unix.ENOTDIR) {
			return "", e
		}
		if current == "/" {
			return "", e
		}
		tail = append(tail, path.Base(current))
		current = path.Dir(current)
	}
}
func (s *Service) parent(n string) (*os.File, string, string, error) {
	n = "/" + strings.TrimPrefix(n, "/")
	if !validDevicePath(n) || path.Clean(n) == "/" {
		return nil, "", "", unix.EPERM
	}
	n = path.Clean(n)
	p, e := resolve(s.root, path.Dir(n))
	if e != nil {
		return nil, "", "", e
	}
	d, e := openDirectory(s.root, p)
	return d, path.Base(n), path.Join(p, path.Base(n)), e
}

type txKey struct{}
type davTxn struct {
	s                                       *Service
	ctx                                     context.Context
	method                                  string
	mutation, readLocked, writeLocked, done bool
	err                                     error
	stages                                  []*stagedUpload
	undo                                    []func()
	cleanup                                 []func()
	props                                   map[string][]webdav.Property
	propsDirty                              bool
	seen                                    map[[2]uint64]bool
	count                                   int
}

func txn(c context.Context) *davTxn { return c.Value(txKey{}).(*davTxn) }
func randomName() string            { return rand.Text() }
func itoa(n int) string             { return strconv.Itoa(n) }
func (t *davTxn) writeLock() {
	if !t.writeLocked {
		t.s.namespace.Lock()
		t.writeLocked = true
	}
}
func (t *davTxn) readLock() func() {
	if t.readLocked || t.writeLocked {
		return func() {}
	}
	t.s.namespace.RLock()
	return t.s.namespace.RUnlock
}
func (s *Service) begin(r *http.Request) (*http.Request, func(bool) error, error) {
	mut := r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" && r.Method != "PROPFIND"
	if s.initErr != nil {
		return nil, nil, s.initErr
	}
	if mut && !s.uploads.TryLock() {
		return nil, nil, userfiles.ErrBusy
	}
	t := &davTxn{s: s, ctx: r.Context(), method: r.Method, mutation: mut, seen: map[[2]uint64]bool{}}
	if !mut {
		s.namespace.RLock()
		t.readLocked = true
	}
	if mut {
		s.namespace.RLock()
	}
	t.props = map[string][]webdav.Property{}
	for k, v := range s.props {
		t.props[k] = append([]webdav.Property(nil), v...)
	}
	if mut {
		s.namespace.RUnlock()
	}
	return r.WithContext(context.WithValue(r.Context(), txKey{}, t)), t.finish, nil
}
func (t *davTxn) finish(success bool) (err error) {
	if t.done {
		return nil
	}
	t.done = true
	defer func() {
		for _, st := range t.stages {
			st.abort()
		}
		for _, f := range t.cleanup {
			f()
		}
		if t.writeLocked {
			t.s.namespace.Unlock()
		}
		if t.readLocked {
			t.s.namespace.RUnlock()
		}
		if t.mutation {
			t.s.uploads.Unlock()
		}
	}()
	rollback := func() {
		for i := len(t.undo) - 1; i >= 0; i-- {
			t.undo[i]()
		}
	}
	if !success || t.err != nil || t.ctx.Err() != nil {
		rollback()
		return t.err
	}
	if !t.mutation {
		return nil
	}
	t.writeLock()
	// Existing destinations are journaled only when the request commits. A
	// stalled PUT never removes its old target or holds the namespace write lock.
	for _, st := range t.stages {
		name := filepath.Join(st.d.Name(), st.name)
		if st.replace {
			if e := t.backupFD(st.d, st.name, name); e != nil {
				rollback()
				return e
			}
		}
		if _, e := st.commit(); e != nil {
			rollback()
			return e
		}
		d, n := st.d, st.name
		t.undo = append(t.undo, func() { unix.Unlinkat(int(d.Fd()), n, 0) })
	}
	if t.propsDirty {
		if err = t.s.saveProperties(t.props); err != nil {
			rollback()
			return err
		}
		t.s.props = t.props
	}
	return nil
}
func (t *davTxn) backupFD(d *os.File, n, full string) error {
	var st unix.Stat_t
	e := unix.Fstatat(int(d.Fd()), n, &st, unix.AT_SYMLINK_NOFOLLOW)
	if errors.Is(e, unix.ENOENT) {
		return nil
	}
	if e != nil {
		return e
	}
	if st.Mode&unix.S_IFMT == unix.S_IFLNK || st.Mode&unix.S_IFMT != unix.S_IFREG && st.Mode&unix.S_IFMT != unix.S_IFDIR {
		return unix.EPERM
	}
	temp := ".echolocal-dav-" + randomName()
	if st.Mode&unix.S_IFMT == unix.S_IFREG {
		e = unix.Linkat(int(d.Fd()), n, int(d.Fd()), temp, 0)
		if errors.Is(e, unix.EOPNOTSUPP) || errors.Is(e, unix.EPERM) {
			return errAtomic
		}
	} else {
		e = unix.Renameat(int(d.Fd()), n, int(d.Fd()), temp)
	}
	if e != nil {
		return e
	}
	fd, e := unix.Dup(int(d.Fd()))
	if e != nil {
		unix.Renameat(int(d.Fd()), temp, int(d.Fd()), n)
		return e
	}
	pin := os.NewFile(uintptr(fd), full)
	t.undo = append(t.undo, func() { unix.Renameat(fd, temp, fd, n) })
	t.cleanup = append(t.cleanup, func() {
		r, e := os.OpenRoot("/proc/self/fd/" + itoa(fd))
		if e == nil {
			r.RemoveAll(temp)
			r.Close()
		}
		pin.Close()
	})
	return nil
}
func (s *Service) saveProperties(p map[string][]webdav.Property) error {
	if s.metadata == "" {
		return nil
	}
	b, e := json.Marshal(p)
	if e != nil {
		return e
	}
	if len(b) > 1<<20 {
		return errFull
	}
	d, e := openDirectory("/", filepath.Dir(s.metadata))
	if e != nil {
		return e
	}
	defer d.Close()
	tmp := ".echolocal-dav-" + randomName()
	fd, e := unix.Openat(int(d.Fd()), tmp, unix.O_CREAT|unix.O_EXCL|unix.O_WRONLY|unix.O_CLOEXEC, 0600)
	if e != nil {
		return e
	}
	defer unix.Unlinkat(int(d.Fd()), tmp, 0)
	f := os.NewFile(uintptr(fd), tmp)
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
	return unix.Renameat(int(d.Fd()), tmp, int(d.Fd()), filepath.Base(s.metadata))
}
func davFailure(w http.ResponseWriter, e error) {
	switch {
	case errors.Is(e, errChanged):
		failure(w, 412, "destination_changed")
	case errors.Is(e, userfiles.ErrBusy):
		failure(w, 423, "transfer_busy")
	case errors.Is(e, userfiles.ErrTooLarge):
		failure(w, 413, "file_too_large")
	case errors.Is(e, unix.EXDEV):
		failure(w, 409, "cross_filesystem_move")
	default:
		transferFailure(w, e)
	}
}

type deviceDAV struct{ s *Service }

func (d deviceDAV) Stat(c context.Context, n string) (os.FileInfo, error) {
	t := txn(c)
	unlock := t.readLock()
	defer unlock()
	n, e := d.s.canonical(n)
	if e != nil {
		return nil, e
	}
	for _, st := range t.stages {
		if path.Clean("/"+strings.TrimPrefix(n, "/")) == stLogical(st, d.s.root) {
			v, e := st.f.Stat()
			return namedInfo{v, path.Base(n)}, e
		}
	}
	f, e := d.open(n)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	return f.Stat()
}
func stLogical(st *stagedUpload, root string) string { return st.logical }
func (d deviceDAV) open(n string) (*deviceFile, error) {
	display := path.Base(path.Clean("/" + n))
	n, e := d.s.canonical(n)
	if e != nil {
		return nil, e
	}
	if n == "/" {
		f, e := openDirectory(d.s.root, "/")
		if e != nil {
			return nil, e
		}
		st, e := f.Stat()
		return &deviceFile{f: f, info: namedInfo{st, "/"}, name: n, s: d.s}, e
	}
	p, e := openDirectory(d.s.root, path.Dir(n))
	if e != nil {
		return nil, e
	}
	defer p.Close()
	fd, e := unix.Openat(int(p.Fd()), path.Base(n), unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, e
	}
	pin := os.NewFile(uintptr(fd), n)
	st, e := pin.Stat()
	if e != nil {
		pin.Close()
		return nil, e
	}
	f := &deviceFile{pin: pin, info: namedInfo{st, display}, name: n, s: d.s}
	if st.IsDir() {
		f.f, e = openDirectory(d.s.root, n)
	}
	if e != nil {
		pin.Close()
		return nil, e
	}
	return f, nil
}
func (d deviceDAV) OpenFile(c context.Context, n string, flags int, mode os.FileMode) (webdav.File, error) {
	t := txn(c)
	unlock := t.readLock()
	defer unlock()
	if flags&(os.O_CREATE|os.O_TRUNC) != 0 {
		p, b, logical, e := d.s.parent(n)
		if e != nil {
			return nil, e
		}
		st, e := newUpload(c, p, b, true)
		if e != nil {
			p.Close()
			t.err = e
			return nil, e
		}
		st.logical = logical
		t.stages = append(t.stages, st)
		info, e := st.f.Stat()
		return &deviceFile{f: st.f, stage: st, info: namedInfo{info, b}, name: logical, s: d.s, t: t}, e
	}
	f, e := d.open(n)
	if e != nil {
		return nil, e
	}
	f.t = t
	if (t.method == "GET" || t.method == "HEAD") && t.readLocked {
		t.s.namespace.RUnlock()
		t.readLocked = false
	}
	if t.method == "COPY" {
		t.count++
		if t.count > 4096 {
			f.Close()
			t.err = errFull
			return nil, errFull
		}
		if f.info.IsDir() {
			st, e := f.f.Stat()
			if e != nil {
				f.Close()
				return nil, e
			}
			v := st.Sys().(*syscall.Stat_t)
			key := [2]uint64{uint64(v.Dev), uint64(v.Ino)}
			if t.seen[key] {
				f.Close()
				t.err = unix.ELOOP
				return nil, unix.ELOOP
			}
			t.seen[key] = true
		}
	}
	return f, nil
}
func (d deviceDAV) Mkdir(c context.Context, n string, mode os.FileMode) error {
	t := txn(c)
	t.writeLock()
	p, b, logical, e := d.s.parent(n)
	if e != nil {
		return e
	}
	defer p.Close()
	if _, e = capacity(p); e != nil {
		t.err = e
		return e
	}
	if e = unix.Mkdirat(int(p.Fd()), b, 0700); e != nil {
		return e
	}
	r, e := os.OpenRoot("/proc/self/fd/" + itoa(int(p.Fd())))
	if e != nil {
		return e
	}
	t.undo = append(t.undo, func() { r.RemoveAll(b) })
	t.cleanup = append(t.cleanup, func() { r.Close() })
	_ = logical
	return nil
}
func (d deviceDAV) RemoveAll(c context.Context, n string) error {
	t := txn(c)
	t.writeLock()
	p, b, logical, e := d.s.parent(n)
	if e != nil {
		return e
	}
	defer p.Close()
	if _, e = capacity(p); e != nil {
		t.err = e
		return e
	}
	var before unix.Stat_t
	if e = unix.Fstatat(int(p.Fd()), b, &before, unix.AT_SYMLINK_NOFOLLOW); errors.Is(e, unix.ENOENT) {
		return nil
	} else if e != nil {
		return e
	}
	if e = t.backupFD(p, b, logical); e != nil {
		return e
	}
	if before.Mode&unix.S_IFMT == unix.S_IFREG {
		if e = unix.Unlinkat(int(p.Fd()), b, 0); e != nil {
			return e
		}
	}

	for k := range t.props {
		if k == logical || strings.HasPrefix(k, logical+"/") {
			delete(t.props, k)
			t.propsDirty = true
		}
	}
	return nil
}
func (d deviceDAV) Rename(c context.Context, a, b string) error {
	t := txn(c)
	t.writeLock()
	src, sb, source, e := d.s.parent(a)
	if e != nil {
		return e
	}
	defer src.Close()
	dst, db, target, e := d.s.parent(b)
	if e != nil {
		return e
	}
	defer dst.Close()
	var st unix.Stat_t
	if e = unix.Fstatat(int(src.Fd()), sb, &st, unix.AT_SYMLINK_NOFOLLOW); e != nil {
		return e
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG && st.Mode&unix.S_IFMT != unix.S_IFDIR {
		return unix.EPERM
	}
	if _, e = capacity(src); e != nil {
		t.err = e
		return e
	}
	if _, e = capacity(dst); e != nil {
		t.err = e
		return e
	}
	if e = unix.Renameat2(int(src.Fd()), sb, int(dst.Fd()), db, unix.RENAME_NOREPLACE); e != nil {
		return e
	}
	sd, _ := unix.Dup(int(src.Fd()))
	dd, _ := unix.Dup(int(dst.Fd()))
	t.undo = append(t.undo, func() { unix.Renameat(dd, db, sd, sb) })
	t.cleanup = append(t.cleanup, func() { unix.Close(sd); unix.Close(dd) })
	moved := map[string][]webdav.Property{}
	for k, v := range t.props {
		if k == source || strings.HasPrefix(k, source+"/") {
			moved[target+strings.TrimPrefix(k, source)] = v
			delete(t.props, k)
			t.propsDirty = true
		}
	}
	for k, v := range moved {
		t.props[k] = v
	}
	return nil
}

type namedInfo struct {
	os.FileInfo
	name string
}

func (n namedInfo) Name() string { return n.name }
func (n namedInfo) ContentType(context.Context) (string, error) {
	return "application/octet-stream", nil
}

type deviceFile struct {
	f, pin  *os.File
	info    namedInfo
	stage   *stagedUpload
	name    string
	s       *Service
	t       *davTxn
	pos     int
	entries []os.FileInfo
}

func (f *deviceFile) Stat() (os.FileInfo, error) {
	if f.stage != nil {
		st, e := f.stage.f.Stat()
		return namedInfo{st, path.Base(f.name)}, e
	}
	return f.info, nil
}
func (f *deviceFile) readable() error {
	if !f.info.Mode().IsRegular() {
		return unix.EPERM
	}
	if f.f == nil {
		fd, e := unix.Open("/proc/self/fd/"+itoa(int(f.pin.Fd())), unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
		if e != nil {
			return e
		}
		f.f = os.NewFile(uintptr(fd), f.name)
	}
	return nil
}
func (f *deviceFile) Read(p []byte) (int, error) {
	if e := f.readable(); e != nil {
		if f.t != nil {
			f.t.err = e
		}
		return 0, e
	}
	n, e := f.f.Read(p)
	if e != nil && e != io.EOF && f.t != nil {
		f.t.err = e
	}
	return n, e
}
func (f *deviceFile) Seek(off int64, whence int) (int64, error) {
	if e := f.readable(); e != nil {
		return 0, e
	}
	return f.f.Seek(off, whence)
}
func (f *deviceFile) Write(b []byte) (int, error) {
	if f.stage == nil {
		return 0, unix.EPERM
	}
	n, e := f.stage.Write(b)
	if e != nil {
		f.t.err = e
	}
	return n, e
}
func (f *deviceFile) ReadFrom(r io.Reader) (int64, error) {
	return io.CopyBuffer(struct{ io.Writer }{f}, r, make([]byte, 64<<10))
}
func (f *deviceFile) WriteTo(w io.Writer) (int64, error) {
	return io.CopyBuffer(w, struct{ io.Reader }{f}, make([]byte, 64<<10))
}
func (f *deviceFile) Close() error {
	if f.stage != nil {
		return f.stage.err
	}
	if f.f != nil {
		f.f.Close()
	}
	if f.pin != nil {
		return f.pin.Close()
	}
	return nil
}
func (f *deviceFile) Readdir(count int) ([]os.FileInfo, error) {
	if !f.info.IsDir() {
		return nil, unix.ENOTDIR
	}
	if f.entries == nil {
		es, e := f.f.Readdir(65537)
		if e != nil && e != io.EOF {
			return nil, e
		}
		if len(es) > 65536 {
			f.t.err = errFull
			return nil, errFull
		}
		f.entries = []os.FileInfo{}
		for _, st := range es {
			if strings.HasPrefix(st.Name(), ".echolocal-upload-") || strings.HasPrefix(st.Name(), ".echolocal-dav-") {
				continue
			}
			name := path.Join(f.name, st.Name())
			v := os.FileInfo(namedInfo{st, st.Name()})
			if st.Mode()&os.ModeSymlink != 0 {
				other, e := deviceDAV{f.s}.open(name)
				if e == nil {
					v = namedInfo{other.info.FileInfo, st.Name()}
					other.Close()
				}
			}
			f.entries = append(f.entries, v)
		}
	}
	if f.pos >= len(f.entries) {
		if count > 0 {
			return nil, io.EOF
		}
		return nil, nil
	}
	end := len(f.entries)
	if count > 0 {
		end = min(end, f.pos+count)
	}
	v := f.entries[f.pos:end]
	f.pos = end
	return v, nil
}
func (f *deviceFile) DeadProps() (map[xml.Name]webdav.Property, error) {
	p := f.s.props
	if f.t != nil {
		p = f.t.props
	}
	out := map[xml.Name]webdav.Property{}
	for _, v := range p[f.name] {
		out[v.XMLName] = v
	}
	n := xml.Name{Space: "DAV:", Local: "lockdiscovery"}
	out[n] = webdav.Property{XMLName: n, InnerXML: f.s.protocol.Discovery(f.name)}
	return out, nil
}
func (f *deviceFile) Patch(ps []webdav.Proppatch) ([]webdav.Propstat, error) {
	if f.t == nil || !f.t.mutation {
		return nil, unix.EPERM
	}
	m := map[xml.Name]webdav.Property{}
	for _, p := range f.t.props[f.name] {
		m[p.XMLName] = p
	}
	status := webdav.Propstat{Status: 200}
	for _, patch := range ps {
		for _, p := range patch.Props {
			if p.XMLName == (xml.Name{Space: "DAV:", Local: "lockdiscovery"}) {
				continue
			}
			status.Props = append(status.Props, webdav.Property{XMLName: p.XMLName})
			if patch.Remove {
				delete(m, p.XMLName)
			} else {
				p.InnerXML = bytes.Clone(p.InnerXML)
				m[p.XMLName] = p
			}
		}
	}
	values := []webdav.Property{}
	for _, p := range m {
		values = append(values, p)
	}
	f.t.props[f.name] = values
	f.t.propsDirty = true
	return []webdav.Propstat{status}, nil
}
