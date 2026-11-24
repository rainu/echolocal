package userfiles

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"syscall"
	"unicode"
	"unicode/utf8"
)

// ValidPath is shared by every managed-file protocol. Reserved names are for
// staging and metadata only, never user-addressable. No path normalization is
// performed: traversal and ambiguous empty components are rejected.
func ValidPath(n string) bool {
	if n == "" || len(n) > 1024 || !utf8.ValidString(n) || strings.Count(n, "/") >= 16 {
		return false
	}
	for _, c := range strings.Split(n, "/") {
		if !validComponent(c) {
			return false
		}
	}
	return true
}
func validComponent(n string) bool {
	if n == "" || n == "." || n == ".." || len(n) > 255 || strings.HasPrefix(n, ".upload-") || strings.HasPrefix(n, ".dav-") {
		return false
	}
	for _, r := range n {
		if unicode.IsControl(r) || strings.ContainsRune("/\\", r) {
			return false
		}
	}
	return true
}

// openTree never follows a symlink in ANY component. Root's containment alone
// would permit internal symlinks; openat(O_NOFOLLOW) intentionally does not.
func (s *Store) openTree(n string) (*os.File, error) {
	d, e := s.root.Open(".")
	if e != nil {
		return nil, e
	}
	if n == "." || n == "" || n == "/" {
		return d, nil
	}
	if !ValidPath(n) {
		d.Close()
		return nil, ErrName
	}
	cs := strings.Split(n, "/")
	for i, c := range cs {
		flags := syscall.O_RDONLY | syscall.O_NOFOLLOW | syscall.O_NONBLOCK | syscall.O_CLOEXEC
		if i < len(cs)-1 {
			flags |= syscall.O_DIRECTORY
		}
		fd, e := syscall.Openat(int(d.Fd()), c, flags, 0)
		d.Close()
		if e != nil {
			if errors.Is(e, syscall.ENOTDIR) {
				return nil, fs.ErrNotExist
			}
			if errors.Is(e, syscall.ELOOP) {
				return nil, ErrBoundary
			}
			return nil, e
		}
		d = os.NewFile(uintptr(fd), c)
	}
	st, e := d.Stat()
	if e != nil || (!st.IsDir() && !st.Mode().IsRegular()) {
		d.Close()
		return nil, ErrBoundary
	}
	return d, nil
}
func (s *Store) parent(n string) (*os.File, string, error) {
	if !ValidPath(n) {
		return nil, "", ErrName
	}
	d, e := s.openTree(path.Dir(n))
	if e == nil {
		st, se := d.Stat()
		if se != nil {
			d.Close()
			return nil, "", se
		}
		if !st.IsDir() {
			d.Close()
			return nil, "", fs.ErrNotExist
		}
	}
	return d, path.Base(n), e
}
func (s *Store) statTree(n string) (os.FileInfo, error) {
	if s.statHook != nil {
		return s.statHook(n)
	}
	f, e := s.openTree(n)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	return f.Stat()
}
func (s *Store) scan(n string, visit func(string, os.FileInfo) error) error {
	d, e := s.openTree(n)
	if e != nil {
		return e
	}
	defer d.Close()
	es, e := d.Readdir(-1)
	if e != nil {
		return e
	}
	for _, st := range es {
		if strings.HasPrefix(st.Name(), ".upload-") || strings.HasPrefix(st.Name(), ".dav-") {
			continue
		}
		child := st.Name()
		if n != "." {
			child = n + "/" + child
		}
		if !ValidPath(child) || (!st.IsDir() && !st.Mode().IsRegular()) {
			return ErrBoundary
		}
		if e = visit(child, st); e != nil {
			return e
		}
		if st.IsDir() {
			if e = s.scan(child, visit); e != nil {
				return e
			}
		}
	}
	return nil
}
func (s *Store) removeTree(ctx context.Context, n string) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	f, e := s.openTree(n)
	if e != nil {
		return e
	}
	st, e := f.Stat()
	if e != nil {
		f.Close()
		return e
	}
	if st.IsDir() {
		es, e := f.Readdir(-1)
		f.Close()
		if e != nil {
			return e
		}
		for _, st := range es {
			if !validComponent(st.Name()) {
				return ErrBoundary
			}
			if e = s.removeTree(ctx, n+"/"+st.Name()); e != nil {
				return e
			}
		}
	} else {
		f.Close()
	}
	p, b, e := s.parent(n)
	if e != nil {
		return e
	}
	defer p.Close()
	// Root.Remove uses the pinned root; parent validation plus the shared mutation
	// admission prevents another protocol changing a component under this call.
	_ = b
	return s.root.Remove(n)
}
func storageError(e error) error {
	if errors.Is(e, syscall.ENOSPC) || errors.Is(e, syscall.EDQUOT) {
		return ErrFull
	}
	return e
}

