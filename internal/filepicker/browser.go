// Package filepicker implements bounded, read-only selection of user assets.
// It never browses the application's configuration or credential directories.
package filepicker

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/ygelfand/echolocal/internal/devicefs"
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
	Locations  []Location
	Initial    string
	Extensions []string // leading dot, case-insensitive; empty means all regular files
	Directory  bool
}
type Entry struct {
	Name, Path string
	Directory  bool
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
	if !valid(name) || b.location(name) == "" {
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
func valid(name string) bool {
	if !filepath.IsAbs(name) || filepath.Clean(name) != name || !utf8.ValidString(name) ||
		strings.ContainsAny(name, "\\\x00") {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if strings.HasPrefix(part, ".") {
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
	if !valid(name) {
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
	return devicefs.OpenDirectory(root, "/"+strings.TrimPrefix(filepath.ToSlash(rel), "."))
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
		name := filepath.Join(name, item.Name())
		if !valid(name) || item.Type()&os.ModeSymlink != 0 {
			continue
		}
		if item.IsDir() {
			entries = append(entries, Entry{item.Name(), name, true})
		} else if !b.options.Directory && item.Type().IsRegular() && b.accepts(name) {
			entries = append(entries, Entry{item.Name(), name, false})
		}
	}
	slices.SortFunc(entries, func(a, c Entry) int {
		if a.Directory != c.Directory {
			if a.Directory {
				return -1
			}
			return 1
		}
		if n := strings.Compare(strings.ToLower(a.Name), strings.ToLower(c.Name)); n != 0 {
			return n
		}
		return strings.Compare(a.Name, c.Name)
	})
	b.Current, b.Selected, b.Entries, b.Offset = name, "", entries, 0
	return nil
}
func (b *Browser) Parent() {
	if b.Current == "" {
		return
	}
	if b.Current == b.location(b.Current) {
		_ = b.Navigate("")
		return
	}
	_ = b.Navigate(filepath.Dir(b.Current))
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
