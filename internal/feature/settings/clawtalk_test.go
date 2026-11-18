//go:build !board_biscuit

package settings

import (
	"github.com/ygelfand/echolocal/internal/feature/clawtalk"
	"github.com/ygelfand/libcountertop/pkg/say"
	"strings"
	"testing"
)

func TestTalkMenusTranslateAndKeepRoutingIdentifiersBackendOnly(t *testing.T) {
	before := say.Chosen()
	defer say.Use(before)
	for _, lang := range say.Languages() {
		say.Use(lang)
		rows, acts, replace := talkRowsFor(0, clawtalk.Slot{OpenClaw: true, Agent: "synthetic-private-agent", Context: "agent:synthetic-private-agent:private", Resume: true, Timeout: 60})
		if !replace || len(rows) != len(acts) {
			t.Fatal("routing row/action mismatch")
		}
		for _, row := range rows {
			for _, value := range []string{row.Label, row.Hint, row.Value} {
				if strings.Contains(value, "synthetic-private") || strings.Contains(value, "agent:") || strings.Contains(value, "clawtalk.") {
					t.Errorf("%s identifier leaked into presentation", lang)
				}
			}
		}
		rows, _, replace = talkRowsFor(1, clawtalk.Slot{})
		if replace || len(rows) != 1 || rows[0].Value != "Home Assistant" {
			t.Fatal("HA settings changed by default")
		}
	}
}

func TestVoiceModelPickerIsDrivenByGatewayCatalogue(t *testing.T) {
	choices := talkModelChoices([]talkModelProvider{
		{ID: "openai", Label: "Synthetic", Configured: true, Transports: []string{"webrtc"}, Brains: []string{"agent-consult"}, VoicesByModel: map[string][]string{"future-model-2099": {"synthetic-voice"}}},
		{ID: "unconfigured", Configured: false, Transports: []string{"webrtc"}, Brains: []string{"agent-consult"}, VoicesByModel: map[string][]string{"must-not-appear": {"voice"}}},
	})
	if len(choices) != 2 || choices[0].id != "" || !strings.Contains(choices[1].label, "future-model-2099") {
		t.Fatal("models frozen to client version or invalid provider exposed")
	}
}
