//go:build !board_biscuit

package gui

import (
	"github.com/ygelfand/libcountertop/pkg/display/screens"
	"github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"

	"github.com/ygelfand/echolocal/internal/ui/filepicker"
)

func extendFilePicker(look *widgets.UI, o *screens.Options, _ func() *screens.App) {
	o.Views = append(o.Views, func(view shell.View) *screens.Screen {
		if p, ok := view.(*filepicker.Page); ok {
			return p.Screen(look)
		}
		return nil
	})
}
