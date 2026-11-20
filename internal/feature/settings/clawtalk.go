//go:build !board_biscuit

package settings

import (
	"context"
	"encoding/json"
	"github.com/ygelfand/echolocal/internal/config"
	"github.com/ygelfand/echolocal/internal/feature/clawtalk"
	"github.com/ygelfand/echolocal/internal/feature/detect"
	"github.com/ygelfand/echolocal/internal/feature/shell"
	"github.com/ygelfand/echolocal/internal/feature/tones"
	"github.com/ygelfand/echolocal/internal/feature/voice"
	"github.com/ygelfand/echolocal/internal/lib/wake"
	"github.com/ygelfand/libcountertop/pkg/display/menu"
	sharedshell "github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/say"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func talkRows(slot int) ([]menu.Row, []func(int), bool) {
	return talkRowsFor(slot, clawtalk.Get().Store.Get(slot))
}
func talkRowsFor(slot int, s clawtalk.Slot) ([]menu.Row, []func(int), bool) {
	target := "Home Assistant"
	if s.OpenClaw {
		target = "OpenClaw"
	}
	rows := []menu.Row{{Label: say.T("clawtalk.target"), Value: target, Kind: menu.Chevron, Open: func() menu.View { return talkTargetPage(slot) }}}
	acts := []func(int){nil}
	return rows, acts, s.OpenClaw
}
func talkOptionsRowsFor(slot int, s clawtalk.Slot) ([]menu.Row, []func(int), bool) {
	rows := []menu.Row{}
	acts := []func(int){}
	if !s.OpenClaw {
		return []menu.Row{{Label: say.T("clawtalk.configure_target"), Kind: menu.Plain}}, []func(int){nil}, false
	}
	if talkSaveFailed.Load() {
		rows = append(rows, menu.Row{Label: say.T("openclaw.error.storage"), Kind: menu.Plain})
		acts = append(acts, nil)
	}
	agent := s.AgentLabel
	if agent == "" {
		agent = say.T("clawtalk.select_agent")
	}
	model := s.Model
	if model == "" {
		model = say.T("clawtalk.gateway_default")
	}

	contextSummary := say.T("clawtalk.new_each")
	if s.Resume {
		contextSummary = say.T("clawtalk.resume")
	}
	rows = append(rows,
		menu.Row{Label: say.T("clawtalk.agent"), Value: agent, Kind: menu.Chevron, Open: func() menu.View { return talkAgentPage(slot) }},
		menu.Row{Label: say.T("clawtalk.model"), Value: model, Hint: say.T("clawtalk.model_hint"), Kind: menu.Chevron, Open: func() menu.View { return talkModelPage(slot) }},
		menu.Row{Label: say.T("clawtalk.context"), Value: contextSummary, Kind: menu.Chevron, Open: func() menu.View { return talkConversationContextPage(slot) }},
		menu.Row{Label: say.T("clawtalk.start_tone"), Value: talkToneLabel(s.StartTone), Kind: menu.Chevron, Open: func() menu.View { return talkTonePage(slot, false) }},
		menu.Row{Label: say.T("clawtalk.end_tone"), Value: talkToneLabel(s.EndTone), Kind: menu.Chevron, Open: func() menu.View { return talkTonePage(slot, true) }},
	)
	acts = append(acts, nil, nil, nil, nil, nil)

	rows = append(rows, menu.Row{Label: endText("title"), Value: s.Ending.Defaults(say.Chosen()).Phrase, Kind: menu.Chevron, Open: func() menu.View { return talkEndPage(slot) }})
	acts = append(acts, nil)
	rows = append(rows, menu.Row{Label: say.T("clawtalk.timeout"), Value: say.F("clawtalk.seconds", map[string]any{"N": s.Timeout}), Kind: menu.Chevron, Open: func() menu.View { return talkTimeoutPage(slot) }})
	acts = append(acts, nil)
	return rows, acts, true
}

// Conversation policy, existing-session selection and device prompt belong
// together; audio/model settings remain on the assistant's main options page.
func talkConversationContextPage(slot int) *TalkPromptEditor {
	p := talkDevicePromptPage(slot)
	p.Title = say.T("clawtalk.context")
	p.Leading = func() ([]menu.Row, []func(int)) {
		return talkContextSettingsRowsFor(slot, clawtalk.Get().Store.Get(slot))
	}
	return p
}

func talkContextSettingsRowsFor(slot int, s clawtalk.Slot) ([]menu.Row, []func(int)) {
	rows := []menu.Row{}
	acts := []func(int){}
	if talkSaveFailed.Load() {
		rows = append(rows, menu.Row{Label: say.T("openclaw.error.storage"), Kind: menu.Plain})
		acts = append(acts, nil)
	}
	rows = append(rows, menu.Row{Label: say.T("clawtalk.resume"), Hint: say.T("clawtalk.resume_hint"), Kind: menu.Toggle, On: s.Resume})
	acts = append(acts, func(int) { saveTalk(slot, func(v *clawtalk.Slot) { v.Resume = !s.Resume }); shell.Get().Redraw() })
	if s.Resume {
		label := s.ContextLabel
		if label == "" {
			label = say.T("clawtalk.new_context")
			if s.Context != "" {
				label = say.T("clawtalk.device_context")
			}
		}
		rows = append(rows, menu.Row{Label: promptText("session"), Value: label, Kind: menu.Chevron, Open: func() menu.View { return talkContextPage(slot) }})
	} else {
		rows = append(rows, menu.Row{Label: say.T("clawtalk.new_each"), Kind: menu.Plain})
	}
	acts = append(acts, nil)
	return rows, acts
}

func talkTargetPage(slot int) *sharedshell.Page {
	return &sharedshell.Page{Title: say.T("clawtalk.target"), Build: func() ([]menu.Row, []func(int)) {
		s := clawtalk.Get().Store.Get(slot)
		choose := func(on bool) func(int) {
			return func(int) {
				go func() {
					clawtalk.Get().Stop()
					if on && claw().EnableTalk() != nil {
						talkSaveFailed.Store(true)
						shell.Get().Redraw()
						return
					}
					if saveTalk(slot, func(v *clawtalk.Slot) { v.OpenClaw = on }) == nil {
						shell.Get().Pop()
					}
				}()
			}
		}
		return []menu.Row{{Label: "Home Assistant", Chosen: !s.OpenClaw}, {Label: "OpenClaw", Chosen: s.OpenClaw}}, []func(int){choose(false), choose(true)}
	}}
}

type talkChoice struct{ id, label string }

func safeTalkLabel(text, id string, index int, key string) string {
	text = strings.TrimSpace(text)
	if text == "" || text == id || strings.Contains(text, "agent:") {
		return say.F(key, map[string]any{"N": index + 1})
	}
	r := []rune(text)
	if len(r) > 48 {
		return string(r[:48])
	}
	return text
}
func talkPicker(title string, read func(context.Context) ([]talkChoice, error), choose func(talkChoice)) *sharedshell.Page {
	var mu sync.Mutex
	var choices []talkChoice
	loading := true
	failed := false
	page := &sharedshell.Page{Title: title}
	page.OnOpen = func() {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			list, e := read(ctx)
			mu.Lock()
			choices = list
			loading = false
			failed = e != nil
			mu.Unlock()
			shell.Get().Redraw()
		}()
	}
	page.Build = func() ([]menu.Row, []func(int)) {
		mu.Lock()
		defer mu.Unlock()
		if loading {
			return []menu.Row{{Label: say.T("clawtalk.loading"), Kind: menu.Plain}}, []func(int){nil}
		}
		if failed {
			return []menu.Row{{Label: say.T("clawtalk.unavailable"), Kind: menu.Plain}}, []func(int){nil}
		}
		var rows []menu.Row
		var acts []func(int)
		for _, v := range choices {
			v := v
			rows = append(rows, menu.Row{Label: v.label})
			acts = append(acts, func(int) { choose(v) })
		}
		return rows, acts
	}
	return page
}
func talkAgentPage(slot int) *sharedshell.Page {
	return talkPicker(say.T("clawtalk.agent"), func(ctx context.Context) ([]talkChoice, error) {
		var result struct {
			Agents []struct {
				ID, Name string
				Identity struct{ Name string }
			}
		}
		e := claw().CallTalk(ctx, "agents.list", map[string]any{}, &result)
		var out []talkChoice
		for i, a := range result.Agents {
			n := a.Identity.Name
			if n == "" {
				n = a.Name
			}
			out = append(out, talkChoice{a.ID, safeTalkLabel(n, a.ID, i, "clawtalk.agent_number")})
		}
		return out, e
	}, func(v talkChoice) {
		clawtalk.Get().Stop()
		if saveTalk(slot, func(s *clawtalk.Slot) { s.Agent = v.id; s.AgentLabel = v.label; s.Context = ""; s.ContextLabel = "" }) == nil {
			shell.Get().Pop()
		}
	})
}
func talkContextPage(slot int) *sharedshell.Page {
	saved := clawtalk.Get().Store.Get(slot)
	return talkPicker(promptText("session"), func(ctx context.Context) ([]talkChoice, error) {
		var result struct {
			Sessions []struct{ Key, Label, DisplayName, DerivedTitle string }
		}
		e := claw().CallTalk(ctx, "sessions.list", map[string]any{"agentId": saved.Agent, "limit": 40, "includeDerivedTitles": true}, &result)
		out := []talkChoice{{"", say.T("clawtalk.new_context")}}
		for i, s := range result.Sessions {
			// Metadata only; no transcript/history request.
			if !strings.HasPrefix(s.Key, "agent:"+saved.Agent+":") {
				continue
			}
			n := s.Label
			if n == "" {
				n = s.DisplayName
			}
			if n == "" {
				n = s.DerivedTitle
			}
			out = append(out, talkChoice{s.Key, safeTalkLabel(n, s.Key, i, "clawtalk.context_number")})
		}
		return out, e
	}, func(v talkChoice) {
		clawtalk.Get().Stop()
		if saveTalk(slot, func(s *clawtalk.Slot) { s.Context = v.id; s.ContextLabel = v.label; s.Resume = true }) == nil {
			shell.Get().Pop()
		}
	})
}
func talkTimeoutPage(slot int) *sharedshell.Page {
	return &sharedshell.Page{Title: say.T("clawtalk.timeout"), Build: func() ([]menu.Row, []func(int)) {
		var rows []menu.Row
		var acts []func(int)
		for _, n := range []int{15, 30, 60, 120, 300, 900} {
			n := n
			rows = append(rows, menu.Row{Label: say.F("clawtalk.seconds", map[string]any{"N": n}), Chosen: clawtalk.Get().Store.Get(slot).Timeout == n})
			acts = append(acts, func(int) {
				saveTalk(slot, func(s *clawtalk.Slot) { s.Timeout = n })
				shell.Get().Pop()
			})
		}
		return rows, acts
	}}
}
func talkWordRow(slot int) (menu.Row, func(int), bool) {
	if !clawtalk.Get().Store.Get(slot).OpenClaw {
		return menu.Row{}, nil, false
	}
	id := config.Get().Wake.Slot(slot).ID
	phrase := say.T("assistants.off")
	for _, m := range wake.Lib().Ours() {
		if m.ID == id {
			phrase = m.Phrase
		}
	}
	return menu.Row{Label: say.T("assistants.word"), Value: phrase, Kind: menu.Chevron, Open: func() menu.View {
		return &sharedshell.Page{Title: say.T("assistants.word"), Build: func() ([]menu.Row, []func(int)) {
			rows := []menu.Row{{Label: say.T("assistants.off"), Chosen: id == ""}}
			ids := []string{""}
			for _, m := range wake.Lib().Ours() {
				rows = append(rows, menu.Row{Label: m.Phrase, Chosen: m.ID == id})
				ids = append(ids, m.ID)
			}
			var acts []func(int)
			for _, modelID := range ids {
				modelID := modelID
				acts = append(acts, func(int) {
					if config.Set().Wake(slot).ID(modelID) == nil {
						slots := voice.Get().Slots()
						for len(slots) <= slot {
							slots = append(slots, "")
						}
						slots[slot] = modelID
						voice.Get().SetSlots(slots)
						detect.Get().Load()
						shell.Get().Pop()
					}
				})
			}
			return rows, acts
		}}
	}}, nil, true
}

