//go:build !board_biscuit

package filesapp

import (
	"fmt"
	gui "github.com/go-gui-org/go-gui/gui"
	"github.com/ygelfand/echolocal/internal/config"
	browser "github.com/ygelfand/echolocal/internal/filepicker"
	"github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/say"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type preferences struct {
	mu sync.Mutex
	p  config.Files
}

func (p *preferences) Read() config.Files { p.mu.Lock(); defer p.mu.Unlock(); return p.p }
func (p *preferences) Update(f func(*config.Files)) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	f(&p.p)
	return nil
}
func settled(t *testing.T, a *App) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		a.mu.Lock()
		busy := a.busy
		a.mu.Unlock()
		if !busy {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("navigation did not settle")
}
func shape(l *gui.Layout, id string) *gui.Shape {
	if l.Shape != nil && l.Shape.ID == id {
		return l.Shape
	}
	for i := range l.Children {
		if s := shape(&l.Children[i], id); s != nil {
			return s
		}
	}
	return nil
}
func TestFilesLayoutsControlsPagingAndPersistence(t *testing.T) {
	old := say.Chosen()
	defer say.Use(old)
	for _, lang := range []string{"en", "de"} {
		say.Use(lang)
		for _, size := range [][2]int{{1280, 800}, {800, 480}, {640, 480}} {
			for _, view := range []string{"list", "grid"} {
				t.Run(fmt.Sprintf("%s-%dx%d-%s", lang, size[0], size[1], view), func(t *testing.T) {
					root := t.TempDir()
					for i := 0; i < 90; i++ {
						os.WriteFile(filepath.Join(root, fmt.Sprintf("%03d-%s.txt", i, strings.Repeat("long name ", 18))), nil, 0600)
					}
					stack := shell.New()
					defer stack.Close()
					prefs := &preferences{p: config.Files{View: view, Tile: 192, Sort: "name"}}
					a := New(Options{Shell: stack, Locations: []browser.Location{{Label: "User files", Path: root}}, Preferences: prefs})
					stack.Push(a)
					a.Navigate(root)
					settled(t, a)
					w := gui.NewWindow(gui.WindowCfg{State: new(int), Width: size[0], Height: size[1]})
					w.SetView(a.Build)
					tree := w.TestRender(nil)
					get := func(id string) *gui.Shape {
						s := shape(tree, id)
						if s == nil {
							t.Fatal("missing", id)
						}
						return s
					}
					for _, id := range []string{"files-parent", "files-refresh", "files-view", "files-size", "files-close", "files-sort", "files-direction", "files-hidden", "files-previous", "files-next"} {
						s := get(id)
						if s.X < 0 || s.Y < 0 || s.X+s.Width > float32(size[0])+.5 || s.Y+s.Height > float32(size[1])+.5 || s.Height < 48 {
							t.Fatal("inaccessible", id, s)
						}
					}
					a.mu.Lock()
					gen := a.generation
					offset := a.offset
					a.mu.Unlock()
					listID := fmt.Sprintf("files-list-%d-%d", gen, offset)
					s := get(listID)
					if !s.Scrollable || s.Height < 100 {
						t.Fatal("unusable list", s)
					}
					bottom := get("files-next").Y
					ids := w.ResolveID(listID)
					if len(ids) != 1 {
						t.Fatal("scroll ID", ids)
					}
					if e := w.TestScroll(ids[0], 0, -100); e != nil {
						t.Fatal(e)
					}
					tree = w.TestRender(nil)
					if get("files-next").Y != bottom {
						t.Fatal("footer scrolled")
					}
					for _, id := range []string{"files-view", "files-sort", "files-direction", "files-hidden", "files-size"} {
						ids := w.ResolveID(id)
						if len(ids) != 1 {
							t.Fatal("control ID", id)
						}
						if e := w.TestClick(ids[0]); e != nil {
							t.Fatal(e)
						}
						settled(t, a)
						tree = w.TestRender(nil)
					}
					a.page(1)
					a.mu.Lock()
					if a.offset != 64 {
						t.Fatal("paging")
					}
					a.mu.Unlock()
					a.preference(func(p *config.Files) { p.Hidden = true; p.Descending = true; p.Sort = "type"; p.Tile = 256 })
					settled(t, a)
					if got := prefs.Read(); !got.Hidden || !got.Descending || got.Sort != "type" || got.Tile != 256 {
						t.Fatal("preferences", got)
					}
					a.Parent()
					settled(t, a)
					a.mu.Lock()
					current := a.b.Current
					a.mu.Unlock()
					if current != "" {
						t.Fatal("parent escaped root", current)
					}
					a.Navigate(filepath.Join(root, "missing"))
					settled(t, a)
					a.mu.Lock()
					notice := a.note
					a.mu.Unlock()
					if notice == "" {
						t.Fatal("unreadable state")
					}
				})
			}
		}
	}
}
func TestUnsupportedOpenAndBackNeverChangesPickerSettings(t *testing.T) {
	root := t.TempDir()
	name := filepath.Join(root, "unsupported.txt")
	os.WriteFile(name, []byte("fixture"), 0600)
	stack := shell.New()
	defer stack.Close()
	prefs := &preferences{}
	a := New(Options{Shell: stack, Locations: []browser.Location{{Label: "files", Path: root}}, Preferences: prefs})
	stack.Push(a)
	a.Navigate(root)
	settled(t, a)
	a.mu.Lock()
	entry := a.b.Entries[0]
	a.mu.Unlock()
	a.open(entry)
	settled(t, a)
	a.mu.Lock()
	note := a.note
	viewer := a.viewer
	a.mu.Unlock()
	if note == "" || viewer == "" {
		t.Fatal("unsupported state")
	}
	a.back()
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.viewer != "" || a.b.Current != root {
		t.Fatal("lost folder")
	}
}

func TestHiddenFolderVisibilityAndCoveredViewerLifecycle(t *testing.T) {
	root := t.TempDir()
	hidden := filepath.Join(root, ".photos")
	os.Mkdir(hidden, 0700)
	stack := shell.New()
	prefs := &preferences{p: config.Files{Hidden: true}}
	a := New(Options{Shell: stack, Locations: []browser.Location{{Label: "files", Path: root}}, Preferences: prefs})
	stack.Push(a)
	a.Navigate(hidden)
	settled(t, a)
	cover := &shell.Page{Title: "native video"}
	stack.Push(cover)
	a.mu.Lock()
	b := a.b
	a.mu.Unlock()
	if b == nil || b.Current != hidden {
		t.Fatal("cover discarded browser or stopped owned viewer")
	}
	stack.Pop()
	a.preference(func(p *config.Files) { p.Hidden = false })
	settled(t, a)
	a.mu.Lock()
	current := a.b.Current
	a.mu.Unlock()
	if current != "" {
		t.Fatal("hidden folder survived toggle", current)
	}
	stack.Push(cover)
	stack.Close()
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.b != nil || a.player != nil {
		t.Fatal("covered app leaked after stack close")
	}
}
