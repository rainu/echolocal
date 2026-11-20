//go:build !board_biscuit

package gui

import (
	"fmt"
	"strings"
	"time"

	gogui "github.com/go-gui-org/go-gui/gui"
	"github.com/ygelfand/echolocal/internal/feature/apps"
	"github.com/ygelfand/echolocal/internal/feature/clawtalk"
	"github.com/ygelfand/echolocal/internal/feature/drawer"
	claw "github.com/ygelfand/echolocal/internal/feature/openclaw"
	featureshell "github.com/ygelfand/echolocal/internal/feature/shell"
	"github.com/ygelfand/echolocal/internal/feature/voice"
	"github.com/ygelfand/echolocal/internal/openclaw/chatapp"
	sharedapps "github.com/ygelfand/libcountertop/pkg/display/apps"
	shareddrawer "github.com/ygelfand/libcountertop/pkg/display/drawer"
	"github.com/ygelfand/libcountertop/pkg/display/screens"
	sharedshell "github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
	"github.com/ygelfand/libcountertop/pkg/say"
)

type clawAppView struct {
	app               *chatapp.App
	look              *widgets.UI
	folded            map[string]bool
	sidebar           bool
	models            bool
	historyGeneration uint64
	historyCount      int
	historyHeight     float32
	followTail        bool
}

func (*clawAppView) Covers() bool           { return true }
func (*clawAppView) Timeout() time.Duration { return 30 * time.Minute }
func (v *clawAppView) Close()               { v.app.Close() }

func extendClawApp(look *widgets.UI, o *screens.Options, _ func() *screens.App) {
	v := &clawAppView{look: look, folded: map[string]bool{}, sidebar: true}
	m := clawtalk.Get()
	v.app = chatapp.New(func() (chatapp.RPC, error) { return claw.Get().BindApp() }, featureshell.Get().Redraw,
		func(s chatapp.Session) (uint64, error) {
			return m.StartSession(s.Key, voice.Get().Turn().Busy() || voice.Get().Turn().Pending())
		}, m.StopOwned, m.Busy)
	apps.Get().Add(sharedapps.App{ID: "openclaw", Name: func() string { return "OpenClaw" }, Icon: "mdi:chat", Open: func() { v.app.Open(); featureshell.Get().Push(v) }})
	drawer.Get().Add(shareddrawer.Entry{Order: 32, Icon: "mdi:chat", Name: func() string { return "OpenClaw" }, Open: func() { v.app.Open(); featureshell.Get().Push(v) }})
	o.Views = append(o.Views, func(view sharedshell.View) *screens.Screen {
		if view != v {
			return nil
		}
		return &screens.Screen{View: v, Fixed: true, Build: v.build}
	})
}

