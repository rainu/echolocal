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
	p.SetActivityRepaint(w.InvalidateRender)
	if c, ok := p.Renderer.(interface{ SetOverlay(func() image.Rectangle) }); ok {
		c.SetOverlay(talkVolumeBounds)
	}
	width, height := w.WindowSize()
	if width/2 >= 128 && height >= 128 {
		p.Start(avatar.Viewport{Width: width, Height: height})
	}
	if p.Fullscreen {
		return fullscreenActivityComposition(p, width, height, p.SwitchMode, func() {
			if len(p.Actions) > 0 {
				p.Actions[len(p.Actions)-1].Do()
			}
		}, p.InputMuted != nil && p.InputMuted(), p.AgentActivity != nil && p.AgentActivity().Working)
	}
	s := p.Renderer.Snapshot()
	text := func(k string) string { return avatar.Text(say.Chosen(), k) }
	rows, _ := p.Build()

	title := ""
	if p.SessionTitle != nil {
		title = p.SessionTitle()
	}
	if title == "" {
		title = text("session_unknown")
	}
	titleStyle := gogui.CurrentTheme().TextStyleDef
	titleStyle.Size = 24
	displayTitle := fitTalkTitle(w, title, titleStyle, float32(width/2-40))
	header := []gogui.View{gogui.Text(gogui.TextCfg{ID: "talk-session-title", A11YCfg: gogui.A11YCfg{A11YLabel: title}, Text: displayTitle, TextStyle: titleStyle, Mode: gogui.TextModeSingleLine})}
	status := ""
	if len(rows) > 0 {
		status = rows[0].Label
	}
	mic := "mic_off"
	if p.MicrophoneStatus != nil {
		mic = p.MicrophoneStatus()
	}
	privacyStyle := gogui.CurrentTheme().TextStyleDef
	privacyStyle.Size = 18
	header = append(header, gogui.Text(gogui.TextCfg{ID: "talk-microphone-status", Text: status + " · " + text(mic), TextStyle: privacyStyle, Mode: gogui.TextModeWrap}))
	// Healthy avatar/audio operation needs no explanatory paragraph. Keep only
	// actionable failures, so transcript space does not depend on static prose.
	if s.Notice != "" && s.Notice != "starting" && s.Notice != "ready" && s.Notice != "running" {
		header = append(header, talkAvatarHint(text(s.Notice)+" "+text("talk_fallback")))
	}
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
		activity = append(activity, talkActivityBadge(p))
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
		ID: "talk-avatar-left", SizeBorder: gogui.NoBorder, Radius: gogui.NoRadius, Width: float32(width / 2), Height: float32(height), Sizing: gogui.FixedFixed,
		Padding: gogui.PadAll(20), Spacing: gogui.SpacingSmall,
		Content: []gogui.View{
			gogui.Column(gogui.ContainerCfg{ID: "talk-avatar-status", SizeBorder: gogui.NoBorder, Radius: gogui.NoRadius, Sizing: gogui.FillFit, Padding: gogui.NoPadding, Spacing: gogui.SpacingSmall, Content: header}),
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
	return fullscreenActivityComposition(nil, width, height, switchMode, end, status...)
}
func fullscreenActivityComposition(p *clawtalk.AvatarPage, width, height int, switchMode, end func(), status ...bool) gogui.View {
	text := func(k string) string { return avatar.Text(say.Chosen(), k) }
	badgeWidth := float32(talkActivityWidth)
	rightContent := []gogui.View{}
	if len(status) > 1 && status[1] {
		rightContent = append(rightContent, talkActivityBadge(p))
	}
	switchButton := talkModeButton("talk-fullscreen-switch", gogui.IconColumns, text("switch_split"), false, switchMode, len(status) > 0 && status[0])
	if p != nil && p.TurnIndicator != nil {
		switchButton = talkTurnButton(p.TurnIndicator(), switchMode)
	}
	controls := gogui.Row(gogui.ContainerCfg{SizeBorder: gogui.NoBorder, Radius: gogui.NoRadius, ID: "talk-fullscreen-controls", Width: 168, Height: 72, Sizing: gogui.FixedFixed, Padding: gogui.NoPadding, Spacing: gogui.SpacingPx(24), Content: []gogui.View{
		switchButton, talkModeButton("talk-fullscreen-stop", gogui.IconPhone, text("end_talk"), true, end),
	}})
	return gogui.Column(gogui.ContainerCfg{SizeBorder: gogui.NoBorder, Radius: gogui.NoRadius, ID: "talk-fullscreen", Width: float32(width), Height: float32(height), Sizing: gogui.FixedFixed, Padding: gogui.PadAll(0), Spacing: gogui.NoSpacing, Content: []gogui.View{
		gogui.Column(gogui.ContainerCfg{SizeBorder: gogui.NoBorder, Radius: gogui.NoRadius, Height: float32(height - 92), Sizing: gogui.FillFixed, Padding: gogui.NoPadding}),
		gogui.Row(gogui.ContainerCfg{SizeBorder: gogui.NoBorder, Radius: gogui.NoRadius, Sizing: gogui.FillFit, Padding: gogui.NoPadding, Spacing: gogui.NoSpacing, Content: []gogui.View{
			gogui.Column(gogui.ContainerCfg{SizeBorder: gogui.NoBorder, Radius: gogui.NoRadius, Width: float32(width/2 - 84), Height: 72, Sizing: gogui.FixedFixed, Padding: gogui.NoPadding}), controls,
			gogui.Column(gogui.ContainerCfg{SizeBorder: gogui.NoBorder, Radius: gogui.NoRadius, Width: float32(width/2 - 84), Height: 72, Sizing: gogui.FixedFixed, Padding: gogui.NewPadding(14, 20, 14, float32(width/2-104)-badgeWidth), Content: rightContent}),
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

const talkActivityWidth = 64
const talkActivityHeight = 44

// Only the dots repaint on the existing Talk tick; geometry and accessible name
// stay fixed. Tapping freezes three dots without canceling the underlying task.
func talkActivityBadge(p *clawtalk.AvatarPage) gogui.View {
	dots := func() int {
		if p == nil {
			return 3
		}
		return p.ActivityDots()
	}
	return gogui.Button(gogui.ButtonCfg{
		ID: "talk-agent-activity", A11YCfg: talkActivityAccessibility(p),
		Width: talkActivityWidth, Height: talkActivityHeight, MinWidth: talkActivityWidth, MinHeight: talkActivityHeight,
		Sizing: gogui.FixedFixed, Padding: gogui.NoPadding, Radius: gogui.RadiusPx(22),
		Color: gogui.Hex(0x23383F), HAlign: gogui.Some(gogui.HAlignCenter), VAlign: gogui.Some(gogui.VAlignMiddle),
		Content: []gogui.View{gogui.DrawCanvas(gogui.DrawCanvasCfg{
			ID: "talk-agent-dots", Width: 40, Height: 16,
			VersionFn: func() uint64 { return uint64(dots()) },
			OnDraw: func(dc *gogui.DrawContext) {
				for i := 0; i < dots(); i++ {
					dc.FilledCircle(8+float32(i)*12, 8, 3, gogui.Hex(0xEEF3F4))
				}
			},
		})},
		OnClick: func(e gogui.EventCtx) {
			if p != nil {
				p.ToggleActivityMotion()
				e.Window.InvalidateLayout()
			}
		},
	})
}

func talkActivityAccessibility(p *clawtalk.AvatarPage) gogui.A11YCfg {
	label := avatar.Text(say.Chosen(), "agent_working")
	action := avatar.Text(say.Chosen(), "activity_pause")
	if p != nil && p.ActivityPaused() {
		action = avatar.Text(say.Chosen(), "activity_resume")
	}
	return gogui.A11YCfg{A11YLabel: label, A11YDescription: action}
}

// Measure the actual font rather than guessing a character budget: wide glyphs
// and every supported language stay inside the split pane on small displays.
func fitTalkTitle(w *gogui.Window, title string, style gogui.TextStyle, width float32) string {
	if w.TextWidth(title, style) <= width {
		return title
	}
	r := []rune(title)
	lo, hi := 0, len(r)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if w.TextWidth(string(r[:mid])+"…", style) <= width {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return string(r[:lo]) + "…"
}
