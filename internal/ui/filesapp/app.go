//go:build !board_biscuit

// Package filesapp is the standalone Files presentation of the shared browser.
package filesapp

import (
	"context"
	"fmt"
	"image"
	"io"
	"os"
	"sync"
	"time"

	gui "github.com/go-gui-org/go-gui/gui"
	"github.com/ygelfand/echolocal/internal/config"
	browser "github.com/ygelfand/echolocal/internal/filepicker"
	"github.com/ygelfand/echolocal/internal/filesmedia"
	"github.com/ygelfand/libcountertop/pkg/display/icons"
	"github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
	"github.com/ygelfand/libcountertop/pkg/say"
	"github.com/ygelfand/libcountertop/pkg/settings/storage"
)

type Options struct {
	Shell       *shell.Shell
	Locations   []browser.Location
	Preferences storage.Section[config.Files]
	Helper      string
	// Open takes ownership of the pinned descriptor, including on error.
	Open func(context.Context, *os.File, string) (io.Closer, error)
}
type App struct {
	o          Options
	mu         sync.Mutex
	b          *browser.Browser
	prefs      config.Files
	busy       bool
	note       string
	generation uint64
	cache      *filesmedia.Cache
	offset     int
	session    io.Closer
	cancel     context.CancelFunc
	navCancel  context.CancelFunc
	imageKeys  map[string]bool
}

