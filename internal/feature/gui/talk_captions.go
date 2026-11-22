//go:build !board_biscuit

package gui

import (
	"fmt"
	gogui "github.com/go-gui-org/go-gui/gui"
	"github.com/ygelfand/echolocal/internal/feature/clawtalk"
	"github.com/ygelfand/libcountertop/pkg/display/screens"
	sharedshell "github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
	"github.com/ygelfand/libcountertop/pkg/say"
)

// Explicit foreground/background pairs keep transcripts readable in both themes.
func captionColor(user bool) gogui.Color {
	if user {
		return gogui.Hex(0x164E45)
	}
	return gogui.Hex(0x253B68)
}

func extendTalkCaptions(look *widgets.UI, o *screens.Options, _ func() *screens.App) {
	o.Views = append(o.Views, func(view sharedshell.View) *screens.Screen {
		p, ok := view.(*clawtalk.CaptionPage)
		if !ok {
			return nil
		}
		return &screens.Screen{Title: p.Title, View: p, Build: func(w *gogui.Window) gogui.View {
			rows, taps := p.Build()
			body := make([]gogui.View, 0, len(rows))
			for i, r := range rows {
				user := r.Label == say.T("clawtalk.caption_user")
				if r.Hint != "" && (user || r.Label == say.T("clawtalk.caption_assistant")) {
					body = append(body, captionCard(fmt.Sprintf("talk-caption-%d", i), r.Hint, user))
				} else {
					var tap func(int)
					if i < len(taps) {
						tap = taps[i]
					}
					body = append(body, look.Row(fmt.Sprintf("talk-status-%d", i), p.Title, r, tap, false))
				}
			}
			return gogui.Column(gogui.ContainerCfg{Sizing: gogui.FillFit, Padding: gogui.NoPadding, Spacing: gogui.SpacingSmall, Content: body})
		}, Foot: func(w *gogui.Window) gogui.View { return look.Toolkit.ActionBar(w, p.Actions, look.Palette()) }}
	})
}

// captionCard keeps the same readable speaker colors in regular Talk, history
// and the standalone avatar conversation. Text is display-only, never markup.
func captionCard(id, text string, user bool) gogui.View {
	st := gogui.CurrentTheme().TextStyleDef
	st.Color = gogui.White
	return gogui.Column(gogui.ContainerCfg{
		ID: id, Sizing: gogui.FillFit,
		Padding: gogui.PaddingMedium, Radius: gogui.RadiusMedium, Color: captionColor(user),
		Content: []gogui.View{gogui.Text(gogui.TextCfg{Text: text, TextStyle: st, Mode: gogui.TextModeWrap})},
	})
}
