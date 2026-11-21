package devicefs

import (
	"crypto/rand"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

const MaxUploadBytes int64 = 128 << 20
const ReserveBytes int64 = 128 << 20

func sameOrigin(r *http.Request) bool {
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Scheme != "https" || u.Host != r.Host {
			return false
		}
	}
	site := r.Header.Get("Sec-Fetch-Site")
	return site == "" || site == "same-origin" || site == "none"
}
func serveFile(w http.ResponseWriter, r *http.Request, root, name string, uploads *sync.Mutex) {
	if name == "/" {
		failure(w, 400, "invalid_path")
		return
	}
	switch r.Method {
	case "GET", "HEAD":
		resolved, err := resolve(root, name)
		if err != nil {
			directoryFailure(w, err)
			return
		}
		d, err := openDirectory(root, path.Dir(resolved))
		if err != nil {
			directoryFailure(w, err)
			return
		}
		defer d.Close()
		// O_PATH pins the inode without invoking a device's open callback.
		// Inspect it before obtaining a readable descriptor: even an attempted
		// open of e.g. a watchdog device must not have physical side effects.
		pinned, err := unix.Openat(int(d.Fd()), path.Base(resolved), unix.O_PATH|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if err != nil {
			directoryFailure(w, err)
			return
		}
		defer unix.Close(pinned)
		var st unix.Stat_t
		if err = unix.Fstat(pinned, &st); err != nil {
			directoryFailure(w, err)
			return
		}
		if st.Mode&unix.S_IFMT != unix.S_IFREG {
			failure(w, 409, "unsupported_file")
			return
		}
		fd, err := unix.Open("/proc/self/fd/"+strconv.Itoa(pinned), unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
		if err != nil {
			directoryFailure(w, err)
			return
		}
		f := os.NewFile(uintptr(fd), name)
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			directoryFailure(w, err)
			return
		}
		if !info.Mode().IsRegular() {
			failure(w, 409, "unsupported_file")
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": path.Base(name)}))
		w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
		// No managed-area or download-size restriction: the owner can download any
		// OS-readable regular file, including system/configuration files.
		http.ServeContent(w, r, path.Base(name), info.ModTime(), f)
	case "PUT":
		if !sameOrigin(r) {
			failure(w, 403, "cross_origin")
			return
		}
		if r.ContentLength < 0 {
			failure(w, 411, "length_required")
			return
		}
		if r.ContentLength > MaxUploadBytes {
			failure(w, 413, "file_too_large")
			return
		}
		if !uploads.TryLock() {
			failure(w, 423, "transfer_busy")
			return
		}
		defer uploads.Unlock()
		parent, err := resolve(root, path.Dir(name))
		if err != nil {
			directoryFailure(w, err)
			return
		}
		d, err := openDirectory(root, parent)
		if err != nil {
			directoryFailure(w, err)
			return
		}
		defer d.Close()
		rc := http.NewResponseController(w)
		_ = rc.SetReadDeadline(time.Now().Add(5 * time.Minute))
		defer rc.SetReadDeadline(time.Time{})
		entry, err := upload(d, path.Base(name), r.ContentLength, r.Body)
		if err != nil {
			transferFailure(w, err)
			return
		}
		respond(w, 201, entry)
	default:
		w.Header().Set("Allow", "GET, HEAD, PUT")
		failure(w, 405, "method_not_allowed")
	}
}
func capacity(d *os.File) (int64, error) {
	var st unix.Statfs_t
	if err := unix.Fstatfs(int(d.Fd()), &st); err != nil {
		return 0, err
	}
	if st.Flags&unix.ST_RDONLY != 0 {
		return 0, unix.EROFS
	}
	return max(int64(0), min(int64(st.Bavail)*int64(st.Bsize), int64(st.Bfree)*int64(st.Bsize)-ReserveBytes)), nil
}

var errExists = errors.New("already_exists")
var errFull = errors.New("storage_full")
var errAtomic = errors.New("atomic_publication_unsupported")

// upload pins the chosen destination, stages in that same filesystem, fsyncs
// bytes and atomically publishes without replacement. No whole-file RAM buffer or
// cross-filesystem move, remount, chmod of existing files or directory creation.
func upload(d *os.File, name string, size int64, body io.Reader) (Entry, error) {
	if len(name) > 255 || name == "." || name == ".." {
		return Entry{}, unix.ENAMETOOLONG
	}
	var st unix.Stat_t
	err := unix.Fstatat(int(d.Fd()), name, &st, unix.AT_SYMLINK_NOFOLLOW)
	if err == nil {
		return Entry{}, errExists
	}
	if !errors.Is(err, unix.ENOENT) {
		return Entry{}, err
	}
	free, err := capacity(d)
	if err != nil {
		return Entry{}, err
	}
	if size > free {
		return Entry{}, errFull
	}
	temp := ".echolocal-upload-" + rand.Text()
	fd, err := unix.Openat(int(d.Fd()), temp, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return Entry{}, err
	}
	f := os.NewFile(uintptr(fd), temp)
	defer f.Close()
	defer unix.Unlinkat(int(d.Fd()), temp, 0)
	n, err := io.CopyBuffer(f, io.LimitReader(body, size+1), make([]byte, 64<<10))
	if err != nil {
		return Entry{}, err
	}
	if n != size {
		return Entry{}, io.ErrUnexpectedEOF
	}
	if err = f.Sync(); err != nil {
		return Entry{}, err
	}
	info, err := f.Stat()
	if err != nil {
		return Entry{}, err
	}
	if err = f.Close(); err != nil {
		return Entry{}, err
	}
	// Recheck after staging: concurrent external writes must not leave a
	// successfully published upload spending the documented free-space reserve.
	if _, err = capacity(d); err != nil {
		return Entry{}, err
	}
	var stat unix.Statfs_t
	if err = unix.Fstatfs(int(d.Fd()), &stat); err != nil {
		return Entry{}, err
	}
	if int64(stat.Bfree)*int64(stat.Bsize) < ReserveBytes {
		return Entry{}, errFull
	}
	err = unix.Renameat2(int(d.Fd()), temp, int(d.Fd()), name, unix.RENAME_NOREPLACE)
	if errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EINVAL) || errors.Is(err, unix.EOPNOTSUPP) {
		err = unix.Linkat(int(d.Fd()), temp, int(d.Fd()), name, 0)
		if errors.Is(err, unix.EOPNOTSUPP) || errors.Is(err, unix.EPERM) {
			err = errAtomic
		}
	}
	if err != nil {
		return Entry{}, err
	}
	_ = d.Sync()
	return Entry{Name: name, Kind: "file", Size: info.Size(), Modified: info.ModTime().UTC(), Mode: info.Mode().String()}, nil
}
func transferFailure(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errAtomic):
		failure(w, 409, "atomic_publication_unsupported")
	case errors.Is(err, errExists), errors.Is(err, unix.EEXIST):
		failure(w, 409, "already_exists")
	case errors.Is(err, io.ErrUnexpectedEOF):
		failure(w, 400, "incomplete_upload")
	case errors.Is(err, errFull), errors.Is(err, syscall.ENOSPC), errors.Is(err, syscall.EDQUOT):
		failure(w, 507, "storage_full")
	default:
		directoryFailure(w, err)
	}
}

// The actual filesystem browser reports storage capacity for this directory,
// not the managed area's quota or an emulated filesystem budget.
func fillCapacity(d *os.File, v *Listing) {
	free, err := capacity(d)
	v.AvailableBytes = free
	v.MaxUploadBytes = MaxUploadBytes
	v.ReadOnly = errors.Is(err, unix.EROFS)
	v.CapacityKnown = err == nil || v.ReadOnly
}
