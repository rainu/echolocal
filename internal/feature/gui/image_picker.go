//go:build !board_biscuit

package gui

import (
	gogui "github.com/go-gui-org/go-gui/gui"
	"github.com/ygelfand/libcountertop/pkg/display/screens"
	sharedshell "github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
)

// The screens router draws concrete Pages by default. Keep the actual picker
// view as the screen owner so its Close cancels any in-flight directory read.
func extendImagePicker(ui *widgets.UI, o *screens.Options, _ func() *screens.App) {
	o.Views = append(o.Views, func(view sharedshell.View) *screens.Screen {
		provider, ok := view.(interface{ ImagePickerPage() *sharedshell.Page })
		if !ok {
			return nil
		}
		page := provider.ImagePickerPage()
		if page == nil {
			return nil
		}
		return &screens.Screen{View: view, Title: page.Title, Build: func(w *gogui.Window) gogui.View {
			return widgets.PageBody(w, page, widgets.PageRenderer{Row: ui.Row, Beside: widgets.Beside})
		}}
	})
}
