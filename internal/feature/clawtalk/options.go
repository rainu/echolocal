//go:build !board_biscuit

package clawtalk

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/ygelfand/echolocal/internal/feature/tones"
	"github.com/ygelfand/libcountertop/pkg/say"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

type Choice struct {
	Value string `json:"value"`
	Label string `json:"label"`
}
type CatalogProvider struct {
	ID, Label                  string
	Configured                 bool
	SupportsToolCalls          *bool
	Transports, Brains, Models []string
	VoicesByModel              map[string][]string
}
type ModelSelection struct{ Provider, Model string }
type TalkCaller interface {
	CallTalk(context.Context, string, any, any) error
}

func SafeLabel(text, id string, index int, key string) string {
	text = strings.TrimSpace(text)
	if text == "" || text == id || strings.Contains(text, "agent:") {
		return say.F(key, map[string]any{"N": index + 1})
	}
	r := []rune(text)
	if len(r) > 48 {
		r = r[:48]
	}
	return string(r)
}
func AgentChoices(ctx context.Context, c TalkCaller) ([]Choice, error) {
	var result struct {
		Agents []struct {
			ID, Name string
			Identity struct{ Name string }
		}
	}
	err := c.CallTalk(ctx, "agents.list", map[string]any{}, &result)
	out := []Choice{}
	for i, a := range result.Agents {
		n := a.Identity.Name
		if n == "" {
			n = a.Name
		}
		out = append(out, Choice{a.ID, SafeLabel(n, a.ID, i, "clawtalk.agent_number")})
	}
	return out, err
}
func SessionChoices(ctx context.Context, c TalkCaller, agent string) ([]Choice, error) {
	var result struct {
		Sessions []struct{ Key, Label, DisplayName, DerivedTitle string }
	}
	err := c.CallTalk(ctx, "sessions.list", map[string]any{"agentId": agent, "limit": 40, "includeDerivedTitles": true}, &result)
	out := []Choice{{"", say.T("clawtalk.new_context")}}
	for i, s := range result.Sessions {
		if agent == "" || !strings.HasPrefix(s.Key, "agent:"+agent+":") {
			continue
		}
		n := s.Label
		if n == "" {
			n = s.DisplayName
		}
		if n == "" {
			n = s.DerivedTitle
		}
		out = append(out, Choice{s.Key, SafeLabel(n, s.Key, i, "clawtalk.context_number")})
	}
	return out, err
}
func ModelChoices(providers []CatalogProvider) []Choice {
	out := []Choice{{"", say.T("clawtalk.gateway_default")}}
	contains := func(list []string, v string) bool {
		for _, x := range list {
			if x == v {
				return true
			}
		}
		return false
	}
	for _, p := range providers {
		if p.ID != "openai" || !p.Configured || !contains(p.Transports, "webrtc") || !contains(p.Brains, "agent-consult") {
			continue
		}
		native := []string{}
		for model, voices := range p.VoicesByModel {
			if len(voices) > 0 {
				native = append(native, model)
			}
		}
		sort.Strings(native)
		for _, model := range native {
			id, _ := json.Marshal(ModelSelection{p.ID, model})
			out = append(out, Choice{string(id), model + " · " + p.Label})
		}
	}
	return out
}
func LiveModelChoices(ctx context.Context, c TalkCaller, session string) ([]Choice, error) {
	var catalog struct {
		Realtime struct{ Providers []CatalogProvider }
	}
	params := map[string]any{}
	if session != "" {
		params["sessionKey"] = session
	}
	err := c.CallTalk(ctx, "talk.catalog", params, &catalog)
	return ModelChoices(catalog.Realtime.Providers), err
}
func ToneChoices() []Choice {
	out := []Choice{{"", say.T("clawtalk.default_tone")}}
	for _, v := range schema.CallChimes() {
		out = append(out, Choice{string(v), v.Label()})
	}
	for _, v := range tones.Get().Choices() {
		out = append(out, Choice{v.Value, v.Label})
	}
	return out
}
