//go:build !board_biscuit

package gui

import (
	"fmt"
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
	app     *chatapp.App
	look    *widgets.UI
	folded  map[string]bool
	sidebar bool
	models  bool
}

func (*clawAppView) Covers() bool           { return true }
func (*clawAppView) Timeout() time.Duration { return 0 }
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
		return &screens.Screen{Title: "OpenClaw", View: v, Fixed: true, Build: v.build}
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
	return gogui.Button(gogui.ButtonCfg{ID: id, Label: label, MinHeight: 60, Padding: gogui.PaddingMedium, Disabled: disabled, OnClick: func(e gogui.EventCtx) { do(); e.Window.InvalidateLayout() }})
}
func (v *clawAppView) text(text string) gogui.View {
	return gogui.Text(gogui.TextCfg{Text: text, Mode: gogui.TextModeWrap})
}
func (v *clawAppView) build(w *gogui.Window) gogui.View {
	s := v.app.Snapshot()
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
	main := []gogui.View{gogui.Row(gogui.ContainerCfg{Sizing: gogui.FillFit, Content: []gogui.View{gogui.Column(gogui.ContainerCfg{Sizing: gogui.FillFit, Content: []gogui.View{v.text(title)}}), v.button("claw-latest", appText("tail"), false, func() { w.ScrollVerticalTo(fmt.Sprintf("claw-history-%d", s.Generation), -1e9) })}})}
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
		if s.Older != "" {
			messages = append(messages, v.button("claw-older", appText("older"), locked, v.app.Older))
		}
		if s.Talk {
			for _, c := range clawtalk.Get().CaptionsFor(s.TalkGeneration) {
				messages = append(messages, gogui.Column(gogui.ContainerCfg{Sizing: gogui.FillFit, Padding: gogui.PaddingMedium, Content: []gogui.View{v.text(appText(c.Role)), v.text(c.Text)}}))
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
			messages = append(messages, gogui.Column(gogui.ContainerCfg{ID: fmt.Sprintf("claw-message-%d", i), Sizing: gogui.FillFit, Padding: gogui.PaddingMedium, Content: []gogui.View{
				v.text(appText(m.Role)), w.Markdown(gogui.MarkdownCfg{ID: fmt.Sprintf("claw-md-%d", i), Source: text, DisableExternalAPIs: true}),
			}}))
		}
		main = append(main, gogui.Column(gogui.ContainerCfg{ID: fmt.Sprintf("claw-history-%d", s.Generation), Sizing: gogui.FillFill, Scrollable: true, ScrollMode: gogui.ScrollVerticalOnly, Content: messages}))
	}
	input := v.look.Input(gogui.InputCfg{ID: fmt.Sprintf("claw-compose-%d", s.Generation), Text: s.Draft, Placeholder: appText("compose"), Sizing: gogui.FillFit, Disabled: locked || !s.Connected || s.Selected.Key == "", OnTextChanged: func(text string, e gogui.EventCtx) { v.app.Draft(text, s.Generation); e.Window.InvalidateLayout() }}, func() {})
	main = append(main, gogui.Row(gogui.ContainerCfg{Sizing: gogui.FillFit, Spacing: gogui.SpacingSmall, Content: []gogui.View{input, v.button("claw-send", appText("send"), locked || !s.Connected || s.Selected.Key == "" || s.Draft == "", func() { v.app.Send(s.Generation) })}}))
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
			side = append(side, v.button(fmt.Sprintf("claw-group-%d", gi), arrow+name, false, func() { v.folded[group] = !v.folded[group] }))
			if !v.folded[group] {
				for si, session := range groupRows {
					key := session.Key
					label := session.Title()
					if s.Selected.Key == key {
						label = "● " + label
					}
					side = append(side, v.button(fmt.Sprintf("claw-session-%d-%d", gi, si), label, locked, func() { v.app.Select(key); v.models = false }))
				}
			}
		}
		width := float32(280)
		vw, _ := w.WindowSize()
		if vw < 900 {
			width = 220
		}
		body = append(body, gogui.Column(gogui.ContainerCfg{ID: "claw-sidebar", Sizing: gogui.FixedFill, Width: width, Scrollable: true, ScrollMode: gogui.ScrollVerticalOnly, Spacing: gogui.SpacingSmall, Content: side}))
	}
	body = append(body, gogui.Column(gogui.ContainerCfg{Sizing: gogui.FillFill, Spacing: gogui.SpacingSmall, Content: main}))
	return gogui.Column(gogui.ContainerCfg{Sizing: gogui.FillFill, Spacing: gogui.SpacingSmall, Content: []gogui.View{gogui.Row(gogui.ContainerCfg{Sizing: gogui.FillFit, Spacing: gogui.SpacingSmall, Content: controls}), gogui.Row(gogui.ContainerCfg{Sizing: gogui.FillFill, Spacing: gogui.SpacingMedium, Content: body})}})
}
