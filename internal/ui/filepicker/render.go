//go:build !board_biscuit

package filepicker

import (
	gui "github.com/go-gui-org/go-gui/gui"

	"github.com/ygelfand/libcountertop/pkg/display/screens"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
)

// Screen uses the standard page scroller, with status and actions in its fixed footer.
func (p *Page) Screen(look *widgets.UI) *screens.Screen {
	return &screens.Screen{
		Title: p.Title,
		View:  p,
		Build: func(w *gui.Window) gui.View {
			return widgets.PageBody(w, p.Page, widgets.PageRenderer{Row: look.Row})
		},
		Foot: func(w *gui.Window) gui.View {
			return gui.Column(gui.ContainerCfg{
				ID:     "picker-footer",
				Sizing: gui.FillFit, Padding: gui.NoPadding, Spacing: gui.SpacingSmall,
				Content: []gui.View{
					gui.Text(gui.TextCfg{ID: "picker-filter", Text: p.Status(),
						TextStyle: widgets.Secondary(), Mode: gui.TextModeWrap, Sizing: gui.FillFit}),
					look.Toolkit.ActionBar(w, p.Actions, look.Palette()),
				},
			})
		},
	}
}
