// Package filepicker implements bounded, read-only selection of user assets.
// It never browses the application's configuration or credential directories.
package filepicker

import (
	"cmp"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ygelfand/echolocal/internal/devicefs"
	"golang.org/x/sys/unix"
)

const PageSize = 64
const maxEntries = 65536

var ErrUnavailable = errors.New("unavailable")
var ErrLimit = errors.New("too_many_entries")

type Location struct {
	Label string
	Path  string
}
type Options struct {
	Context    context.Context
	Locations  []Location
	Initial    string
	Extensions []string // leading dot, case-insensitive; empty means all regular files
	Directory  bool
	ShowHidden bool
	Metadata   bool
	Sort       string
	Descending bool
}
type Entry struct {
	Name, Path string
	Directory  bool
	Size       int64
	Modified   time.Time
	Type       string
}

type Browser struct {
	options           Options
	Current, Selected string
	Entries           []Entry
	Offset            int
	Err               error
}

func New(options Options) *Browser {
	options.Locations = slices.Clone(options.Locations)
	options.Extensions = slices.Clone(options.Extensions)
	return &Browser{options: options}
}
func (b *Browser) Locations() []Location { return slices.Clone(b.options.Locations) }

// Start is explicit: constructing a settings search index must not read disks.
func (b *Browser) Start() {
	if b.options.Initial == "" {
		return
	}
	name := b.options.Initial
	if !b.valid(name) || b.location(name) == "" {
		b.Err = ErrUnavailable
		return
	}
	info, err := os.Lstat(name)
	if err == nil && info.IsDir() {
		_ = b.Navigate(name)
		return
	}
	parent := filepath.Dir(name)
	for b.location(parent) != "" {
		if b.Navigate(parent) == nil {
			if err == nil && info.Mode().IsRegular() && b.accepts(name) {
				b.Selected = name
				for i, e := range b.Entries {
					if e.Path == name {
						b.Offset = i / PageSize * PageSize
						break
					}
				}
			} else {
				b.Err = ErrUnavailable
			}
			return
		}
		parent = filepath.Dir(parent)
	}
	b.Current, b.Entries, b.Selected, b.Offset = "", nil, "", 0
	b.Err = ErrUnavailable
}
func valid(name string) bool { return validPath(name, false) }
func validPath(name string, hidden bool) bool {
	if !filepath.IsAbs(name) || filepath.Clean(name) != name || !utf8.ValidString(name) ||
		strings.ContainsAny(name, "\\\x00") {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if (!hidden && strings.HasPrefix(part, ".")) || privateName(part) {
			return false
		}
		for _, r := range part {
			if r < 32 || r == 127 {
				return false
			}
		}
	}
	return true
}
func (b *Browser) location(name string) string {
	if !b.valid(name) {
		return ""
	}
	for _, loc := range b.options.Locations {
		if name == loc.Path || strings.HasPrefix(name, loc.Path+"/") {
			return loc.Path
		}
	}
	return ""
}
func (b *Browser) accepts(name string) bool {
	return len(b.options.Extensions) == 0 ||
		slices.ContainsFunc(
			b.options.Extensions,
			func(ext string) bool { return strings.EqualFold(filepath.Ext(name), ext) },
		)
}
func (b *Browser) openDir(name string) (*os.File, error) {
	root := b.location(name)
	if root == "" {
		return nil, ErrUnavailable
	}
	rel, err := filepath.Rel(root, name)
	if err != nil {
		return nil, ErrUnavailable
	}
	if rel == "." {
		rel = ""
	}
	return devicefs.OpenDirectory(root, "/"+filepath.ToSlash(rel))
}
func (b *Browser) Navigate(name string) error {
	b.Err = nil
	if name == "" {
		b.Current, b.Selected, b.Entries, b.Offset = "", "", nil, 0
		return nil
	}
	d, err := b.openDir(name)
	if err != nil {
		b.Err = ErrUnavailable
		return b.Err
	}
	defer d.Close()
	list, err := d.ReadDir(maxEntries + 1)
	if err != nil && err != io.EOF {
		b.Err = ErrUnavailable
		return b.Err
	}
	if len(list) > maxEntries {
		b.Err = ErrLimit
		return b.Err
	}
	var entries []Entry
	for _, item := range list {
		if b.options.Context != nil && b.options.Context.Err() != nil {
			b.Err = ErrUnavailable
			return b.Err
		}
		name := filepath.Join(name, item.Name())
		if !b.valid(name) || item.Type()&os.ModeSymlink != 0 {
			continue
		}
		if !item.IsDir() && (b.options.Directory || !item.Type().IsRegular() || !b.accepts(name)) {
			continue
		}
		entry := Entry{Name: item.Name(), Path: name, Directory: item.IsDir(), Type: strings.ToLower(filepath.Ext(name))}
		if b.options.Metadata {
			var st unix.Stat_t
			if err := unix.Fstatat(int(d.Fd()), item.Name(), &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
				continue
			}
			mode := st.Mode & unix.S_IFMT
			if (entry.Directory && mode != unix.S_IFDIR) || (!entry.Directory && mode != unix.S_IFREG) {
				continue
			}
			entry.Size, entry.Modified = st.Size, time.Unix(int64(st.Mtim.Sec), int64(st.Mtim.Nsec))
		}
		entries = append(entries, entry)
	}
	Sort(entries, b.options.Sort, b.options.Descending)
	b.Current, b.Selected, b.Entries, b.Offset = name, "", entries, 0
	return nil
}
func (b *Browser) ParentPath() string {
	if b.Current == "" || b.Current == b.location(b.Current) {
		return ""
	}
	return filepath.Dir(b.Current)
}
func (b *Browser) Parent() {
	if b.Current == "" {
		return
	}
	if b.Current == b.location(b.Current) {
		_ = b.Navigate("")
		return
	}
	_ = b.Navigate(b.ParentPath())
}
func (b *Browser) Select(name string) {
	b.Err = nil
	b.Selected = ""
	if b.options.Directory {
		return
	}
	for _, e := range b.Entries {
		if e.Path == name && !e.Directory {
			b.Selected = name
			return
		}
	}
}
func (b *Browser) Candidate() string {
	if b.options.Directory {
		return b.Current
	}
	return b.Selected
}