func appText(key string) string {
	words := map[string][2]string{
		"activity": {"Assistant is working…", "Assistent arbeitet…"}, "sessions": {"Sessions", "Sitzungen"}, "home": {"Home", "Startseite"}, "send": {"Send", "Senden"}, "talk": {"Talk", "Sprechen"}, "stop": {"Stop Talk", "Talk beenden"}, "stopping": {"Stopping…", "Wird beendet…"}, "model": {"Model", "Modell"}, "refresh": {"Refresh", "Aktualisieren"}, "older": {"Older messages", "Ältere Nachrichten"}, "tail": {"Latest messages", "Neueste Nachrichten"}, "empty": {"No sessions", "Keine Sitzungen"}, "choose": {"Choose a session on the left", "Links eine Sitzung auswählen"}, "compose": {"Message to selected session", "Nachricht an ausgewählte Sitzung"}, "ungrouped": {"Ungrouped", "Ohne Gruppe"}, "loading": {"Loading…", "Wird geladen…"}, "disconnected": {"Disconnected. Check the existing connection settings.", "Getrennt. Bestehende Verbindungseinstellungen prüfen."}, "unavailable": {"Unavailable or missing operator permissions. Refresh to retry.", "Nicht verfügbar oder Operator-Berechtigung fehlt. Zum Wiederholen aktualisieren."}, "session_changed": {"Session changed or removed. Select it again before continuing.", "Sitzung geändert oder entfernt. Vor dem Fortsetzen erneut auswählen."}, "stop_first": {"Finish sending / stop Talk before switching sessions.", "Vor dem Wechsel Senden abschließen bzw. Talk beenden."}, "history_failed": {"History unavailable. Refresh to retry.", "Verlauf nicht verfügbar. Zum Wiederholen aktualisieren."}, "action_failed": {"Action unavailable or rejected; no automatic retry.", "Aktion nicht verfügbar oder abgelehnt; keine automatische Wiederholung."}, "sending": {"Sending…", "Wird gesendet…"}, "sent": {"Message accepted", "Nachricht angenommen"}, "delivery_uncertain": {"Delivery not confirmed. Check history. Send again retries the same message ID.", "Zustellung unbestätigt. Verlauf prüfen. Erneutes Senden verwendet dieselbe Nachrichten-ID."}, "model_changed": {"Model changed", "Modell geändert"}, "talk_ended": {"Talk ended. Check Talk settings/diagnostics if unexpected.", "Talk beendet. Bei unerwartetem Ende Talk-Einstellungen/Diagnose prüfen."}, "talk_active": {"Talk in the selected session — stop before switching.", "Talk in der gewählten Sitzung — vor dem Wechsel beenden."}, "talk_failed": {"Talk unavailable. Check Talk permissions/settings.", "Talk nicht verfügbar. Talk-Berechtigung/Einstellungen prüfen."}, "default": {"Agent default", "Agent-Standard"}, "user": {"You", "Du"}, "assistant": {"Assistant", "Assistent"},
	}
	if w, ok := words[key]; ok {
		if say.Chosen() == "de" {
			return w[1]
		}
		return w[0]
	}
	return key
}
func (v *clawAppView) button(id, label string, disabled bool, do func()) gogui.View {
	// Muted action-specific accents keep the toolbar readable without bright fills.
	color := gogui.Hex(0x30343A)
	switch id {
	case "claw-sessions":
		color = gogui.Hex(0x343E50)
	case "claw-refresh":
		color = gogui.Hex(0x2D4546)
	case "claw-talk":
		color = gogui.Hex(0x354A3D)
	case "claw-model":
		color = gogui.Hex(0x443C50)
	case "claw-send":
		color = gogui.Hex(0x354655)
	}
	style := appSmallStyle(v)
	style.Color = gogui.Hex(0xE0E3E6)
	return gogui.Button(gogui.ButtonCfg{ID: id, Variant: gogui.ButtonGhost, Color: color, Content: []gogui.View{gogui.Text(gogui.TextCfg{Text: label, TextStyle: style})}, MinHeight: 44, Height: 44, Padding: gogui.PaddingSmall, Disabled: disabled, OnClick: func(e gogui.EventCtx) { do(); e.Window.InvalidateLayout() }})
}
func (v *clawAppView) text(text string) gogui.View {
	return gogui.Text(gogui.TextCfg{Text: text, Mode: gogui.TextModeWrap})
}
func (v *clawAppView) build(w *gogui.Window) gogui.View {
	s := v.app.Snapshot()
	_, vh := w.WindowSize()
	height := float32(vh)
	if v.look.Editor.Typing(w) && v.look.Editor.Top > 0 {
		height = v.look.Editor.Top
	}
	resized := v.historyHeight != height
	v.historyHeight = height
	locked := s.Busy || s.Talk
	controls := []gogui.View{
		v.button("claw-sessions", appText("sessions"), false, func() { v.sidebar = !v.sidebar }),
		v.button("claw-home", appText("home"), false, func() { v.app.Close(); featureshell.Get().Close() }),
		v.button("claw-refresh", appText("refresh"), s.Busy, v.app.Refresh),
	}
	talkLabel := appText("talk")
	if s.Talk {
		talkLabel = appText("stop")
	}
	if s.Stopping {
		talkLabel = appText("stopping")
	}
	controls = append(controls, v.button("claw-talk", talkLabel, s.Stopping || s.Busy || !s.Connected || s.Selected.Key == "", func() { v.app.Talk(s.Generation) }))
	controls = append(controls, v.button("claw-model", appText("model"), locked || !s.Connected || s.Selected.Key == "", func() {
		v.models = !v.models
		if v.models {
			v.app.LoadModels()
		}
	}))
	title := s.Selected.Title()
	if title == "" {
		title = appText("choose")
	}
	if s.Selected.Model != "" {
		title += " · " + s.Selected.Model
	}
	main := []gogui.View{gogui.Text(gogui.TextCfg{ID: "claw-title", Sizing: gogui.FillFit, Clip: true, Text: title, TextStyle: appSmallStyle(v), Mode: gogui.TextModeSingleLine})}

	if s.Talk {
		main = append(main, v.text(say.T("clawtalk.state."+string(clawtalk.Get().Status().State))))
	}
	if s.Active {
		main = append(main, v.text(appText("activity")))
	}
	if s.Loading {
		main = append(main, v.text(appText("loading")))
	}
	if s.Notice != "" {
		main = append(main, v.text(appText(s.Notice)))
	}
	if v.models {
		modelViews := []gogui.View{v.button("claw-model-default", appText("default"), locked, func() { v.app.SetModel("", s.Generation); v.models = false })}
		if !s.ModelsReady {
			modelViews = append(modelViews, v.text(appText("loading")))
		}
		for i, m := range s.Models {
			key := m.Key()
			modelViews = append(modelViews, v.button(fmt.Sprintf("claw-model-%d", i), key, locked, func() { v.app.SetModel(key, s.Generation); v.models = false }))
		}
		main = append(main, gogui.Column(gogui.ContainerCfg{ID: "claw-model-list", Sizing: gogui.FillFill, Scrollable: true, Content: modelViews}))
	} else {
		messages := []gogui.View{}

		if s.Talk {
			for _, c := range clawtalk.Get().CaptionsFor(s.TalkGeneration) {
				messages = append(messages, v.message(w, fmt.Sprintf("claw-caption-%d", len(messages)), c.Role, c.Text))
			}
		}
		for i, m := range s.Messages {
			if m.Role != "user" && m.Role != "assistant" {
				continue
			}
			text := m.Text()
			if text == "" {
				continue
			}
			messages = append(messages, v.message(w, fmt.Sprintf("claw-message-%d", i), m.Role, text))
		}
		historyID := fmt.Sprintf("claw-history-%d", s.Generation)
		changed := v.historyGeneration != s.Generation
		tail := changed || (v.followTail && (resized || v.historyCount != len(s.Messages)))
		v.historyGeneration = s.Generation
		v.historyCount = len(s.Messages)
		main = append(main, gogui.Column(gogui.ContainerCfg{ID: historyID, Sizing: gogui.FillFill, Padding: gogui.PaddingSmall, Spacing: gogui.SpacingSmall, Scrollable: true, DragScroll: true, ScrollMode: gogui.ScrollVerticalOnly, Content: messages,
			AmendLayout: func(e gogui.EventCtx) {
				if tail {
					e.Window.QueueCommand(func(w *gogui.Window) {
						for _, id := range w.ResolveID(historyID) {
							w.ScrollVerticalTo(id, -1e9)
						}
						v.followTail = true
					})
				}
			},
			OnScroll: func(e gogui.EventCtx) {
				ids := e.Window.ResolveID(historyID)
				if len(ids) != 1 {
					return
				}
				id := ids[0]
				offset := e.Window.ScrollVerticalOffset(id)
				v.followTail = e.Window.ScrollVerticalPct(id) >= 0.98
				if offset >= 0 && s.Older != "" && !locked && !s.Loading {
					v.app.Older()
				}
			},
		}))
	}
	input := v.look.Input(gogui.InputCfg{ID: fmt.Sprintf("claw-compose-%d", s.Generation), Text: s.Draft, Placeholder: appText("compose"), Sizing: gogui.FillFit, Height: 44, MinHeight: 44, MaxHeight: 44, Padding: gogui.PaddingSmall, Disabled: locked || !s.Connected || s.Selected.Key == "", OnTextChanged: func(text string, e gogui.EventCtx) { v.app.Draft(text, s.Generation); e.Window.InvalidateLayout() }}, func() {})
	main = append(main, gogui.Row(gogui.ContainerCfg{ID: "claw-composer", Sizing: gogui.FillFit, VAlign: gogui.VAlignMiddle, Padding: gogui.NoPadding, Spacing: gogui.SpacingSmall, Content: []gogui.View{input, v.button("claw-send", appText("send"), locked || !s.Connected || s.Selected.Key == "" || s.Draft == "", func() { v.app.Send(s.Generation) })}}))
	body := []gogui.View{}
	if v.sidebar {
		groups := append([]string(nil), s.Groups...)
		groups = append(groups, "")
		known := map[string]bool{}
		for _, g := range groups {
			known[g] = true
		}
		for _, session := range s.Sessions {
			if !known[session.Category] {
				groups = append(groups, session.Category)
				known[session.Category] = true
			}
		}
		side := []gogui.View{}
		if len(s.Sessions) == 0 {
			side = append(side, v.text(appText("empty")))
		}
		for gi, group := range groups {
			groupRows := []chatapp.Session{}
			for _, session := range s.Sessions {
				if session.Category == group {
					groupRows = append(groupRows, session)
				}
			}
			if len(groupRows) == 0 {
				continue
			}
			name := group
			if name == "" {
				name = appText("ungrouped")
			}
			arrow := "▾ "
			if v.folded[group] {
				arrow = "▸ "
			}
			side = append(side, v.listButton(fmt.Sprintf("claw-group-%d", gi), arrow+name, false, func() { v.folded[group] = !v.folded[group] }))
			if !v.folded[group] {
				for si, session := range groupRows {
					key := session.Key
					label := session.Title()
					if s.Selected.Key == key {
						label = "● " + label
					}
					side = append(side, v.listButton(fmt.Sprintf("claw-session-%d-%d", gi, si), label, locked, func() { v.app.Select(key); v.models = false }))
				}
			}
		}
		width := float32(220)
		vw, _ := w.WindowSize()
		if vw < 900 {
			width = 180
		}
		body = append(body, gogui.Column(gogui.ContainerCfg{ID: "claw-sidebar", Sizing: gogui.FixedFill, Width: width, Scrollable: true, DragScroll: true, ScrollMode: gogui.ScrollVerticalOnly, Padding: gogui.NoPadding, Spacing: gogui.NoSpacing, Content: side}))
	}
	body = append(body, gogui.Column(gogui.ContainerCfg{Sizing: gogui.FillFill, Padding: gogui.NoPadding, Spacing: gogui.SpacingSmall, Content: main}))
	return gogui.Column(gogui.ContainerCfg{ID: "claw-app", Height: height, AmendLayout: func(e gogui.EventCtx) {
		if v.look.Editor.Typing(e.Window) && v.look.Editor.Top > 0 && height > v.look.Editor.Top {
			e.Window.QueueCommand(func(w *gogui.Window) { w.InvalidateLayout() })
		}
	}, Padding: gogui.PaddingSmall, Sizing: gogui.FillFixed, Spacing: gogui.SpacingSmall, Content: []gogui.View{gogui.Row(gogui.ContainerCfg{Sizing: gogui.FillFit, Padding: gogui.NoPadding, Spacing: gogui.SpacingSmall, Content: controls}), gogui.Row(gogui.ContainerCfg{Sizing: gogui.FillFill, Padding: gogui.NoPadding, Spacing: gogui.SpacingSmall, Content: body})}})
}