var talkSaveFailed atomic.Bool

func saveTalk(slot int, change func(*clawtalk.Slot)) error {
	err := clawtalk.Get().Store.Update(slot, change)
	talkSaveFailed.Store(err != nil)
	shell.Get().Redraw()
	return err
}

type talkModelProvider struct {
	ID, Label                  string
	Configured                 bool
	SupportsToolCalls          *bool
	Transports, Brains, Models []string
	VoicesByModel              map[string][]string
}
type talkModelSelection struct{ Provider, Model string }

func talkModelChoices(providers []talkModelProvider) []talkChoice {
	out := []talkChoice{{"", say.T("clawtalk.gateway_default")}}
	for _, provider := range providers {
		if provider.ID != "openai" || !provider.Configured || !containsTalkValue(provider.Transports, "webrtc") || !containsTalkValue(provider.Brains, "agent-consult") {
			continue
		}
		// Advertised per-model voice profiles are the Gateway's native Live model
		// catalogue. Never freeze a list of current model names in this client.
		native := []string{}
		for model, voices := range provider.VoicesByModel {
			if len(voices) > 0 {
				native = append(native, model)
			}
		}
		sort.Strings(native)
		for _, model := range native {
			id, _ := json.Marshal(talkModelSelection{provider.ID, model})
			out = append(out, talkChoice{string(id), model + " · " + provider.Label})
		}
	}
	return out
}
func containsTalkValue(list []string, value string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}
func talkModelPage(slot int) *sharedshell.Page {
	saved := clawtalk.Get().Store.Get(slot)
	return talkPicker(say.T("clawtalk.model"), func(ctx context.Context) ([]talkChoice, error) {
		var catalog struct {
			Realtime struct{ Providers []talkModelProvider }
		}
		params := map[string]any{}
		if saved.Context != "" {
			params["sessionKey"] = saved.Context
		}
		err := claw().CallTalk(ctx, "talk.catalog", params, &catalog)
		return talkModelChoices(catalog.Realtime.Providers), err
	}, func(v talkChoice) {
		var selected talkModelSelection
		if v.id != "" && json.Unmarshal([]byte(v.id), &selected) != nil {
			return
		}
		clawtalk.Get().Stop()
		if saveTalk(slot, func(s *clawtalk.Slot) { s.Provider = selected.Provider; s.Model = selected.Model }) == nil {
			shell.Get().Pop()
		}
	})
}

