//go:build !board_biscuit

// Package filesapp is the standalone Files presentation of the shared browser.
package filesapp

import (
	"context"
	"fmt"
	"image"
	"os"
	"strconv"
	"sync"
	"time"

	gui "github.com/go-gui-org/go-gui/gui"
	"github.com/ygelfand/echolocal/internal/config"
	browser "github.com/ygelfand/echolocal/internal/filepicker"
	"github.com/ygelfand/echolocal/internal/filesmedia"
	"github.com/ygelfand/libcountertop/pkg/display/icons"
	"github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
	"github.com/ygelfand/libcountertop/pkg/media/urlplayer"
	"github.com/ygelfand/libcountertop/pkg/say"
	"github.com/ygelfand/libcountertop/pkg/settings/storage"
)

type Player interface {
	Play() error
	Pause() error
	Stop() error
	Close() error
	Snapshot() urlplayer.Snapshot
}
type Options struct {
	Shell       *shell.Shell
	Locations   []browser.Location
	Preferences storage.Section[config.Files]
	Helper      string
	Play        func(*os.File, string) (Player, error)
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
	viewer     string
	picture    *image.NRGBA
	player     Player
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
	o.Shell.Changed.Listen(func(shell.Change) { a.Close() })
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
	p := a.player
	a.player = nil
	a.b = nil
	a.picture = nil
	a.viewer = ""
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
	a.viewer = e.Name
	a.busy = true
	a.note = ""
	a.mu.Unlock()
	a.o.Shell.Redraw()
	go func() {
		var img *image.NRGBA
		var p Player
		f, err := b.Open(e.Path)
		if err == nil {
			switch filesmedia.Kind(e.Name) {
			case "image":
				img, err = filesmedia.ImageContext(ctx, f, 1280, 800)
				f.Close()
			case "video", "audio":
				if a.o.Play != nil {
					a.mu.Lock()
					if gen == a.generation && ctx.Err() == nil {
						p, err = a.o.Play(f, e.Name)
					} else {
						err = filesmedia.ErrPreview
					}
					a.mu.Unlock()
				} else {
					err = filesmedia.ErrPreview
				}
				if err != nil {
					f.Close()
				}
			default:
				f.Close()
				err = filesmedia.ErrPreview
			}
		}
		a.mu.Lock()
		if gen != a.generation || ctx.Err() != nil {
			a.mu.Unlock()
			if p != nil {
				p.Close()
			}
			return
		}
		a.picture = img
		a.player = p
		a.busy = false
		if err != nil {
			a.note = Words("Unsupported, damaged or unavailable media. No external handler is required.", "Nicht unterstützte, beschädigte oder nicht verfügbare Datei. Kein externer Player erforderlich.")
		}
		a.mu.Unlock()
		a.o.Shell.Redraw()
	}()
}
func (a *App) back() {
	a.mu.Lock()
	a.generation++
	if a.cancel != nil {
		a.cancel()
	}
	p := a.player
	a.player = nil
	a.picture = nil
	a.viewer = ""
	a.note = ""
	a.busy = false
	a.dropImages()
	a.mu.Unlock()
	if p != nil {
		p.Close()
	}
	a.o.Shell.Redraw()
}
func button(id, label string, disabled bool, do func()) gui.View {
	return gui.Button(gui.ButtonCfg{ID: id, Variant: gui.ButtonGhost, Sizing: gui.FillFit, MinHeight: 56, Height: 56, Padding: gui.PaddingSmall, Disabled: disabled, Content: []gui.View{labelView(label)}, OnClick: func(e gui.EventCtx) { do(); e.Window.InvalidateLayout() }})
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
	if a.viewer != "" {
		return a.buildViewer(w)
	}
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
	view := Words("List", "Liste")
	if p.View == "grid" {
		view = Words("Grid", "Kacheln")
	}
	toolbar := row("files-toolbar",
		button("files-parent", Words("Up", "Hoch"), b.Current == "", a.Parent),
		button("files-refresh", Words("Refresh", "Aktualisieren"), a.busy, a.Refresh),
		button("files-view", view, false, func() {
			a.preference(func(p *config.Files) {
				if p.View == "list" {
					p.View = "grid"
				} else {
					p.View = "list"
				}
			})
		}),
		button("files-size", strconv.Itoa(p.Tile)+" px", false, func() {
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
		button("files-close", Words("Close", "Schließen"), false, func() { a.o.Shell.Remove(a) }))
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
				icon := Words("File", "Datei")
				if e.Directory {
					icon = Words("Folder", "Ordner")
				} else {
					switch filesmedia.Kind(e.Name) {
					case "image":
						icon = Words("Image", "Bild")
					case "video":
						icon = "Video"
					case "audio":
						icon = "Audio"
					}
				}
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
				var preview gui.View = gui.Column(gui.ContainerCfg{Sizing: gui.FitFit, Content: []gui.View{gui.Label(icons.Glyph(icons.Named(glyph)), st), labelView(icon)}})
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
	footer := row("files-paging", button("files-previous", Words("Previous", "Zurück"), a.offset == 0, func() { a.page(-1) }), labelView(fmt.Sprintf("%d–%d / %d", min(a.offset+1, end), end, len(b.Entries))), button("files-next", Words("Next", "Weiter"), end >= len(b.Entries), func() { a.page(1) }))
	return gui.Column(gui.ContainerCfg{ID: "files-app", Sizing: gui.FillFill, Padding: gui.PadAll(12), Spacing: gui.SpacingSmall, Content: []gui.View{toolbar, sorting, gui.Column(gui.ContainerCfg{ID: "files-location", Sizing: gui.FillFit, Height: 58, Scrollable: true, Content: []gui.View{labelView(path)}}), content, footer}})
}
func (a *App) buildViewer(w *gui.Window) gui.View {
	controls := []gui.View{button("files-back", Words("Back to files", "Zurück zu Dateien"), false, a.back)}
	body := []gui.View{labelView(a.viewer)}
	if a.picture != nil {
		width, height := w.WindowSize()
		img := a.picture
		iw, ih := img.Rect.Dx(), img.Rect.Dy()
		if iw > width-32 {
			ih = max(1, ih*(width-32)/iw)
			iw = width - 32
		}
		if ih > height-150 {
			iw = max(1, iw*(height-150)/ih)
			ih = height - 150
		}
		key := fmt.Sprintf("files/viewer/%d", a.generation)
		body = append(body, gui.Image(gui.ImageCfg{ID: "files-image", Src: a.source(key, img), Width: float32(iw), Height: float32(ih)}))
	}
	if a.busy {
		body = append(body, labelView(Words("Opening…", "Wird geöffnet…")))
	}
	if a.note != "" {
		body = append(body, labelView(a.note))
	}
	if a.player != nil {
		p := a.player
		s := p.Snapshot()
		state := s.State
		if s.State == "error" {
			state = Words("Playback unavailable: unsupported or damaged media, or decoder missing.", "Wiedergabe nicht verfügbar: Format beschädigt/nicht unterstützt oder Decoder fehlt.")
		}
		body = append(body, labelView(fmt.Sprintf("%s · %.0f / %.0f s", state, s.Position, s.Duration)))
		controls = append(controls, button("files-play", Words("Play", "Abspielen"), false, func() { _ = p.Play(); a.o.Shell.Redraw() }), button("files-pause", Words("Pause", "Pause"), false, func() { _ = p.Pause(); a.o.Shell.Redraw() }), button("files-stop", Words("Stop", "Stopp"), false, func() { _ = p.Stop(); a.o.Shell.Redraw() }))
		time.AfterFunc(250*time.Millisecond, a.o.Shell.Redraw)
	}
	return gui.Column(gui.ContainerCfg{ID: "files-viewer", Sizing: gui.FillFill, Padding: gui.PadAll(12), Content: []gui.View{row("files-viewer-controls", controls...), gui.Column(gui.ContainerCfg{Sizing: gui.FillFill, Scrollable: true, DragScroll: true, Content: body})}})
}
