// Package devicefs provides owner-only real filesystem browsing and transfers.
// It must only be mounted in the owner-authenticated HTTPS API.
package devicefs

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"
)

const pageSize = 256
const maxOffset = 65536 - pageSize

type Entry struct {
	Name       string    `json:"name"`
	Kind       string    `json:"kind"`
	Size       int64     `json:"size"`
	Modified   time.Time `json:"modified"`
	Mode       string    `json:"mode"`
	TargetKind string    `json:"targetKind,omitempty"`
	TargetPath string    `json:"targetPath,omitempty"`
}
type Listing struct {
	AvailableBytes int64   `json:"availableBytes"`
	MaxUploadBytes int64   `json:"maxUploadBytes"`
	ReadOnly       bool    `json:"readOnly"`
	CapacityKnown  bool    `json:"capacityKnown"`
	LimitReached   bool    `json:"limitReached,omitempty"`
	Path           string  `json:"path"`
	Parent         string  `json:"parent"`
	Entries        []Entry `json:"entries"`
	NextOffset     *int    `json:"nextOffset,omitempty"`
}

// Handler browses the actual filesystem and streams owner-authorized transfers.
// Uploads are create-only; no remount, overwrite, delete or execute operation.
func Handler() http.Handler { return handler("/") }

// handler takes a trusted root only for isolated fixture tests, never from HTTP.
func handler(root string) http.Handler {
	var uploads sync.Mutex
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		query := r.URL.Query()
		name := query.Get("path")
		if name == "" {
			name = "/"
		}
		if len(name) > 4096 || !utf8.ValidString(name) || !strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\x00\\") {
			failure(w, 400, "invalid_path")
			return
		}
		for _, part := range strings.Split(name, "/") {
			if part == "." || part == ".." {
				failure(w, 400, "invalid_path")
				return
			}
			for _, c := range part {
				if c < 32 || c == 127 {
					failure(w, 400, "invalid_path")
					return
				}
			}
		}
		name = path.Clean(name)
		if r.URL.Path == "/file" {
			serveFile(w, r, root, name, &uploads)
			return
		}
		if r.URL.Path != "" && r.URL.Path != "/" {
			failure(w, 404, "not_found")
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			failure(w, 405, "method_not_allowed")
			return
		}
		resolved, err := resolve(root, name)
		if err != nil {
			directoryFailure(w, err)
			return
		}
		name = resolved
		offset := 0
		if raw := query.Get("offset"); raw != "" {
			var err error
			offset, err = strconv.Atoi(raw)
			if err != nil || offset < 0 || offset > maxOffset || offset%pageSize != 0 {
				failure(w, 400, "invalid_offset")
				return
			}
		}
		d, err := openDirectory(root, name)
		if err != nil {
			directoryFailure(w, err)
			return
		}
		defer d.Close()
		for skipped := 0; skipped < offset; skipped += pageSize {
			entries, e := d.Readdir(pageSize)
			if e != nil && e != io.EOF {
				directoryFailure(w, e)
				return
			}
			if len(entries) < pageSize {
				empty := Listing{Path: name, Parent: parent(name), Entries: []Entry{}}
				fillCapacity(d, &empty)
				respond(w, 200, empty)
				return
			}
		}
		infos, err := d.Readdir(pageSize + 1)
		if err != nil && err != io.EOF {
			directoryFailure(w, err)
			return
		}
		listing := Listing{Path: name, Parent: parent(name), Entries: []Entry{}}
		fillCapacity(d, &listing)
		if len(infos) > pageSize {
			infos = infos[:pageSize]
			next := offset + pageSize
			if next > maxOffset {
				listing.LimitReached = true
			} else {
				listing.NextOffset = &next
			}
		}
		for _, info := range infos {
			kind := "special"
			switch {
			case info.Mode()&os.ModeSymlink != 0:
				kind = "symlink"
			case info.IsDir():
				kind = "directory"
			case info.Mode().IsRegular():
				kind = "file"
			}
			entry := Entry{Name: info.Name(), Kind: kind, Size: info.Size(), Modified: info.ModTime().UTC(), Mode: info.Mode().String()}
			if kind == "symlink" {
				target, e := resolve(root, path.Join(name, info.Name()))
				if e == nil {
					st, e := os.Lstat(filepath.Join(root, target))
					if e == nil && (st.IsDir() || st.Mode().IsRegular()) {
						entry.TargetPath = target
						if st.IsDir() {
							entry.TargetKind = "directory"
						} else {
							entry.TargetKind = "file"
						}
					}
				}
			}
			listing.Entries = append(listing.Entries, entry)
		}
		// Sort each bounded page; page order follows the underlying directory stream.
		slices.SortFunc(listing.Entries, func(a, b Entry) int {
			if (a.Kind == "directory") != (b.Kind == "directory") {
				if a.Kind == "directory" {
					return -1
				}
				return 1
			}
			return strings.Compare(a.Name, b.Name)
		})
		respond(w, 200, listing)
	})
}
func parent(name string) string {
	if name == "/" {
		return ""
	}
	return path.Dir(name)
}

// Pin every directory component; no symlinks (including intermediate ones),
// magic links, devices, pipes or regular files can be opened as directories.
func openDirectory(root, name string) (*os.File, error) {
	fd, err := syscall.Open(root, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	for _, part := range strings.Split(strings.TrimPrefix(name, "/"), "/") {
		if part == "" {
			continue
		}
		next, e := syscall.Openat(fd, part, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		syscall.Close(fd)
		if e != nil {
			return nil, e
		}
		fd = next
	}
	return os.NewFile(uintptr(fd), name), nil
}
func directoryFailure(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, syscall.EROFS):
		failure(w, 403, "read_only_filesystem")
	case errors.Is(err, fs.ErrPermission):
		failure(w, 403, "permission_denied")
	case errors.Is(err, fs.ErrNotExist):
		failure(w, 404, "not_found")
	case errors.Is(err, errOutside):
		failure(w, 403, "outside_root")
	case errors.Is(err, syscall.ENAMETOOLONG):
		failure(w, 400, "invalid_path")
	case errors.Is(err, syscall.ENOTDIR), errors.Is(err, syscall.ELOOP):
		failure(w, 409, "not_directory")
	default:
		failure(w, 500, "listing_failed")
	}
}
func failure(w http.ResponseWriter, status int, code string) {
	respond(w, status, map[string]string{"error": code})
}
func respond(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// resolve follows normal filesystem symlinks, then maps the canonical path into
// the trusted root. Production root is /. Tests use an isolated fixture root.
// Subsequent opens pin each canonical component with O_NOFOLLOW, so a symlink
// replacement after resolution cannot redirect the descriptor.
var errOutside = errors.New("outside_root")

func resolve(root, name string) (string, error) {
	real, err := filepath.EvalSymlinks(filepath.Join(root, name))
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, real)
	if err != nil || relative == ".." || strings.HasPrefix(relative, "../") {
		return "", errOutside
	}
	if relative == "." {
		return "/", nil
	}
	return "/" + filepath.ToSlash(relative), nil
}