func talkToneLabel(choice string) string {
	if choice == "" {
		return say.T("clawtalk.default_tone")
	}
	for _, option := range tones.Get().Choices() {
		if option.Value == choice {
			return option.Label
		}
	}
	return schema.Chime(choice).Label()
}

// Use the largest normal built-in catalogue shared by calls/alerts, plus
// installed custom clips. No per-feature hard-coded subset.
func talkBuiltinToneChoices() []talkChoice {
	choices := []talkChoice{{"", say.T("clawtalk.default_tone")}}
	for _, v := range schema.CallChimes() {
		choices = append(choices, talkChoice{string(v), v.Label()})
	}
	return choices
}
func talkToneChoices() []talkChoice {
	choices := talkBuiltinToneChoices()
	for _, v := range tones.Get().Choices() {
		choices = append(choices, talkChoice{v.Value, v.Label})
	}
	return choices
}
func talkToneDetail(title string, choice talkChoice, preview, use func()) *sharedshell.Page {
	return &sharedshell.Page{Title: title, Build: func() ([]menu.Row, []func(int)) {
		return []menu.Row{{Label: choice.label, Hint: say.T("clawtalk.tone_preview_hint"), Kind: menu.Plain}}, []func(int){nil}
	}, Actions: []sharedshell.Action{
		{Label: say.T("clawtalk.tone_preview"), MinHeight: 72, Do: preview},
		{Label: say.T("clawtalk.tone_use"), MinHeight: 72, Role: sharedshell.Primary, Do: use},
	}}
}
func talkTonePage(slot int, end bool) *sharedshell.Page {
	key := "clawtalk.start_tone"
	if end {
		key = "clawtalk.end_tone"
	}
	return &sharedshell.Page{Title: say.T(key), Build: func() ([]menu.Row, []func(int)) {
		saved := clawtalk.Get().Store.Get(slot)
		selected := saved.StartTone
		if end {
			selected = saved.EndTone
		}
		rows := []menu.Row{}
		for _, choice := range talkToneChoices() {
			rows = append(rows, menu.Row{Label: choice.label, Chosen: choice.id == selected, Kind: menu.Chevron, Open: func() menu.View {
				return talkToneDetail(say.T(key), choice, func() { clawtalk.Get().PreviewTone(choice.id, end) }, func() {
					if saveTalk(slot, func(s *clawtalk.Slot) {
						if end {
							s.EndTone = choice.id
						} else {
							s.StartTone = choice.id
						}
					}) == nil {
						shell.Get().Pop()
						shell.Get().Pop()
					}
				})
			}})
		}
		return rows, make([]func(int), len(rows))
	}}
}

func talkOptionsPage(slot int) *sharedshell.Page {
	return &sharedshell.Page{Title: say.F("assistants.slot", map[string]any{"N": slot + 1}), Build: func() ([]menu.Row, []func(int)) {
		rows, acts, _ := talkOptionsRowsFor(slot, clawtalk.Get().Store.Get(slot))
		return rows, acts
	}}
}
