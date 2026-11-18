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
	if talkSaveFailed.Load() {
		rows = append(rows, menu.Row{Label: say.T("openclaw.error.storage"), Kind: menu.Plain})
		acts = append(acts, nil)
	}
	if !s.OpenClaw {
		return rows, acts, false
	}
	agent := s.AgentLabel
	if agent == "" {
		agent = say.T("clawtalk.select_agent")
	}
	model := s.Model
	if model == "" {
		model = say.T("clawtalk.gateway_default")
	}
	rows = append(rows,
		menu.Row{Label: say.T("clawtalk.agent"), Value: agent, Kind: menu.Chevron, Open: func() menu.View { return talkAgentPage(slot) }},
		menu.Row{Label: say.T("clawtalk.model"), Value: model, Hint: say.T("clawtalk.model_hint"), Kind: menu.Chevron, Open: func() menu.View { return talkModelPage(slot) }},
		menu.Row{Label: say.T("clawtalk.resume"), Hint: say.T("clawtalk.resume_hint"), Kind: menu.Toggle, On: s.Resume},
	)
	acts = append(acts, nil, nil, func(int) {
		saveTalk(slot, func(v *clawtalk.Slot) { v.Resume = !s.Resume })
		shell.Get().Redraw()
	})
	contextLabel := s.ContextLabel
	if contextLabel == "" {
		contextLabel = say.T("clawtalk.new_context")
		if s.Context != "" {
			contextLabel = say.T("clawtalk.device_context")
		}
	}
	if s.Resume {
		rows = append(rows, menu.Row{Label: say.T("clawtalk.context"), Value: contextLabel, Kind: menu.Chevron, Open: func() menu.View { return talkContextPage(slot) }})
		acts = append(acts, nil)
	} else {
		rows = append(rows, menu.Row{Label: say.T("clawtalk.new_each"), Kind: menu.Plain})
		acts = append(acts, nil)
	}
	rows = append(rows,
		menu.Row{Label: say.T("clawtalk.start_tone"), Value: talkToneLabel(s.StartTone), Kind: menu.Chevron, Open: func() menu.View { return talkTonePage(slot, false) }},
		menu.Row{Label: say.T("clawtalk.end_tone"), Value: talkToneLabel(s.EndTone), Kind: menu.Chevron, Open: func() menu.View { return talkTonePage(slot, true) }})
	acts = append(acts, nil, nil)
	rows = append(rows, menu.Row{Label: say.T("clawtalk.timeout"), Value: say.F("clawtalk.seconds", map[string]any{"N": s.Timeout}), Kind: menu.Chevron, Open: func() menu.View { return talkTimeoutPage(slot) }})
	acts = append(acts, nil)
	return rows, acts, true
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
	return talkPicker(say.T("clawtalk.context"), func(ctx context.Context) ([]talkChoice, error) {
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
		choices := []talkChoice{{"", say.T("clawtalk.default_tone")}}
		for _, value := range schema.Chimes() {
			choices = append(choices, talkChoice{string(value), value.Label()})
		}
		for _, value := range tones.Get().Choices() {
			choices = append(choices, talkChoice{value.Value, value.Label})
		}
		var rows []menu.Row
		var acts []func(int)
		for _, choice := range choices {
			choice := choice
			rows = append(rows, menu.Row{Label: choice.label, Chosen: choice.id == selected})
			acts = append(acts, func(int) {
				clawtalk.Get().Stop()
				if saveTalk(slot, func(s *clawtalk.Slot) {
					if end {
						s.EndTone = choice.id
					} else {
						s.StartTone = choice.id
					}
				}) == nil {
					shell.Get().Pop()
				}
			})
		}
		return rows, acts
	}}
}
