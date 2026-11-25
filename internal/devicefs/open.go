package devicefs

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"path"
	"strconv"
	"strings"
	"unicode/utf8"
)

// OpenRegular uses the same trusted root, canonical symlink policy and pinned
// inode as owner browser downloads. It never opens special-file callbacks.
// Callers must authorize the selection; root must never come from user input.
var errUnsupported = errors.New("unsupported_file")

func OpenRegular(root, name string) (*os.File, error) {
	if !utf8.ValidString(name) || len(name) > 4096 || !strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\x00\\") {
		return nil, errors.New("invalid_path")
	}
	for _, part := range strings.Split(name, "/") {
		if part == "." || part == ".." {
			return nil, errors.New("invalid_path")
		}
		for _, c := range part {
			if c < 32 || c == 127 {
				return nil, errors.New("invalid_path")
			}
		}
	}
	resolved, err := resolve(root, name)
	if err != nil {
		return nil, err
	}
	d, err := openDirectory(root, path.Dir(resolved))
	if err != nil {
		return nil, err
	}
	defer d.Close()
	return OpenRegularAt(d, path.Base(resolved))
}

// OpenRegularAt opens a regular basename relative to a pinned directory.
// It rejects symlinks and special files before opening a read descriptor.
func OpenRegularAt(d *os.File, name string) (*os.File, error) {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") {
		return nil, errUnsupported
	}
	pinned, err := unix.Openat(int(d.Fd()), name, unix.O_PATH|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	defer unix.Close(pinned)
	var st unix.Stat_t
	if err = unix.Fstat(pinned, &st); err != nil {
		return nil, err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG {
		return nil, errUnsupported
	}
	fd, err := unix.Open("/proc/self/fd/"+strconv.Itoa(pinned), unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), name)
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		f.Close()
		if err != nil {
			return nil, err
		}
		return nil, errUnsupported
	}
	return f, nil
}
