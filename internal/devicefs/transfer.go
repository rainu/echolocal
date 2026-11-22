package devicefs

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
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
func serveFile(w http.ResponseWriter, r *http.Request, root, name string, uploads *sync.Mutex, guards ...func(string) (func(), error)) {
	if name == "/" {
		failure(w, 400, "invalid_path")
		return
	}
	switch r.Method {
	case "GET", "HEAD":
		f, err := OpenRegular(root, name)
		if err != nil {
			if errors.Is(err, errUnsupported) {
				failure(w, 409, "unsupported_file")
			} else {
				directoryFailure(w, err)
			}
			return
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			directoryFailure(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": path.Base(name)}))
		w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
		releaseNamespace(r)
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
		if len(guards) > 0 && guards[0] != nil {
			release, e := guards[0](name)
			if e != nil {
				davFailure(w, e)
				return
			}
			defer release()
		}
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
		entry, err := uploadContext(r.Context(), d, path.Base(name), r.ContentLength, r.Body)
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
	return uploadContext(context.Background(), d, name, size, body)
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