// stage is used for DAV writes as well as API uploads: no active file is ever
// truncated. The caller owns the shared mutation slot through commit/abort.
type stage struct {
	*os.File
	s            *Store
	ctx          context.Context
	name, temp   string
	count, limit int64
	err          error
	done         bool
}

func (s *Store) newStage(ctx context.Context, n string, replace bool) (*stage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, _, e := s.parent(n)
	if e != nil {
		return nil, e
	}
	p.Close()
	st, e := s.statTree(n)
	old := int64(0)
	if e == nil {
		if !replace {
			return nil, ErrExists
		}
		if !st.Mode().IsRegular() {
			return nil, ErrBoundary
		}
		old = st.Size()
	} else if !errors.Is(e, fs.ErrNotExist) {
		return nil, e
	}
	v, e := s.view()
	if e != nil {
		return nil, e
	}
	if st == nil && s.entryCount >= MaxFiles {
		return nil, ErrFull
	}
	free, e := s.usableBytes()
	if e != nil {
		return nil, e
	}
	limit := min(s.maxFile, s.budget-v.Used+old, free)
	if limit < 0 {
		return nil, ErrFull
	}
	for range 10 {
		temp := ".upload-" + randomID()
		f, e := s.root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
		if errors.Is(e, fs.ErrExist) {
			continue
		}
		if e != nil {
			return nil, e
		}
		return &stage{File: f, s: s, ctx: ctx, name: n, temp: temp, limit: limit}, nil
	}
	return nil, ErrBusy
}
func (f *stage) Write(p []byte) (int, error) {
	if f.err != nil {
		return 0, f.err
	}
	if e := f.ctx.Err(); e != nil {
		f.err = e
		return 0, e
	}
	if int64(len(p)) > f.limit-f.count {
		f.err = ErrFull
		if f.count+int64(len(p)) > f.s.maxFile {
			f.err = ErrTooLarge
		}
		return 0, f.err
	}
	n, e := f.File.Write(p)
	f.count += int64(n)
	f.err = storageError(e)
	return n, f.err
}
func (f *stage) Close() error {
	// DAV closes a writable file before replying. Publication is deliberately
	// deferred to finish(), after the handler AND request body succeeded.
	return f.err
}
func (f *stage) abort() {
	if !f.done {
		f.done = true
		f.File.Close()
		f.s.root.Remove(f.temp)
	}
}
func (f *stage) finish() error {
	defer f.abort()
	if f.err != nil {
		return f.err
	}
	if e := f.ctx.Err(); e != nil {
		return e
	}
	if e := f.File.Sync(); e != nil {
		return storageError(e)
	}
	if e := f.File.Close(); e != nil {
		return e
	}
	f.s.mu.Lock()
	defer f.s.mu.Unlock()
	if e := f.ctx.Err(); e != nil {
		return e
	}
	free, e := f.s.usableBytes()
	if e != nil {
		return e
	}
	if free < 0 {
		return ErrFull
	}
	p, _, e := f.s.parent(f.name)
	if e != nil {
		return e
	}
	defer p.Close()
	if st, e := f.s.statTree(f.name); e == nil && !st.Mode().IsRegular() {
		return ErrBoundary
	} else if e != nil && !errors.Is(e, fs.ErrNotExist) {
		return e
	}
	return storageError(f.s.root.Rename(f.temp, f.name))
}

var _ io.Writer = (*stage)(nil)