// The embedded device theme predates go-gui's document-role ladder. Do not
// inherit zero-sized role fonts from it: materialize the complete public grid.
func appMarkdownStyle(ink ...gogui.Color) gogui.MarkdownStyle {
	style := gogui.DefaultMarkdownStyle()
	base := gogui.CurrentTheme().Cfg.TextStyleDef
	base.Color = gogui.White
	if len(ink) > 0 {
		base.Color = ink[0]
	}
	if base.Size < 18 {
		base.Size = 18
	}
	style.Text = base
	style.Bold = base.Bold()
	style.Italic = base.Italic()
	style.BoldItalic = base.Bold().Italic()
	style.H1 = base.Bold()
	style.H1.Size = base.Size * 1.5
	style.H2 = base.Bold()
	style.H2.Size = base.Size * 1.3
	style.H3 = base.Bold()
	style.H3.Size = base.Size * 1.15
	style.H4 = base.Bold()
	style.H5 = base.Bold()
	style.H6 = base.Bold()
	style.Code = base
	return style
}

// The native renderer supplies LayoutText, but not LayoutRichText. Lower
// Markdown blocks to supported Text views instead of invisible RTF layouts.
func (v *clawAppView) markdown(w *gogui.Window, id, text string) gogui.View {
	style := appMarkdownStyle(gogui.White)
	// Use the cached Markdown parser, but return only our native block views.
	// The stock document widget always adds a clipboard toolbar and reserves
	// room for it; neither belongs on this touch-only chat surface.
	return gogui.ViewFunc(func(w *gogui.Window) gogui.View {
		blocks := []gogui.View{}
		w.Markdown(gogui.MarkdownCfg{ID: id, Source: text, Style: style, DisableExternalAPIs: true,
			RenderBlock: func(_ *gogui.Window, el gogui.MarkdownElement) (gogui.View, bool) {
				blocks = append(blocks, v.markdownBlock(el, style))
				return nil, true
			},
		}).GenerateLayout(w)
		return gogui.Column(gogui.ContainerCfg{ID: id + "-native", Sizing: gogui.FillFit, Padding: gogui.NoPadding, Spacing: gogui.SpacingSmall, Content: blocks})
	})
}
func (v *clawAppView) markdownBlock(el gogui.MarkdownElement, style gogui.MarkdownStyle) gogui.View {
	if el.Kind == gogui.MarkdownKindHeading {
		st := style.H4
		switch el.HeaderLevel {
		case 1:
			st = style.H1
		case 2:
			st = style.H2
		case 3:
			st = style.H3
		}
		return gogui.Text(gogui.TextCfg{Text: el.PlainText, TextStyle: st, Mode: gogui.TextModeWrap})
	}
	if el.Kind == gogui.MarkdownKindImage {
		return v.text(el.ImageAlt)
	}
	if el.Kind == gogui.MarkdownKindMath {
		return v.text(el.MathLatex)
	}
	if el.Kind == gogui.MarkdownKindHR {
		return v.text("────────────────")
	}
	if el.Kind == gogui.MarkdownKindCode {
		st := style.Code
		st.Color = gogui.White
		return gogui.Column(gogui.ContainerCfg{Sizing: gogui.FillFit, Padding: gogui.PaddingMedium, Color: gogui.Hex(0x30343A), Content: []gogui.View{gogui.Text(gogui.TextCfg{Text: el.PlainText, TextStyle: st, Mode: gogui.TextModeWrap})}})
	}
	if el.Kind == gogui.MarkdownKindTable && el.TableData != nil {
		rows := [][]gogui.RichText{el.TableData.Headers}
		rows = append(rows, el.TableData.Rows...)
		body := []gogui.View{}
		for ri, row := range rows {
			cells := []gogui.View{}
			for _, cell := range row {
				text := ""
				for _, run := range cell.Runs {
					text += run.Text
				}
				st := style.Text
				if ri == 0 {
					st = st.Bold()
				}
				cells = append(cells, gogui.Column(gogui.ContainerCfg{Sizing: gogui.FillFit, Padding: gogui.PaddingSmall, Content: []gogui.View{gogui.Text(gogui.TextCfg{Text: text, TextStyle: st, Mode: gogui.TextModeWrap})}}))
			}
			body = append(body, gogui.Row(gogui.ContainerCfg{Sizing: gogui.FillFit, Content: cells}))
		}
		return gogui.Column(gogui.ContainerCfg{Sizing: gogui.FillFit, Content: body})
	}
	spans := []gogui.View{}
	if el.ListPrefix != "" {
		spans = append(spans, gogui.Text(gogui.TextCfg{Text: el.ListPrefix + " ", TextStyle: style.Text, Sizing: gogui.FitFit}))
	}
	if el.IsTaskItem {
		mark := "☐ "
		if el.TaskChecked {
			mark = "☑ "
		}
		spans = append(spans, gogui.Text(gogui.TextCfg{Text: mark, TextStyle: style.Text, Sizing: gogui.FitFit}))
	}
	if el.Kind == gogui.MarkdownKindBlockquote {
		spans = append(spans, gogui.Text(gogui.TextCfg{Text: "› ", TextStyle: style.Text, Sizing: gogui.FitFit}))
	}
	words := 0
	for _, run := range el.Content.Runs {
		st := run.Style
		st.Color = style.Text.Color
		if st.Size < 18 {
			st.Size = 18
		}
		for _, word := range strings.Fields(run.Text) {
			words++
			if words > 300 {
				return gogui.Text(gogui.TextCfg{Text: el.PlainText, TextStyle: style.Text, Mode: gogui.TextModeWrap})
			}
			spans = append(spans, gogui.Text(gogui.TextCfg{Text: word + " ", TextStyle: st, Sizing: gogui.FitFit}))
		}
	}
	if len(spans) == 0 {
		return v.text(el.PlainText)
	}
	return gogui.Wrap(gogui.ContainerCfg{Sizing: gogui.FillFit, Padding: gogui.NoPadding, Spacing: gogui.NoSpacing, Content: spans})
}
func appSmallStyle(v *clawAppView) gogui.TextStyle {
	st := appMarkdownStyle(widgets.Color(v.look.Palette().Text)).Text
	st.Size = 18
	return st
}
func (v *clawAppView) listButton(id, label string, disabled bool, do func()) gogui.View {
	style := appSmallStyle(v)
	style.Align = gogui.TextAlignRight
	align := gogui.HAlignEnd
	if strings.HasPrefix(id, "claw-group-") {
		style = style.Bold()
		style.Align = gogui.TextAlignLeft
		align = gogui.HAlignStart
	}
	return gogui.Button(gogui.ButtonCfg{ID: id, Variant: gogui.ButtonGhost, HAlign: gogui.Some(align), Sizing: gogui.FillFit, Content: []gogui.View{gogui.Text(gogui.TextCfg{Sizing: gogui.FillFit, Text: label, TextStyle: style, Mode: gogui.TextModeSingleLine, Clip: true})}, MinHeight: 40, Height: 40, Padding: gogui.PaddingSmall, Disabled: disabled, OnClick: func(e gogui.EventCtx) { do(); e.Window.InvalidateLayout() }})
}
func (v *clawAppView) message(w *gogui.Window, id, role, text string) gogui.View {
	user := role == "user"
	color := gogui.Hex(0x232526)
	align := gogui.HAlignStart
	if user {
		color = gogui.Hex(0x2B3033)
		align = gogui.HAlignEnd
	}
	label := appSmallStyle(v)
	label.Color = gogui.Hex(0xCBD5E1)
	var content gogui.View = v.markdown(w, id+"-md", text)
	if user {
		st := appMarkdownStyle(gogui.White).Text
		st.Align = gogui.TextAlignRight
		content = gogui.Text(gogui.TextCfg{Text: text, TextStyle: st, Mode: gogui.TextModeWrap})
		label.Align = gogui.TextAlignRight
	}
	bubble := gogui.Column(gogui.ContainerCfg{ID: id, Sizing: gogui.FillFit, Padding: gogui.PaddingSmall, Color: color, Content: []gogui.View{gogui.Text(gogui.TextCfg{Sizing: gogui.FillFit, Text: appText(role), TextStyle: label}), content}})
	spacer := gogui.Column(gogui.ContainerCfg{Sizing: gogui.FixedFit, Width: 40, Padding: gogui.NoPadding})
	children := []gogui.View{bubble, spacer}
	if user {
		children = []gogui.View{spacer, bubble}
	}
	return gogui.Row(gogui.ContainerCfg{ID: id + "-row", Sizing: gogui.FillFit, HAlign: align, Padding: gogui.NoPadding, Content: children})
}
