//go:build !board_biscuit

package settings

import (
	"github.com/ygelfand/echolocal/internal/feature/clawtalk"
	"github.com/ygelfand/echolocal/internal/openclaw"
	"github.com/ygelfand/echolocal/internal/openclaw/talk"
	"github.com/ygelfand/libcountertop/pkg/display/menu"
	"github.com/ygelfand/libcountertop/pkg/say"
	"strings"
	"testing"
)

func TestGreetingEditorDraftExplicitSaveAndRestoreDefault(t *testing.T) {
	old := say.Chosen()
	defer say.Use(old)
	say.Use("de")
	saved := []string{}
	p := greetingPromptPageFor("", func(s string) error { saved = append(saved, s); return nil }, func() {})
	rows, _ := p.Build()
	if p.Title != "Begrüßungsprompt" || rows[0].Value != talk.GreetingForLanguage("de") || len(rows) != 1 || len(saved) != 0 {
		t.Fatal("default editor includes device placeholders or persisted on open")
	}
	custom := "Sage nur guten Abend.\nDann höre zu."
	rows[0].Save(custom)
	if len(saved) != 0 {
		t.Fatal("draft saved implicitly")
	}
	say.Use("en")
	rows, _ = p.Build()
	if rows[0].Value != custom {
		t.Fatal("custom prompt translated")
	}
	p.Actions[1].Do()
	if len(saved) != 1 || saved[0] != custom {
		t.Fatal("explicit save lost custom text")
	}
	rows, _ = p.Build()
	rows[0].Save(strings.Repeat("ü", talk.GreetingMaxBytes))
	p.Actions[1].Do()
	if len(saved) != 1 {
		t.Fatal("oversized draft persisted")
	}
	p.Actions[0].Do()
	rows, _ = p.Build()
	if len(saved) != 1 || rows[0].Value != talk.GreetingForLanguage("en") {
		t.Fatal("reset did not stay in draft or follow app language")
	}
	p.Actions[1].Do()
	if len(saved) != 2 || saved[1] != "" {
		t.Fatal("default sentinel not restored")
	}
}

func TestConversationContextHasTwoInlinePromptsAndAtomicSave(t *testing.T) {
	initial := clawtalk.Slot{OpenClaw: true, DeviceContextTemplate: "Original node", GreetingPrompt: "Original greeting"}
	writes := 0
	saved := initial
	render := func(s string) (string, error) { return s, openclaw.ValidateDeviceContextTemplate(s) }
	p := talkConversationPromptPageFor(initial, render, func(d, g string) error {
		writes++
		saved.DeviceContextTemplate = d
		saved.GreetingPrompt = g
		return nil
	}, func() {})
	fields := func() []menu.Row {
		rows, _ := p.Build()
		for _, row := range rows {
			if row.Label == promptText("next") {
				t.Fatal("redundant next-conversation hint remains")
			}
		}
		result := []menu.Row{}
		for _, r := range rows {
			if r.Kind == menu.Field {
				result = append(result, r)
			}
		}
		return result
	}
	f := fields()
	if len(f) != 2 || f[0].Label != greetingPromptTitle() || f[1].Label != promptText("title") || f[1].Open != nil {
		t.Fatal("prompts not separate inline fields")
	}
	f[0].Save("New greeting")
	f[1].Save("New node")
	if writes != 0 {
		t.Fatal("draft writes immediately")
	}
	p.Actions[1].Do()
	if writes != 1 || saved.DeviceContextTemplate != "New node" || saved.GreetingPrompt != "New greeting" {
		t.Fatal("save not one combined transaction")
	}
	f = fields()
	f[1].Save("Next node")
	f[0].Save(strings.Repeat("x", talk.GreetingMaxBytes+1))
	p.Actions[1].Do()
	if writes != 1 || saved.DeviceContextTemplate != "New node" {
		t.Fatal("invalid greeting partially committed device prompt")
	}
	p.Actions[0].Do()
	if writes != 1 {
		t.Fatal("reset saved implicitly")
	}
	p.Actions[1].Do()
	if writes != 2 || saved.DeviceContextTemplate != "" || saved.GreetingPrompt != "" {
		t.Fatal("both defaults not restored atomically")
	}
	rows, _, _ := talkOptionsRowsFor(0, initial)
	for _, row := range rows {
		if row.Label == greetingPromptTitle() {
			t.Fatal("greeting still in main assistant menu")
		}
	}
}