// Confirm reopens the target through pinned, no-symlink directory descriptors.
// A removed/replaced entry cannot become a credential, pipe or escaped path.
func (b *Browser) Confirm() (string, error) {
	name := b.Candidate()
	if name == "" {
		return "", ErrUnavailable
	}
	if b.options.Directory {
		d, err := b.openDir(name)
		if err != nil {
			return "", ErrUnavailable
		}
		d.Close()
		return name, nil
	}
	if !b.accepts(name) {
		return "", ErrUnavailable
	}
	d, err := b.openDir(filepath.Dir(name))
	if err != nil {
		return "", ErrUnavailable
	}
	defer d.Close()
	f, err := devicefs.OpenRegularAt(d, filepath.Base(name))
	if err != nil {
		return "", ErrUnavailable
	}
	f.Close()
	return name, nil
}

// valid applies caller visibility without weakening the immutable private-name policy.
func (b *Browser) valid(name string) bool { return validPath(name, b.options.ShowHidden) }

func privateName(name string) bool {
	name = strings.ToLower(name)
	switch name {
	case ".ssh", ".gnupg", ".android", ".aws", ".kube", ".config", "android",
		".env", "authorized_keys", "adb_keys", "id_rsa", "id_ed25519",
		"credentials", "credentials.json", "cast-credentials.json":
		return true
	}
	return strings.HasPrefix(name, ".env.") || strings.HasSuffix(name, ".key") ||
		strings.HasSuffix(name, ".pem")
}

// Sort is shared by the picker and Files app. Folders always precede files;
// name is a deterministic tie-breaker in either direction.
func Sort(entries []Entry, key string, descending bool) {
	slices.SortFunc(entries, func(a, b Entry) int {
		if a.Directory != b.Directory {
			if a.Directory {
				return -1
			}
			return 1
		}
		n := 0
		switch key {
		case "modified":
			n = a.Modified.Compare(b.Modified)
		case "size":
			n = cmp.Compare(a.Size, b.Size)
		case "type":
			n = strings.Compare(a.Type, b.Type)
		}
		if n == 0 {
			n = strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
		}
		if n == 0 {
			n = strings.Compare(a.Name, b.Name)
		}
		if descending {
			n = -n
		}
		return n
	})
}

// Open pins an allowed regular file. Consumers retain this descriptor rather
// than checking a path and later reopening it with broader filesystem rights.
func (b *Browser) Open(name string) (*os.File, error) {
	if !b.accepts(name) {
		return nil, ErrUnavailable
	}
	d, err := b.openDir(filepath.Dir(name))
	if err != nil || !b.valid(name) {
		if d != nil {
			d.Close()
		}
		return nil, ErrUnavailable
	}
	defer d.Close()
	return devicefs.OpenRegularAt(d, filepath.Base(name))
}

// Allowed reports the shared lexical/root policy, without reading storage.
func (b *Browser) Allowed(name string) bool { return b.location(name) != "" }
