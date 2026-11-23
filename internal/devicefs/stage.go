package devicefs

import (
	"context"
	"errors"
	"github.com/ygelfand/echolocal/internal/userfiles"
	"golang.org/x/sys/unix"
	"io"
	"os"
)

// stagedUpload is common to the web browser and DAV. It never truncates an
// active inode, follows a destination symlink, or leaves a partial target.
type stagedUpload struct {
	f, d                *os.File
	name, temp, logical string
	ctx                 context.Context
	n, limit            int64
	err                 error
	closed              bool
	replace             bool
	old                 unix.Stat_t
	existed             bool
}

func newUpload(ctx context.Context, d *os.File, name string, replace bool) (*stagedUpload, error) {
	if len(name) > 255 || name == "." || name == ".." {
		return nil, unix.ENAMETOOLONG
	}
	var st unix.Stat_t
	e := unix.Fstatat(int(d.Fd()), name, &st, unix.AT_SYMLINK_NOFOLLOW)
	existed := e == nil
	if e == nil {
		if !replace {
			return nil, errExists
		}
		if st.Mode&unix.S_IFMT != unix.S_IFREG {
			return nil, unix.EPERM
		}
	} else if !errors.Is(e, unix.ENOENT) {
		return nil, e
	}
	free, e := capacity(d)
	if e != nil {
		return nil, e
	}
	temp := ".echolocal-upload-" + randomName()
	fd, e := unix.Openat(int(d.Fd()), temp, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if e != nil {
		return nil, e
	}
	return &stagedUpload{f: os.NewFile(uintptr(fd), temp), d: d, name: name, temp: temp, ctx: ctx, limit: min(free, MaxUploadBytes), replace: replace && existed, old: st, existed: existed}, nil
}
func (s *stagedUpload) Write(p []byte) (int, error) {
	if s.err != nil {
		return 0, s.err
	}
	if e := s.ctx.Err(); e != nil {
		s.err = e
		return 0, e
	}
	if int64(len(p)) > s.limit-s.n {
		s.err = errFull
		if s.n+int64(len(p)) > MaxUploadBytes {
			s.err = userfiles.ErrTooLarge
		}
		return 0, s.err
	}
	n, e := s.f.Write(p)
	s.n += int64(n)
	s.err = e
	return n, e
}
func (s *stagedUpload) abort() {
	if !s.closed {
		s.closed = true
		s.f.Close()
		unix.Unlinkat(int(s.d.Fd()), s.temp, 0)
		s.d.Close()
	}
}
func (s *stagedUpload) commit() (Entry, error) {
	if s.err != nil {
		return Entry{}, s.err
	}
	if e := s.ctx.Err(); e != nil {
		return Entry{}, e
	}
	if e := s.f.Sync(); e != nil {
		return Entry{}, e
	}
	st, e := s.f.Stat()
	if e != nil {
		return Entry{}, e
	}
	if e = s.f.Close(); e != nil {
		return Entry{}, e
	}
	if _, e = capacity(s.d); e != nil {
		return Entry{}, e
	}
	var stat unix.Statfs_t
	if e = unix.Fstatfs(int(s.d.Fd()), &stat); e != nil {
		return Entry{}, e
	}
	if int64(stat.Bfree)*int64(stat.Bsize) < ReserveBytes {
		return Entry{}, errFull
	}
	if e = s.ctx.Err(); e != nil {
		return Entry{}, e
	}
	if s.replace {
		var now unix.Stat_t
		if e = unix.Fstatat(int(s.d.Fd()), s.name, &now, unix.AT_SYMLINK_NOFOLLOW); e != nil {
			return Entry{}, e
		}
		if now.Dev != s.old.Dev || now.Ino != s.old.Ino || now.Size != s.old.Size || now.Mtim != s.old.Mtim {
			return Entry{}, errChanged
		}
		e = unix.Renameat(int(s.d.Fd()), s.temp, int(s.d.Fd()), s.name)
	} else {
		e = unix.Renameat2(int(s.d.Fd()), s.temp, int(s.d.Fd()), s.name, unix.RENAME_NOREPLACE)
		if errors.Is(e, unix.ENOSYS) || errors.Is(e, unix.EINVAL) || errors.Is(e, unix.EOPNOTSUPP) {
			e = unix.Linkat(int(s.d.Fd()), s.temp, int(s.d.Fd()), s.name, 0)
			if errors.Is(e, unix.EOPNOTSUPP) || errors.Is(e, unix.EPERM) {
				e = errAtomic
			}
		}
	}
	if e != nil {
		return Entry{}, e
	}
	_ = s.d.Sync()
	return Entry{Name: s.name, Kind: "file", Size: st.Size(), Modified: st.ModTime().UTC(), Mode: st.Mode().String()}, nil
}
func uploadContext(ctx context.Context, d *os.File, name string, size int64, body io.Reader) (Entry, error) {
	fd, e := unix.Dup(int(d.Fd()))
	if e != nil {
		return Entry{}, e
	}
	owned := os.NewFile(uintptr(fd), d.Name())
	s, e := newUpload(ctx, owned, name, false)
	if e != nil {
		owned.Close()
		return Entry{}, e
	}
	defer s.abort()
	if size < 0 || size > MaxUploadBytes {
		return Entry{}, userfiles.ErrTooLarge
	}
	n, e := io.CopyBuffer(s, io.LimitReader(body, size+1), make([]byte, 64<<10))
	if e != nil {
		return Entry{}, e
	}
	if n != size {
		return Entry{}, io.ErrUnexpectedEOF
	}
	return s.commit()
}
