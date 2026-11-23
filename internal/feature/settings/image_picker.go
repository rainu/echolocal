//go:build !board_biscuit

package settings

import (
	"context"
	"path"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/ygelfand/echolocal/internal/avatar"
	"github.com/ygelfand/echolocal/internal/devicefs"
	featureshell "github.com/ygelfand/echolocal/internal/feature/shell"
	"github.com/ygelfand/echolocal/internal/layout"
	"github.com/ygelfand/libcountertop/pkg/display/menu"
	sharedshell "github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/say"
)

type imagePickerState struct {
	name    string
	offset  int
	listing devicefs.Listing
	loading bool
	note    string
}
type imagePickerView struct {
	*sharedshell.Page
	mu     sync.Mutex
	state  imagePickerState
	epoch  uint64
	closed bool
	cancel context.CancelFunc
	list   func(context.Context, string, int) (devicefs.Listing, error)
	choose func(string) error
	redraw func()
}

func (p *imagePickerView) SettingsPage() *sharedshell.Page    { return p.Page }
func (p *imagePickerView) ImagePickerPage() *sharedshell.Page { return p.Page }
func (p *imagePickerView) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	p.epoch++
	if p.cancel != nil {
		p.cancel()
	}
}
func (p *imagePickerView) navigate(name string, offset int) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	if p.cancel != nil {
		p.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.epoch++
	epoch := p.epoch
	p.state = imagePickerState{name: name, offset: offset, loading: true}
	p.mu.Unlock()
	p.redraw()
	go func() {
		listing, err := p.list(ctx, name, offset)
		p.mu.Lock()
		if p.closed || p.epoch != epoch {
			p.mu.Unlock()
			return
		}
		p.state.loading = false
		if err != nil {
			p.state.note = "picker_error"
		} else {
			p.state.listing = listing
			p.state.name = listing.Path
		}
		p.mu.Unlock()
		p.redraw()
	}()
}
func imagePickerStart(current string) string {
	if current != "" && avatar.ValidateBackgroundPath(current) == nil {
		return path.Dir(current)
	}
	return layout.StateDir + "/user-files"
}
func imagePickerEntry(parent string, e devicefs.Entry) (string, bool, bool) {
	if !utf8.ValidString(e.Name) || strings.ContainsAny(e.Name, "\x00\\") {
		return "", false, false
	}
	for _, c := range e.Name {
		if c < 32 || c == 127 {
			return "", false, false
		}
	}
	name := path.Join(parent, e.Name)
	directory := e.Kind == "directory" || e.TargetKind == "directory"
	regular := e.Kind == "file" || e.TargetKind == "file"
	if directory {
		return name, true, true
	}
	ext := strings.ToLower(path.Ext(e.Name))
	return name, false, regular && (ext == ".png" || ext == ".jpg" || ext == ".jpeg") && avatar.ValidateBackgroundPath(name) == nil
}
func newImagePicker(current string, choose func(string) error) *imagePickerView {
	p := &imagePickerView{choose: choose, redraw: featureshell.Get().Redraw, list: func(ctx context.Context, name string, offset int) (devicefs.Listing, error) {
		return devicefs.ListDirectory(ctx, "/", name, offset)
	}}
	p.Page = &sharedshell.Page{Title: avatar.Text(say.Chosen(), "picker_title"), Build: p.build}
	p.navigate(imagePickerStart(current), 0)
	return p
}
func (p *imagePickerView) build() ([]menu.Row, []func(int)) {
	p.mu.Lock()
	resume := p.closed
	p.closed = false
	s := p.state
	p.mu.Unlock()
	if resume {
		p.navigate(s.name, s.offset)
		p.mu.Lock()
		s = p.state
		p.mu.Unlock()
	}
	text := func(k string) string { return avatar.Text(say.Chosen(), k) }
	rows := []menu.Row{}
	acts := []func(int){}
	add := func(row menu.Row, act func(int)) { rows = append(rows, row); acts = append(acts, act) }
	add(menu.Row{Label: text("picker_cancel"), Kind: menu.Plain}, func(int) { featureshell.Get().Pop() })
	add(menu.Row{Label: text("picker_folder"), Value: s.name, Kind: menu.Plain}, nil)
	add(menu.Row{Label: text("picker_uploads"), Kind: menu.Chevron}, func(int) { p.navigate(layout.StateDir+"/user-files", 0) })
	if s.name != "/" {
		add(menu.Row{Label: text("picker_up"), Kind: menu.Chevron}, func(int) { p.navigate(path.Dir(s.name), 0) })
	}
	add(menu.Row{Label: text("picker_refresh"), Kind: menu.Plain}, func(int) { p.navigate(s.name, s.offset) })
	if s.loading {
		add(menu.Row{Label: text("picker_loading"), Kind: menu.Plain}, nil)
		return rows, acts
	}
	if s.note != "" {
		note := text(s.note)
		if note == "" {
			note = s.note
		}
		add(menu.Row{Label: note, Kind: menu.Plain}, nil)
	}
	shown := 0
	for _, e := range s.listing.Entries {
		name, dir, ok := imagePickerEntry(s.name, e)
		if !ok {
			continue
		}
		shown++
		if dir {
			add(menu.Row{Label: e.Name + "/", Kind: menu.Chevron}, func(int) { p.navigate(name, 0) })
		} else {
			tooLarge := e.Kind == "file" && e.Size > avatar.MaxBackgroundBytes
			row := menu.Row{Label: e.Name, Kind: menu.Plain, Dim: tooLarge}
			if tooLarge {
				row.Hint = text("background_invalid")
			}
			add(row, func(int) {
				if tooLarge {
					return
				}
				if err := p.choose(name); err != nil {
					p.mu.Lock()
					p.state.note = err.Error()
					p.mu.Unlock()
					p.redraw()
				} else {
					featureshell.Get().Pop()
				}
			})
		}
	}
	if shown == 0 && s.note == "" {
		add(menu.Row{Label: text("picker_empty"), Kind: menu.Plain}, nil)
	}
	if s.offset > 0 {
		add(menu.Row{Label: text("picker_previous"), Kind: menu.Plain}, func(int) { p.navigate(s.name, max(0, s.offset-256)) })
	}
	if s.listing.NextOffset != nil {
		next := *s.listing.NextOffset
		add(menu.Row{Label: text("picker_next"), Kind: menu.Plain}, func(int) { p.navigate(s.name, next) })
	}
	if s.listing.LimitReached {
		add(menu.Row{Label: text("picker_limit"), Kind: menu.Plain}, nil)
	}
	return rows, acts
}