func Words(en, de string) string {
	if say.Chosen() == "de" {
		return de
	}
	return en
}
func New(o Options) *App {
	a := &App{o: o, imageKeys: map[string]bool{}}
	// Shell.Close also means "covered". Keep a playing viewer while the
	// existing native video page covers it, but clean up on actual stack removal.
	o.Shell.Changed.Listen(func(c shell.Change) {
		a.Close()
		if c.To == a {
			a.release()
		}
	})
	return a
}
func (*App) Covers() bool           { return true }
func (*App) Timeout() time.Duration { return 30 * time.Minute }
func (a *App) Opened() {
	a.mu.Lock()
	initialized := a.b != nil
	if !initialized {
		a.prefs = a.o.Preferences.Read().Presentation()
	}
	a.mu.Unlock()
	if !initialized {
		a.Navigate("")
	}
}
func (a *App) Close() {
	for _, v := range a.o.Shell.Views() {
		if v == a {
			return
		}
	}

	a.mu.Lock()
	a.generation++
	if a.navCancel != nil {
		a.navCancel()
	}
	if a.cancel != nil {
		a.cancel()
	}
	if a.cache != nil {
		a.cache.Close()
	}
	p := a.session
	a.session = nil
	a.b = nil
	a.dropImages()
	a.mu.Unlock()
	if p != nil {
		p.Close()
	}
}
func (a *App) dropImages() {
	for k := range a.imageKeys {
		gui.DropImage(k)
	}
	clear(a.imageKeys)
}
func (a *App) options() browser.Options {
	return browser.Options{Locations: a.o.Locations, ShowHidden: a.prefs.Hidden, Metadata: true, Sort: a.prefs.Sort, Descending: a.prefs.Descending}
}
func (a *App) Navigate(name string) {
	a.mu.Lock()
	a.generation++
	gen := a.generation
	if a.navCancel != nil {
		a.navCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.navCancel = cancel
	if a.cancel != nil {
		a.cancel()
	}
	opt := a.options()
	opt.Context = ctx
	a.busy = true
	a.note = ""
	a.mu.Unlock()
	a.o.Shell.Redraw()
	go func() {
		b := browser.New(opt)
		e := b.Navigate(name)
		a.mu.Lock()
		if gen != a.generation {
			a.mu.Unlock()
			return
		}
		if e != nil {
			a.note = Words("Folder unavailable or too large. Refresh to retry.", "Ordner nicht verfügbar oder zu groß. Zum Wiederholen aktualisieren.")
			if a.b == nil {
				a.b = browser.New(opt)
			}
		} else {
			a.b = b
			a.offset = 0
			a.dropImages()
			if a.cache != nil {
				a.cache.Close()
			}
			a.cache = filesmedia.NewCache(a.o.Shell.Redraw)
		}
		a.busy = false
		a.mu.Unlock()
		a.o.Shell.Redraw()
	}()
}
func (a *App) Parent() {
	a.mu.Lock()
	if a.b == nil {
		a.mu.Unlock()
		return
	}
	b := browser.New(a.options())
	// Parent is implemented by the shared browser, not a second path policy.
	b.Current = a.b.Current
	name := b.ParentPath()
	a.mu.Unlock()
	a.Navigate(name)
}
func (a *App) Refresh() {
	a.mu.Lock()
	name := ""
	if a.b != nil {
		name = a.b.Current
	}
	a.mu.Unlock()
	a.Navigate(name)
}
func (a *App) preference(change func(*config.Files)) {
	a.mu.Lock()
	next := a.prefs
	change(&next)
	next = next.Presentation()
	a.mu.Unlock()
	if e := a.o.Preferences.Update(func(p *config.Files) { *p = next }); e != nil {
		a.mu.Lock()
		a.note = Words("Preferences could not be saved.", "Anzeige konnte nicht gespeichert werden.")
		a.mu.Unlock()
		a.o.Shell.Redraw()
		return
	}
	a.mu.Lock()
	a.prefs = next
	name := ""
	if a.b != nil {
		name = a.b.Current
	}
	if !browser.New(a.options()).Allowed(name) {
		name = ""
	}
	a.mu.Unlock()
	a.Navigate(name)
}
func (a *App) page(delta int) {
	a.mu.Lock()
	if a.b != nil {
		a.offset = max(0, min(a.offset+delta*browser.PageSize, (max(1, len(a.b.Entries))-1)/browser.PageSize*browser.PageSize))
		if a.cache != nil {
			a.cache.Close()
		}
		a.cache = filesmedia.NewCache(a.o.Shell.Redraw)
		a.dropImages()
	}
	a.mu.Unlock()
	a.o.Shell.Redraw()
}
func (a *App) open(e browser.Entry) {
	a.release()
	if e.Directory {
		a.Navigate(e.Path)
		return
	}
	a.mu.Lock()
	a.generation++
	gen := a.generation
	b := browser.New(a.options())
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.busy = true
	a.note = ""
	a.mu.Unlock()
	a.o.Shell.Redraw()
	go func() {
		var session io.Closer
		f, err := b.Open(e.Path)
		if err == nil {
			if filesmedia.Kind(e.Name) == "" || a.o.Open == nil {
				f.Close()
				err = filesmedia.ErrPreview
			} else {
				session, err = a.o.Open(ctx, f, e.Name)
			}
		}
		a.mu.Lock()
		if gen != a.generation || ctx.Err() != nil {
			a.mu.Unlock()
			if session != nil {
				session.Close()
			}
			return
		}
		a.session = session
		a.busy = false
		if err != nil {
			a.note = Words("Unsupported, damaged or unavailable media.", "Nicht unterstützte, beschädigte oder nicht verfügbare Datei.")
		}
		a.mu.Unlock()
		a.o.Shell.Redraw()
	}()
}

// Leaving a media app returns to this exact browser instance and releases its
// descriptor/player. Covering Files must not destroy its directory or paging.
func (a *App) release() {
	a.mu.Lock()
	p := a.session
	a.session = nil
	a.mu.Unlock()
	if p != nil {
		p.Close()
	}
}

func button(id, label string, disabled bool, do func()) gui.View {
	return gui.Button(gui.ButtonCfg{ID: id, Variant: gui.ButtonGhost, Sizing: gui.FillFit, MinHeight: 56, Height: 56, Padding: gui.PaddingSmall, Disabled: disabled, Content: []gui.View{labelView(label)}, OnClick: func(e gui.EventCtx) { do(); e.Window.InvalidateLayout() }})
}
func iconButton(id, glyph, label string, disabled bool, do func()) gui.View {
	st := widgets.IconStyle(gui.CurrentTheme().Cfg.ColorTextSecondary)
	st.Family = icons.Family
	st.Size = 32
	b := gui.Button(gui.ButtonCfg{ID: id, A11YCfg: gui.A11YCfg{A11YLabel: label}, Variant: gui.ButtonGhost, Sizing: gui.FixedFixed, Width: 56, Height: 56, Disabled: disabled, Content: []gui.View{gui.Label(icons.Glyph(icons.Named(glyph)), st)}, OnClick: func(e gui.EventCtx) { do(); e.Window.InvalidateLayout() }})
	return b
}
func labelView(s string) gui.View {
	return gui.Text(gui.TextCfg{Text: s, Mode: gui.TextModeWrap, TextStyle: widgets.Secondary()})
}
func row(id string, content ...gui.View) gui.View {
	return gui.Row(gui.ContainerCfg{ID: id, Sizing: gui.FillFit, Padding: gui.NoPadding, Spacing: gui.SpacingSmall, Content: content})
}
func (a *App) source(key string, img *image.NRGBA) string {
	a.imageKeys[key] = true
	return gui.UseImage(key, img.Rect.Dx(), img.Rect.Dy(), img.Pix)
}
func (a *App) Build(w *gui.Window) gui.View {
	a.mu.Lock()
	defer a.mu.Unlock()
	b := a.b
	p := a.prefs
	if b == nil {
		return labelView(Words("Loading…", "Wird geladen…"))
	}
	path := b.Current
	if path == "" {
		path = Words("User file locations", "Benutzerdatei-Orte")
	}
	dir := Words("Ascending", "Aufsteigend")
	if p.Descending {
		dir = Words("Descending", "Absteigend")
	}
	sortLabels := map[string]string{"name": Words("Name", "Name"), "modified": Words("Modified", "Geändert"), "size": Words("Size", "Größe"), "type": Words("Type", "Typ")}
	hidden := Words("Hidden: off", "Versteckt: aus")
	if p.Hidden {
		hidden = Words("Hidden: on", "Versteckt: an")
	}
	view := "view-grid"
	viewLabel := Words("Switch to grid view", "Zur Kachelansicht wechseln")
	if p.View == "grid" {
		view = "view-list"
		viewLabel = Words("Switch to list view", "Zur Listenansicht wechseln")
	}
	size := map[int]string{128: Words("Small", "Klein"), 192: Words("Medium", "Mittel"), 256: Words("Large", "Groß")}[p.Tile]
	toolbar := row("files-toolbar",
		iconButton("files-parent", "folder-arrow-up", Words("Parent folder", "Übergeordneter Ordner"), b.Current == "", a.Parent),
		iconButton("files-refresh", "refresh", Words("Refresh folder", "Ordner aktualisieren"), a.busy, a.Refresh),
		iconButton("files-view", view, viewLabel, false, func() {
			a.preference(func(p *config.Files) {
				if p.View == "list" {
					p.View = "grid"
				} else {
					p.View = "list"
				}
			})
		}),
		button("files-size", Words("Tiles: ", "Kacheln: ")+size, false, func() {
			a.preference(func(p *config.Files) {
				switch p.Tile {
				case 128:
					p.Tile = 192
				case 192:
					p.Tile = 256
				default:
					p.Tile = 128
				}
			})
		}),
		gui.Column(gui.ContainerCfg{Sizing: gui.FillFit}),
		iconButton("files-close", "close", Words("Close Files", "Dateien schließen"), false, func() { a.o.Shell.Remove(a) }))
	sorting := row("files-sorting",
		button("files-sort", Words("Sort: ", "Sortierung: ")+sortLabels[p.Sort], false, func() {
			a.preference(func(p *config.Files) {
				keys := []string{"name", "modified", "size", "type"}
				for i, k := range keys {
					if p.Sort == k {
						p.Sort = keys[(i+1)%len(keys)]
						break
					}
				}
			})
		}),
		button("files-direction", dir, false, func() { a.preference(func(p *config.Files) { p.Descending = !p.Descending }) }),
		button("files-hidden", hidden, false, func() { a.preference(func(p *config.Files) { p.Hidden = !p.Hidden }) }))
	var entries []gui.View
	if b.Current == "" {
		for i, l := range b.Locations() {
			l := l
			entries = append(entries, button(fmt.Sprintf("files-location-%d", i), l.Label, false, func() { a.Navigate(l.Path) }))
		}
	} else {
		end := min(a.offset+browser.PageSize, len(b.Entries))
		width, _ := w.WindowSize()
		cols := max(1, (width-32)/(p.Tile+12))
		for i := a.offset; i < end; {
			var tiles []gui.View
			count := 1
			if p.View == "grid" {
				count = cols
			}
			for n := 0; n < count && i < end; n++ {
				e := b.Entries[i]
				id := fmt.Sprintf("files-entry-%d", i)
				i++
				glyph := "file"
				if e.Directory {
					glyph = "folder"
				} else {
					switch filesmedia.Kind(e.Name) {
					case "image":
						glyph = "file-image"
					case "video":
						glyph = "file-video"
					case "audio":
						glyph = "file-music"
					}
				}
				st := widgets.IconStyle(gui.CurrentTheme().Cfg.ColorTextSecondary)
				st.Family = icons.Family
				// The artwork, not just its click target, fills the preview area.
				st.Size = 64
				if p.View == "grid" {
					st.Size = float32(p.Tile - 24)
				}
				var preview gui.View = gui.Text(gui.TextCfg{ID: id + "-fallback", Text: icons.Glyph(icons.Named(glyph)), TextStyle: st})
				if !e.Directory && (filesmedia.Kind(e.Name) == "image" || filesmedia.Kind(e.Name) == "video") && a.cache != nil {
					key := filesmedia.Key(e.Path, e.Size, e.Modified.UnixNano())
					// The generation prevents a GUI texture from surviving an explicit refresh.
					imageKey := fmt.Sprintf("files/%d/%s", a.generation, key)
					img := a.cache.Get(key, e.Name, a.o.Helper, func() (*os.File, error) { return b.Open(e.Path) })
					if img != nil {
						side := float32(56)
						if p.View == "grid" {
							side = float32(p.Tile - 16)
						}
						preview = gui.Image(gui.ImageCfg{ID: id + "-preview", Src: a.source(imageKey, img), Width: side * float32(img.Rect.Dx()) / float32(max(img.Rect.Dx(), img.Rect.Dy())), Height: side * float32(img.Rect.Dy()) / float32(max(img.Rect.Dx(), img.Rect.Dy()))})
					}
				}
				meta := fmt.Sprintf("%s · %d B · %s", e.Type, e.Size, e.Modified.Format("2006-01-02 15:04"))
				if e.Directory {
					meta = Words("Folder", "Ordner")
				}
				if p.View == "grid" {
					preview = gui.Column(gui.ContainerCfg{ID: id + "-thumbnail", Sizing: gui.FixedFixed, Width: float32(p.Tile - 16), Height: float32(p.Tile - 16), VAlign: gui.VAlignMiddle, HAlign: gui.HAlignCenter, Content: []gui.View{preview}})
				}
				content := []gui.View{preview, labelView(e.Name), labelView(meta)}
				cfg := gui.ContainerCfg{ID: id, Sizing: gui.FillFit, MinHeight: 80, Padding: gui.PaddingSmall, Spacing: gui.SpacingSmall, Content: content, OnClick: func(ev gui.EventCtx) { a.open(e); ev.Window.InvalidateLayout(); ev.Consume() }}
				if p.View == "grid" {
					cfg.Width = float32(p.Tile)
					cfg.Sizing = gui.FixedFit
					tiles = append(tiles, gui.Column(cfg))
				} else {
					cfg.Content = []gui.View{gui.Column(gui.ContainerCfg{Width: 72, Sizing: gui.FixedFit, Content: []gui.View{preview}}), gui.Column(gui.ContainerCfg{Sizing: gui.FillFit, Content: []gui.View{labelView(e.Name), labelView(meta)}})}
					tiles = append(tiles, gui.Row(cfg))
				}
			}
			entries = append(entries, row(fmt.Sprintf("files-group-%d", i), tiles...))
		}
		if len(b.Entries) == 0 {
			entries = append(entries, labelView(Words("This folder is empty.", "Dieser Ordner ist leer.")))
		}
	}
	if a.busy {
		entries = append([]gui.View{labelView(Words("Loading…", "Wird geladen…"))}, entries...)
	}
	if a.note != "" {
		entries = append([]gui.View{labelView(a.note)}, entries...)
	}
	content := gui.Column(gui.ContainerCfg{ID: fmt.Sprintf("files-list-%d-%d", a.generation, a.offset), Sizing: gui.FillFill, Scrollable: true, DragScroll: true, ScrollMode: gui.ScrollVerticalOnly, Padding: gui.NoPadding, Spacing: gui.SpacingSmall, Content: entries})
	end := min(a.offset+browser.PageSize, len(b.Entries))
	footer := gui.Row(gui.ContainerCfg{ID: "files-paging", Sizing: gui.FillFit, Content: []gui.View{
		iconButton("files-previous", "chevron-left", Words("Previous page", "Vorherige Seite"), a.offset == 0, func() { a.page(-1) }),
		gui.Column(gui.ContainerCfg{ID: "files-page-indicator", Sizing: gui.FillFit, HAlign: gui.HAlignCenter, Content: []gui.View{labelView(fmt.Sprintf("%d–%d / %d %s", min(a.offset+1, end), end, len(b.Entries), Words("entries", "Einträge")))}}),
		iconButton("files-next", "chevron-right", Words("Next page", "Nächste Seite"), end >= len(b.Entries), func() { a.page(1) }),
	}})
	return gui.Column(gui.ContainerCfg{ID: "files-app", Sizing: gui.FillFill, Padding: gui.PadAll(12), Spacing: gui.SpacingSmall, Content: []gui.View{toolbar, sorting, gui.Column(gui.ContainerCfg{ID: "files-location", Sizing: gui.FillFit, Height: 58, Scrollable: true, Content: []gui.View{labelView(path)}}), content, footer}})
}
