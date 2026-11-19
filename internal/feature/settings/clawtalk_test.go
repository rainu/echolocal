//go:build !board_biscuit

package settings

import (
	"github.com/ygelfand/echolocal/internal/feature/clawtalk"
	"github.com/ygelfand/libcountertop/pkg/say"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
	"strings"
	"testing"
)

func TestTalkMenusTranslateAndKeepRoutingIdentifiersBackendOnly(t *testing.T) {
	before := say.Chosen()
	defer say.Use(before)
	for _, lang := range say.Languages() {
		say.Use(lang)
		rows, acts, replace := talkOptionsRowsFor(0, clawtalk.Slot{OpenClaw: true, Agent: "synthetic-private-agent", Context: "agent:synthetic-private-agent:private", Resume: true, Timeout: 60})
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
		targetRows, _, targetReplace := talkRowsFor(0, clawtalk.Slot{OpenClaw: true})
		if !targetReplace || len(targetRows) != 1 || targetRows[0].Label != say.T("clawtalk.target") {
			t.Fatal("OpenClaw options remain in assistant menu")
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

func TestOpenClawRootHasCentralSlotAndHistoryNavigation(t *testing.T) {
	rows, acts := openclawPage().Build()
	if len(rows) != 6 || len(acts) != 6 {
		t.Fatal("unexpected central navigation")
	}
	for _, r := range rows {
		if r.Open == nil {
			t.Fatal("central row has no destination")
		}
	}
	if rows[5].Label != say.T("clawtalk.history") {
		t.Fatal("global history missing")
	}
}

func TestTonePreviewDoesNotCommitAndCatalogueMatchesOtherMenus(t *testing.T) {
	played, saved := 0, 0
	p := talkToneDetail("synthetic tone", talkChoice{"ding", "synthetic"}, func() { played++ }, func() { saved++ })
	p.Build()
	if played != 0 || saved != 0 {
		t.Fatal("opening detail caused side effect")
	}
	p.Actions[0].Do()
	p.Actions[0].Do()
	if played != 2 || saved != 0 {
		t.Fatal("preview changed selection")
	}
	p.Actions[1].Do()
	if saved != 1 || played != 2 {
		t.Fatal("selection did not stay explicit")
	}
}

func TestTalkToneCatalogueIncludesCallAndAlertSounds(t *testing.T) {
	choices := talkBuiltinToneChoices()
	seen := map[string]bool{}
	for _, c := range choices {
		if seen[c.id] {
			t.Fatal("duplicate tone")
		}
		seen[c.id] = true
	}
	if !seen[""] {
		t.Fatal("default missing")
	}
	for _, c := range schema.CallChimes() {
		if !seen[string(c)] {
			t.Fatalf("shared tone omitted: %s", c)
		}
	}
}

func TestToneDetailActionsTranslateExplicitly(t *testing.T) {
	before := say.Chosen()
	defer say.Use(before)
	for _, lang := range say.Languages() {
		say.Use(lang)
		p := talkToneDetail("synthetic", talkChoice{"none", "synthetic"}, func() {}, func() {})
		for i, k := range []string{"clawtalk.tone_preview", "clawtalk.tone_use"} {
			if p.Actions[i].Label != say.T(k) || p.Actions[i].Label == k {
				t.Fatalf("bad tone action in %s", lang)
			}
		}
	}
}
