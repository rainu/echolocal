// Package userfiles provides the single managed user-asset store.
// Consumers must use List/Open, validate their own formats, and explicitly import
// assets into feature-owned storage. Uploading never activates content.
package userfiles

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"
)

const (
	MaxFileBytes int64 = 128 << 20
	BudgetBytes  int64 = 512 << 20
	ReserveBytes int64 = 128 << 20
	MaxFiles           = 256
)

var (
	ErrName     = errors.New("invalid_name")
	ErrExists   = errors.New("already_exists")
	ErrTooLarge = errors.New("file_too_large")
	ErrFull     = errors.New("storage_full")
	ErrBusy     = errors.New("transfer_busy")
	ErrBoundary = errors.New("unsafe_entry")
)

type Metadata struct {
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
}
type View struct {
	Directory string     `json:"directory"`
	Files     []Metadata `json:"files"`
	Used      int64      `json:"usedBytes"`
	Available int64      `json:"availableBytes"`
	Budget    int64      `json:"budgetBytes"`
	MaxFile   int64      `json:"maxFileBytes"`
	MaxFiles  int        `json:"maxFiles"`
}

// Store owns a pinned directory descriptor. The parent path is trusted
// application layout, never request input. Its contents are flat, regular files.
type Store struct {
	path                     string
	once                     sync.Once
	root                     *os.Root
	err                      error
	mu                       sync.Mutex // short metadata operations and atomic publication
	upload                   sync.Mutex // at most one transfer, nonblocking admission
	free                     func() (int64, error)
	budget, maxFile, reserve int64
}

func New(path string) *Store {
	return &Store{path: path, budget: BudgetBytes, maxFile: MaxFileBytes, reserve: ReserveBytes}
}
func ValidName(name string) bool {
	if len(name) == 0 || len(name) > 128 || !utf8.ValidString(name) || name[0] == '.' || strings.TrimSpace(name) != name {
		return false
	}
	for _, c := range name {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '-' || c == '_' || c == ' ') {
			return false
		}
	}
	return true
}
func (s *Store) init() error {
	s.once.Do(func() {
		if err := os.MkdirAll(s.path, 0700); err != nil {
			s.err = err
			return
		}
		info, err := os.Lstat(s.path)
		if err != nil {
			s.err = err
			return
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			s.err = ErrBoundary
			return
		}
		s.root, s.err = os.OpenRoot(s.path)
		if s.err != nil {
			return
		}
		// Crash leftovers never become active files. Do not follow any symlinks.
		entries, err := s.entries()
		if err != nil {
			s.err = err
			return
		}
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".upload-") {
				if !e.Mode().IsRegular() {
					s.err = ErrBoundary
					return
				}
				if err := s.root.Remove(e.Name()); err != nil {
					s.err = err
					return
				}
			}
		}
	})
	return s.err
}
func (s *Store) entries() ([]fs.FileInfo, error) {
	d, err := s.root.Open(".")
	if err != nil {
		return nil, err
	}
	defer d.Close()
	return d.Readdir(-1)
}

