//go:build !board_biscuit

package gui

import (
	"fmt"
	gogui "github.com/go-gui-org/go-gui/gui"
	"github.com/ygelfand/echolocal/internal/feature/settings"
	"github.com/ygelfand/libcountertop/pkg/display/menu"
	"github.com/ygelfand/libcountertop/pkg/display/screens"
	sharedshell "github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
)

const talkPromptInputID = "talk-device-prompt-multiline"

func talkPromptFieldID(field int) string {
	if field == 0 {
		return talkPromptInputID
	}
	return fmt.Sprintf("%s-%d", talkPromptInputID, field)
}
func talkPromptInput(row menu.Row, height float32) gogui.InputCfg {
	return gogui.InputCfg{ID: talkPromptInputID, Text: row.Value, Mode: gogui.InputMultiline, Scrollable: true, Height: height, Sizing: gogui.FillFixed,
		OnTextChanged: func(text string, _ gogui.EventCtx) {
			if row.Save != nil {
				row.Save(text)
			}
		},
	}
}
func extendTalkPrompt(look *widgets.UI, o *screens.Options, _ func() *screens.App) {
	o.Views = append(o.Views, func(view sharedshell.View) *screens.Screen {
		p, ok := view.(*settings.TalkPromptEditor)
		if !ok {
			return nil
		}
		return &screens.Screen{Title: p.Title, View: p, Build: func(w *gogui.Window) gogui.View {
			rows, taps := p.Build()
			if len(rows) == 0 {
				return nil
			}
			height := float32(120)
			if look.Editor.Typing(w) {
				height = 100
			}
			body := []gogui.View{}
			if p.Leading != nil {
				leading, actions := p.Leading()
				for i, row := range leading {
					var tap func(int)
					if i < len(actions) {
						tap = actions[i]
					}
					body = append(body, look.Row(fmt.Sprintf("context-row-%d", i), p.Title, row, tap, false))
				}
			}
			field := 0
			for i, row := range rows {
				if row.Kind != menu.Field {
					var tap func(int)
					if i < len(taps) {
						tap = taps[i]
					}
					body = append(body, look.Row(fmt.Sprintf("prompt-row-%d", i), p.Title, row, tap, false))
					continue
				}
				id := talkPromptFieldID(field)
				field++
				cfg := talkPromptInput(row, height)
				cfg.ID = id
				input := look.Input(cfg, func() {})
				// Each toolbar restores its own input focus and cursor, never the other
				// prompt's. Field IDs do not change when validation notices appear.
				newline := look.Toolkit.ActionBar(w, []sharedshell.Action{{Label: p.NewlineLabel(), MinHeight: 48, Role: sharedshell.Plain, Do: func() {
					w.QueueCommand(func(w *gogui.Window) {
						for _, resolved := range w.ResolveID(id) {
							w.SetFocus(resolved)
						}
						w.EventFn(&gogui.Event{Type: gogui.EventKeyDown, KeyCode: gogui.KeyEnter})
						w.EventFn(&gogui.Event{Type: gogui.EventKeyUp, KeyCode: gogui.KeyEnter})
						w.InvalidateLayout()
					})
				}}}, look.Palette())
				body = append(body, gogui.Column(gogui.ContainerCfg{Sizing: gogui.FillFit, Padding: gogui.PaddingMedium, Spacing: gogui.SpacingSmall, Content: []gogui.View{
					gogui.Text(gogui.TextCfg{Text: row.Label, Mode: gogui.TextModeWrap}),
					gogui.Text(gogui.TextCfg{Text: row.Hint, TextStyle: widgets.Secondary(), Mode: gogui.TextModeWrap}),
					newline, input,
				}}))
			}
			return gogui.Column(gogui.ContainerCfg{Sizing: gogui.FillFit, Padding: gogui.NoPadding, Spacing: gogui.SpacingSmall, Content: body})
		}, Foot: func(w *gogui.Window) gogui.View { return look.Toolkit.ActionBar(w, p.Actions, look.Palette()) }}
	})
}
