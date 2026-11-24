package userfiles

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// A DAV mutation works in a bounded, hard-linked snapshot. Only changed file
// contents consume additional space; writes always use new staging inodes.
// Existing readers keep their open descriptors. The shared mutation slot keeps
// API writers out until commit/abort. The two-rename journal recovers to a whole
// old or new tree after a crash, never a partial collection COPY/MOVE.
type treeTransaction struct {
	live, work           *Store
	parent               *os.Root
	base, temp, previous string
	committed            bool
}

func journalNames(p string) (base, temp, previous string) {
	base = filepath.Base(p)
	return base, ".dav-" + base + "-transaction", ".dav-" + base + "-previous"
}
func recoverTree(p string) error {
	base, temp, previous := journalNames(p)
	parent, e := os.OpenRoot(filepath.Dir(p))
	if errors.Is(e, fs.ErrNotExist) {
		return nil
	}
	if e != nil {
		return e
	}
	defer parent.Close()
	if _, e = parent.Lstat(base); errors.Is(e, fs.ErrNotExist) {
		if old, e := parent.Lstat(previous); e == nil {
			if !old.IsDir() || old.Mode()&os.ModeSymlink != 0 {
				return ErrBoundary
			}
			if e = parent.Rename(previous, base); e != nil {
				return e
			}
		}
	}
	for _, n := range []string{temp, previous} {
		if e = parent.RemoveAll(n); e != nil {
			return e
		}
	}
	return nil
}
func (s *Store) beginTree(ctx context.Context) (*treeTransaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, e := s.view(); e != nil {
		return nil, e
	}
	parent, e := os.OpenRoot(filepath.Dir(s.path))
	if e != nil {
		return nil, e
	}
	base, temp, previous := journalNames(s.path)
	t := &treeTransaction{live: s, parent: parent, base: base, temp: temp, previous: previous}
	if e = parent.Mkdir(temp, 0700); e != nil {
		parent.Close()
		return nil, e
	}
	ok := false
	defer func() {
		if !ok {
			parent.RemoveAll(temp)
			parent.Close()
		}
	}()
	e = s.scan(".", func(n string, st os.FileInfo) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		if st.IsDir() {
			return parent.Mkdir(temp+"/"+n, 0700)
		}
		return parent.Link(base+"/"+n, temp+"/"+n)
	})
	if e != nil {
		return nil, e
	}
	if _, e = parent.Lstat(base + "/.dav-properties"); e == nil {
		if e = parent.Link(base+"/.dav-properties", temp+"/.dav-properties"); e != nil {
			return nil, e
		}
	} else if !errors.Is(e, fs.ErrNotExist) {
		return nil, e
	}
	w := New(filepath.Join(filepath.Dir(s.path), temp))
	w.budget = s.budget
	w.maxFile = s.maxFile
	w.reserve = s.reserve
	w.free = s.free
	if e = w.init(); e != nil {
		return nil, e
	}
	w.dav.locks = s.dav.locks
	t.work = w
	ok = true
	return t, nil
}
func (t *treeTransaction) abort() {
	if !t.committed && t.work != nil && t.work.root != nil {
		t.work.root.Close()
	}
	t.parent.RemoveAll(t.temp)
	t.parent.Close()
}
func (t *treeTransaction) commit(ctx context.Context) error {
	s := t.live
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := ctx.Err(); e != nil {
		return e
	}
	// Flush namespace changes before making the new tree authoritative.
	if e := t.work.scan(".", func(n string, st os.FileInfo) error {
		if !st.IsDir() {
			return nil
		}
		f, e := t.work.openTree(n)
		if e != nil {
			return e
		}
		defer f.Close()
		return f.Sync()
	}); e != nil {
		return e
	}
	d, e := t.work.root.Open(".")
	if e != nil {
		return e
	}
	e = d.Sync()
	d.Close()
	if e != nil {
		return e
	}
	if e = t.parent.Rename(t.base, t.previous); e != nil {
		return e
	}
	if e = t.parent.Rename(t.temp, t.base); e != nil {
		t.parent.Rename(t.previous, t.base)
		return e
	}
	// All publication readers use this same mutex, so they see one complete tree.
	old := s.root
	s.root = t.work.root
	s.dav.props = t.work.dav.props
	old.Close()
	t.committed = true
	t.work.root = nil
	pd, e := t.parent.Open(".")
	if e == nil {
		e = pd.Sync()
		pd.Close()
	}
	if e != nil {
		return e
	} // keep previous for journal recovery if durability failed
	return t.parent.RemoveAll(t.previous)
}
