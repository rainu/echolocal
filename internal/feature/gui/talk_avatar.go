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
	"image"
	"strings"
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
	if c, ok := p.Renderer.(interface{ SetOverlay(func() image.Rectangle) }); ok {
		c.SetOverlay(talkVolumeBounds)
	}
	width, height := w.WindowSize()
	if width/2 >= 128 && height >= 128 {
		p.Start(avatar.Viewport{Width: width, Height: height})
	}
	if p.Fullscreen {
		return fullscreenComposition(width, height, p.SwitchMode, func() {
			if len(p.Actions) > 0 {
				p.Actions[len(p.Actions)-1].Do()
			}
		}, p.InputMuted != nil && p.InputMuted(), p.AgentActivity != nil && p.AgentActivity().Working)
	}
	s := p.Renderer.Snapshot()
	text := func(k string) string { return avatar.Text(say.Chosen(), k) }
	rows, _ := p.Build()
	title := p.Title
	if s.ModelName != "" {
		name := []rune(strings.Join(strings.Fields(s.ModelName), " "))
		if len(name) > 40 {
			name = append(name[:40], '…')
		}
		title += " · " + string(name)
	}
	header := []gogui.View{avatarText(title)}
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
	if s.ForegroundNotice != "" {
		captions = append(captions, talkAvatarHint(text(s.ForegroundNotice)))
	}
	if s.BackgroundNotice != "" {
		captions = append(captions, talkAvatarHint(text(s.BackgroundNotice)))
	}
	for i := 2; i < len(rows); i++ {
		r := rows[i]
		if r.Hint != "" && (r.Label == say.T("clawtalk.caption_user") || r.Label == say.T("clawtalk.caption_assistant")) {
			captions = append(captions, captionCard(fmt.Sprintf("talk-avatar-caption-%d", i-2), r.Hint, r.Label == say.T("clawtalk.caption_user")))
		} else {
			captions = append(captions, avatarText(r.Label))
		}
	}
	activity := []gogui.View{}
	if p.AgentActivity != nil && p.AgentActivity().Working {
		activity = append(activity, talkActivityBadge())
	}
	actions := []gogui.View{}
	for i, a := range p.Actions {
		id := "talk-avatar-history"
		if i == len(p.Actions)-1 {
			id = "talk-avatar-stop"
		}
		actions = append(actions, talkAvatarAction(id, a.Label, a.Do, i == len(p.Actions)-1))
	}
	actions = append(actions, talkModeButton("talk-avatar-fullscreen", gogui.IconExpand, text("switch_fullscreen"), false, p.SwitchMode))
	left := gogui.Column(gogui.ContainerCfg{
		ID: "talk-avatar-left", Width: float32(width / 2), Height: float32(height), Sizing: gogui.FixedFixed,
		Padding: gogui.PadAll(20), Spacing: gogui.SpacingSmall,
		Content: []gogui.View{
			gogui.Column(gogui.ContainerCfg{ID: "talk-avatar-status", Sizing: gogui.FillFit, Padding: gogui.NoPadding, Spacing: gogui.SpacingSmall, Content: header}),
			gogui.Column(gogui.ContainerCfg{ID: "talk-avatar-transcript", Sizing: gogui.FillFill, Padding: gogui.NoPadding, Spacing: gogui.SpacingSmall, Scrollable: true, DragScroll: true, ScrollMode: gogui.ScrollVerticalOnly, Content: captions}),
			gogui.Column(gogui.ContainerCfg{ID: "talk-avatar-activity-area", Sizing: gogui.FillFit, Padding: gogui.NoPadding, Content: activity}),
			gogui.Row(gogui.ContainerCfg{ID: "talk-avatar-actions", Sizing: gogui.FillFit, Padding: gogui.NoPadding, Spacing: gogui.SpacingSmall, Content: actions}),
		},
	})
	return avatarComposition(width, height, left)
}
func avatarComposition(width, height int, left gogui.View) gogui.View {
	right := gogui.Column(gogui.ContainerCfg{ID: "talk-avatar-canvas", SizeBorder: gogui.NoBorder, Radius: gogui.NoRadius, Spacing: gogui.NoSpacing, Width: float32(width - width/2), Height: float32(height), Sizing: gogui.FixedFixed, Padding: gogui.NoPadding})
	return gogui.Row(gogui.ContainerCfg{SizeBorder: gogui.NoBorder, Radius: gogui.NoRadius, Spacing: gogui.NoSpacing, Width: float32(width), Height: float32(height), Sizing: gogui.FixedFixed, Padding: gogui.NoPadding, Content: []gogui.View{left, right}})
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

// No title strip, footer reservation or square crop on the avatar half.
func talkAvatarViewport(width, height int) avatar.Viewport {
	return avatar.ConversationViewport(width, height)
}

// Must match native ControlCutouts: 72px circles, 24px gap, 20px bottom inset.
// Optional status flags are input-muted and verified agent-working, respectively.
func fullscreenComposition(width, height int, switchMode, end func(), status ...bool) gogui.View {
	text := func(k string) string { return avatar.Text(say.Chosen(), k) }
	leftContent := []gogui.View{}
	if len(status) > 1 && status[1] {
		leftContent = append(leftContent, talkActivityBadge(float32(min(244, width/2-116))))
	}
	controls := gogui.Row(gogui.ContainerCfg{SizeBorder: gogui.NoBorder, Radius: gogui.NoRadius, ID: "talk-fullscreen-controls", Width: 168, Height: 72, Sizing: gogui.FixedFixed, Padding: gogui.NoPadding, Spacing: gogui.SpacingPx(24), Content: []gogui.View{
		talkModeButton("talk-fullscreen-switch", gogui.IconColumns, text("switch_split"), false, switchMode, len(status) > 0 && status[0]), talkModeButton("talk-fullscreen-stop", gogui.IconPhone, text("end_talk"), true, end),
	}})
	return gogui.Column(gogui.ContainerCfg{SizeBorder: gogui.NoBorder, Radius: gogui.NoRadius, ID: "talk-fullscreen", Width: float32(width), Height: float32(height), Sizing: gogui.FixedFixed, Padding: gogui.PadAll(0), Spacing: gogui.NoSpacing, Content: []gogui.View{
		gogui.Column(gogui.ContainerCfg{SizeBorder: gogui.NoBorder, Radius: gogui.NoRadius, Height: float32(height - 92), Sizing: gogui.FillFixed, Padding: gogui.NoPadding}),
		gogui.Row(gogui.ContainerCfg{SizeBorder: gogui.NoBorder, Radius: gogui.NoRadius, Sizing: gogui.FillFit, Padding: gogui.NoPadding, Spacing: gogui.NoSpacing, Content: []gogui.View{
			gogui.Column(gogui.ContainerCfg{SizeBorder: gogui.NoBorder, Radius: gogui.NoRadius, Width: float32(width/2 - 84), Height: 72, Sizing: gogui.FixedFixed, Padding: gogui.NewPadding(16, 12, 16, 20), Content: leftContent}), controls,
		}}),
	}})
}

func talkModeButton(id, icon, label string, stop bool, do func(), muted ...bool) gogui.View {
	st := gogui.CurrentTheme().TextStyleIconLarge
	st.Size = 32
	st.Color = gogui.Hex(0xFFFFFF)
	color := gogui.Hex(0x23383F)
	if stop {
		color = gogui.Hex(0x74343D)
	}
	content := []gogui.View{gogui.Text(gogui.TextCfg{Text: icon, TextStyle: st})}
	if len(muted) > 0 && muted[0] {
		color = gogui.Hex(0x74343D)
		label = say.T("camera.muted") + " · " + label
		st.Size = 26
		content = []gogui.View{gogui.Row(gogui.ContainerCfg{Sizing: gogui.FitFit, Padding: gogui.NoPadding, Spacing: gogui.NoSpacing, Content: []gogui.View{
			gogui.Text(gogui.TextCfg{Text: icon, TextStyle: st}),
			gogui.DrawCanvas(gogui.DrawCanvasCfg{ID: "talk-muted-badge", Width: 22, Height: 28, OnDraw: func(dc *gogui.DrawContext) {
				dc.RoundedRect(8, 2, 7, 13, 3, gogui.White, 2)
				dc.Polyline([]float32{4, 10, 4, 14, 7, 19, 15, 19, 19, 14, 19, 10}, gogui.White, 2)
				dc.Line(11, 19, 11, 24, gogui.White, 2)
				dc.Line(6, 25, 16, 25, gogui.White, 2)
				dc.Line(2, 2, 21, 26, gogui.Hex(0x74343D), 5)
				dc.Line(2, 2, 21, 26, gogui.White, 2)
			}}),
		}})}
	}
	return gogui.Button(gogui.ButtonCfg{ID: id, A11YCfg: gogui.A11YCfg{A11YLabel: label}, Width: 72, Height: 72, MinWidth: 72, MinHeight: 72, Sizing: gogui.FixedFixed, Radius: gogui.RadiusPx(36), Color: color,
		Content: content, OnClick: func(e gogui.EventCtx) {
			if do != nil {
				do()
			}
			e.Window.InvalidateLayout()
		}})
}

// Static for every user, including reduced-motion users: no pulses, timers,
// animation, speech, input handler or color-only meaning.
func talkActivityBadge(width ...float32) gogui.View {
	badgeWidth := float32(244)
	if len(width) > 0 {
		badgeWidth = width[0]
	}
	label := avatar.Text(say.Chosen(), "agent_working")
	st := gogui.CurrentTheme().TextStyleDef
	st.Size = 16
	st.Color = gogui.Hex(0xEEF3F4)
	return gogui.Row(gogui.ContainerCfg{
		ID: "talk-agent-activity", A11YCfg: gogui.A11YCfg{A11YLabel: label},
		Width: badgeWidth, Height: 40, Sizing: gogui.FixedFixed,
		Padding: gogui.NewPadding(8, 12, 8, 12), Radius: gogui.RadiusPx(12),
		Color:   gogui.Hex(0x23383F),
		Content: []gogui.View{gogui.Text(gogui.TextCfg{Text: label, TextStyle: st})},
	})
}
