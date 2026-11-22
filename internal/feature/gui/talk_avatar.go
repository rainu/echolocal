//go:build !board_biscuit

package gui

import (
	gogui "github.com/go-gui-org/go-gui/gui"
	"github.com/ygelfand/echolocal/internal/avatar"
	"github.com/ygelfand/echolocal/internal/feature/clawtalk"
	"github.com/ygelfand/libcountertop/pkg/display/screens"
	sharedshell "github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
	"github.com/ygelfand/libcountertop/pkg/say"
)

func extendTalkAvatar(_ *widgets.UI, o *screens.Options, _ func() *screens.App) {
	o.Views = append(o.Views, func(view sharedshell.View) *screens.Screen {
		p, ok := view.(*clawtalk.AvatarPage)
		if !ok {
			return nil
		}
		return &screens.Screen{View: p, Fixed: true, Build: func(w *gogui.Window) gogui.View {
			width, height := w.WindowSize()
			size := min(width/2, height-100)
			if size >= 128 {
				p.Start(avatar.Viewport{X: width / 2, Y: 80, Size: size})
			}
			s := p.Renderer.Snapshot()
			text := func(k string) string { return avatar.Text(say.Chosen(), k) }
			rows, _ := p.Build()
			body := []gogui.View{avatarText("OpenClaw")}
			for i := 0; i < min(2, len(rows)); i++ {
				body = append(body, avatarText(rows[i].Label))
				if i == 0 {
					body = append(body, avatarText(rows[i].Hint))
				}
			}
			note := text("talk_loading")
			if s.Ready && s.Running {
				note = text("talk_running")
			}
			if s.Notice != "starting" && s.Notice != "ready" && (!s.Running || s.Notice != "running") {
				note = text(s.Notice) + " " + text("talk_fallback")
			}
			body = append(body, avatarText(note), avatarButton("talk-avatar-stop", say.T("clawtalk.end"), false, false, func() { clawtalk.Get().Stop() }))
			left := gogui.Column(gogui.ContainerCfg{Width: float32(width) / 2, Height: float32(height), Sizing: gogui.FixedFixed, Padding: gogui.PadAll(20), Spacing: gogui.SpacingSmall, Scrollable: true, DragScroll: true, ScrollMode: gogui.ScrollVerticalOnly, Content: body})
			right := gogui.Column(gogui.ContainerCfg{Width: float32(width) / 2, Height: float32(height), Sizing: gogui.FixedFixed, Padding: gogui.PadAll(16), Content: []gogui.View{avatarText(s.ModelName)}})
			return gogui.Row(gogui.ContainerCfg{Width: float32(width), Height: float32(height), Sizing: gogui.FixedFixed, Padding: gogui.NoPadding, Content: []gogui.View{left, right}})
		}}
	})
}
