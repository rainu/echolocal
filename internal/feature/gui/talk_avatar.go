//go:build !board_biscuit

package gui

import (
	"fmt"
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
		return &screens.Screen{View: p, Fixed: true, Build: func(w *gogui.Window) gogui.View { return talkAvatarView(w, p) }}
	})
}

func talkAvatarView(w *gogui.Window, p *clawtalk.AvatarPage) gogui.View {
	width, height := w.WindowSize()
	size := min(width/2, height-100)
	if size >= 128 {
		p.Start(avatar.Viewport{X: width / 2, Y: 80, Size: size})
	}
	s := p.Renderer.Snapshot()
	text := func(k string) string { return avatar.Text(say.Chosen(), k) }
	rows, _ := p.Build()
	header := []gogui.View{avatarText(p.Title)}
	for i := 0; i < min(2, len(rows)); i++ {
		header = append(header, avatarText(rows[i].Label))
		if i == 0 && rows[i].Hint != "" {
			header = append(header, talkAvatarHint(rows[i].Hint))
		}
	}
	// End-phrase and caption semantics remain available without displacing the
	// live transcript. The stop button below is always visible, not in the scroller.
	if len(rows) > 1 && rows[1].Hint != "" {
		header = append(header, talkAvatarHint(rows[1].Hint))
	}
	note := text("talk_loading")
	if s.Ready && s.Running {
		note = text("talk_running")
	}
	if s.Notice != "starting" && s.Notice != "ready" && (!s.Running || s.Notice != "running") {
		note = text(s.Notice) + " " + text("talk_fallback")
	}
	header = append(header, talkAvatarHint(note))
	captions := []gogui.View{}
	for i := 2; i < len(rows); i++ {
		r := rows[i]
		if r.Hint != "" && (r.Label == say.T("clawtalk.caption_user") || r.Label == say.T("clawtalk.caption_assistant")) {
			captions = append(captions, captionCard(fmt.Sprintf("talk-avatar-caption-%d", i-2), r.Hint, r.Label == say.T("clawtalk.caption_user")))
		} else {
			captions = append(captions, avatarText(r.Label))
		}
	}
	actions := []gogui.View{}
	for i, a := range p.Actions {
		id := "talk-avatar-history"
		if i == len(p.Actions)-1 {
			id = "talk-avatar-stop"
		}
		actions = append(actions, talkAvatarAction(id, a.Label, a.Do, i == len(p.Actions)-1))
	}
	left := gogui.Column(gogui.ContainerCfg{
		ID: "talk-avatar-left", Width: float32(width) / 2, Height: float32(height), Sizing: gogui.FixedFixed,
		Padding: gogui.PadAll(20), Spacing: gogui.SpacingSmall,
		Content: []gogui.View{
			gogui.Column(gogui.ContainerCfg{ID: "talk-avatar-status", Sizing: gogui.FillFit, Padding: gogui.NoPadding, Spacing: gogui.SpacingSmall, Content: header}),
			gogui.Column(gogui.ContainerCfg{ID: "talk-avatar-transcript", Sizing: gogui.FillFill, Padding: gogui.NoPadding, Spacing: gogui.SpacingSmall, Scrollable: true, DragScroll: true, ScrollMode: gogui.ScrollVerticalOnly, Content: captions}),
			gogui.Row(gogui.ContainerCfg{ID: "talk-avatar-actions", Sizing: gogui.FillFit, Padding: gogui.NoPadding, Spacing: gogui.SpacingSmall, Content: actions}),
		},
	})
	right := gogui.Column(gogui.ContainerCfg{Width: float32(width) / 2, Height: float32(height), Sizing: gogui.FixedFixed, Padding: gogui.PadAll(16), Content: []gogui.View{avatarText(s.ModelName)}})
	return gogui.Row(gogui.ContainerCfg{Width: float32(width), Height: float32(height), Sizing: gogui.FixedFixed, Padding: gogui.NoPadding, Content: []gogui.View{left, right}})
}

func talkAvatarHint(text string) gogui.View {
	st := gogui.CurrentTheme().TextStyleDef
	st.Size = 15
	return gogui.Text(gogui.TextCfg{Text: text, TextStyle: st, Mode: gogui.TextModeWrap})
}
func talkAvatarAction(id, label string, do func(), stop bool) gogui.View {
	color := gogui.Hex(0x283C42)
	if stop {
		color = gogui.Hex(0x713B40)
	}
	st := gogui.CurrentTheme().TextStyleDef
	st.Size = 19
	st.Color = gogui.Hex(0xEEF3F4)
	return gogui.Button(gogui.ButtonCfg{
		ID: id, MinHeight: 72, Height: 72, Sizing: gogui.FillFit, Color: color,
		Content: []gogui.View{gogui.Text(gogui.TextCfg{Text: label, TextStyle: st, Mode: gogui.TextModeWrap})},
		OnClick: func(e gogui.EventCtx) {
			if do != nil {
				do()
			}
			e.Window.InvalidateLayout()
		},
	})
}