// reserveCapacity respects the OS's unavailable blocks without subtracting
// our explicit free-space reserve twice. A negative result forbids publication.
func reserveCapacity(available, free, reserve int64) int64 {
	return min(available, free-reserve)
}
func (s *Store) usableBytes() (int64, error) {
	if s.free != nil {
		free, err := s.free()
		return reserveCapacity(free, free, s.reserve), err
	}
	d, err := s.root.Open(".")
	if err != nil {
		return 0, err
	}
	defer d.Close()
	var st syscall.Statfs_t
	if err = syscall.Fstatfs(int(d.Fd()), &st); err != nil {
		return 0, err
	}
	return reserveCapacity(int64(st.Bavail)*int64(st.Bsize), int64(st.Bfree)*int64(st.Bsize), s.reserve), nil
}
func (s *Store) view() (View, error) {
	v := View{Directory: s.path, Files: []Metadata{}, Budget: s.budget, MaxFile: s.maxFile, MaxFiles: MaxFiles}
	entries, err := s.entries()
	if err != nil {
		return v, err
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".upload-") {
			continue
		}
		if !ValidName(e.Name()) || !e.Mode().IsRegular() {
			return v, ErrBoundary
		}
		v.Files = append(v.Files, Metadata{e.Name(), e.Size(), e.ModTime().UTC()})
		v.Used += e.Size()
	}
	slices.SortFunc(v.Files, func(a, b Metadata) int { return strings.Compare(a.Name, b.Name) })
	free, err := s.usableBytes()
	if err != nil {
		return v, err
	}
	v.Available = max(int64(0), min(s.budget-v.Used, free))
	if len(v.Files) >= MaxFiles {
		v.Available = 0
	}
	return v, nil
}
func (s *Store) List() (View, error) {
	if err := s.init(); err != nil {
		return View{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.view()
}

// Open returns only a regular, non-symlink file inside the pinned root.
// Caller must Close. Deletion after Open does not invalidate its descriptor.
func (s *Store) Open(name string) (*os.File, Metadata, error) {
	if !ValidName(name) {
		return nil, Metadata{}, ErrName
	}
	if err := s.init(); err != nil {
		return nil, Metadata{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// os.Root resolves internal symlinks even with O_NOFOLLOW supplied.
	// Use an atomic no-follow openat relative to its pinned directory instead.
	dir, err := s.root.Open(".")
	if err != nil {
		return nil, Metadata{}, err
	}
	fd, err := syscall.Openat(int(dir.Fd()), name, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	dir.Close()
	if err != nil {
		return nil, Metadata{}, err
	}
	f := os.NewFile(uintptr(fd), name)
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		f.Close()
		return nil, Metadata{}, ErrBoundary
	}
	return f, Metadata{name, st.Size(), st.ModTime().UTC()}, nil
}
func (s *Store) Delete(name string) error {
	if !ValidName(name) {
		return ErrName
	}
	if err := s.init(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.root.Lstat(name)
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() {
		return ErrBoundary
	}
	return s.root.Remove(name)
}
func (s *Store) Upload(name string, size int64, body io.Reader) (Metadata, error) {
	return s.UploadContext(context.Background(), name, size, body)
}

func (s *Store) UploadContext(ctx context.Context, name string, size int64, body io.Reader) (Metadata, error) {
	body = contextReader{ctx, body}
	if err := ctx.Err(); err != nil {
		return Metadata{}, err
	}

	if !ValidName(name) {
		return Metadata{}, ErrName
	}
	if size < 0 {
		return Metadata{}, ErrName
	}
	if size > s.maxFile {
		return Metadata{}, ErrTooLarge
	}
	if !s.upload.TryLock() {
		return Metadata{}, ErrBusy
	}
	defer s.upload.Unlock()
	if err := s.init(); err != nil {
		return Metadata{}, err
	}
	s.mu.Lock()
	_, err := s.root.Lstat(name)
	if err == nil {
		s.mu.Unlock()
		return Metadata{}, ErrExists
	}
	if !errors.Is(err, fs.ErrNotExist) {
		s.mu.Unlock()
		return Metadata{}, err
	}
	v, err := s.view()
	s.mu.Unlock()
	if err != nil {
		return Metadata{}, err
	}
	if len(v.Files) >= MaxFiles || size > v.Available {
		return Metadata{}, ErrFull
	}
	// Random temporary names are never accepted by the public name validator.
	// OpenFile O_EXCL plus Root keeps creation inside the pinned directory.
	var f *os.File
	var temp string
	for range 10 {
		temp = ".upload-" + time.Now().Format("20060102150405.000000000")
		f, err = s.root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if !errors.Is(err, fs.ErrExist) {
			break
		}
	}
	if err != nil {
		return Metadata{}, err
	}
	defer s.root.Remove(temp)
	defer f.Close()
	// Content-Length is mandatory. Read one extra byte to reject mismatches.
	n, err := io.CopyBuffer(f, io.LimitReader(body, size+1), make([]byte, 64<<10))
	if errors.Is(err, syscall.ENOSPC) || errors.Is(err, syscall.EDQUOT) {
		return Metadata{}, ErrFull
	}
	if err != nil {
		return Metadata{}, err
	}
	if n != size {
		return Metadata{}, io.ErrUnexpectedEOF
	}
	if err = f.Sync(); err != nil {
		return Metadata{}, err
	}
	if err = f.Close(); err != nil {
		return Metadata{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Metadata{}, err
	}
	free, err := s.usableBytes()
	if err != nil {
		return Metadata{}, err
	}
	if free < 0 {
		return Metadata{}, ErrFull
	}
	// Link is atomic and never overwrites an existing name (including symlinks).
	if err = s.root.Link(temp, name); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return Metadata{}, ErrExists
		}
		return Metadata{}, err
	}
	st, err := s.root.Stat(name)
	if err != nil {
		return Metadata{}, err
	}
	return Metadata{name, st.Size(), st.ModTime().UTC()}, nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.r.Read(p)
	if e := r.ctx.Err(); e != nil {
		return n, e
	}
	return n, err
}
