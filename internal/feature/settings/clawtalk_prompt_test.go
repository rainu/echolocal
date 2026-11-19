//go:build !board_biscuit

package settings

import (
	"github.com/ygelfand/echolocal/internal/openclaw"
	"github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/say"
	"strings"
	"testing"
)

func TestPromptEditorOnlyCommitsValidExplicitSaves(t *testing.T) {
	c := openclaw.NewNode(t.TempDir()+"/node", "test", openclaw.NodeMetadata{Name: "Synthetic", Model: "test"})
	saved := []string{}
	p := talkPromptPageFor("", c.ConversationDeviceContextWithTemplate, func(v string) error { saved = append(saved, v); return nil }, func() {})
	rows, _ := p.Build()
	if rows[0].Value != openclaw.DefaultDeviceContextTemplate() || len(saved) != 0 {
		t.Fatal("default/open changed persistence")
	}
	rows[0].Save("Custom {{device_name}}")
	if len(saved) != 0 {
		t.Fatal("editing saved without confirmation")
	}
	p.Actions[1].Do()
	if len(saved) != 1 || saved[0] != "Custom {{device_name}}" {
		t.Fatal("custom save failed")
	}
	rows, _ = p.Build()
	rows[0].Save("{{unknown}}")
	p.Actions[1].Do()
	if len(saved) != 1 {
		t.Fatal("invalid template committed")
	}
	rows, _ = p.Build()
	rows[0].Save(strings.Repeat("{{device_name}}", 200))
	p.Actions[1].Do()
	if len(saved) != 1 {
		t.Fatal("oversized template committed")
	}
	p.Actions[0].Do()
	if len(saved) != 1 {
		t.Fatal("reset bypassed explicit save")
	}
	p.Actions[1].Do()
	if len(saved) != 2 || saved[1] != "" {
		t.Fatal("reset did not persist default sentinel")
	}
	rows, _ = p.Build()
	help := rows[1].Open().(*shell.Page)
	r, a := help.Build()
	if len(r) != len(openclaw.DeviceContextPlaceholders) || len(r) != len(a) {
		t.Fatal("placeholder help incomplete")
	}
}
func TestPromptEditorLabelsAreLocalized(t *testing.T) {
	old := say.Chosen()
	defer say.Use(old)
	for _, lang := range say.Languages() {
		say.Use(lang)
		for _, key := range []string{"newline", "title", "template", "default", "custom", "next", "help", "reset", "pending", "tooLong", "placeholder", "invalid", "bytes", "insert"} {
			if promptText(key) == "" {
				t.Fatalf("missing %s/%s", lang, key)
			}
		}
		for _, key := range openclaw.DeviceContextPlaceholders {
			if promptText(key) == "" {
				t.Fatalf("missing placeholder help %s/%s", lang, key)
			}
		}
	}
}

func TestPromptCustomViewRetainsSettingsIdentity(t *testing.T) {
	p := &shell.Page{Title: "Synthetic prompt"}
	view := &TalkPromptEditor{Page: p}
	if shell.SettingsPage(view) != p {
		t.Fatal("custom editor lost settings protection/navigation")
	}
}

func TestDefaultPromptDraftTracksLanguageButCustomDraftDoesNot(t *testing.T) {
	old := say.Chosen()
	defer say.Use(old)
	say.Use("de")
	c := openclaw.NewNode(t.TempDir()+"/node", "test", openclaw.NodeMetadata{Name: "Synthetic", Model: "test"})
	p := talkPromptPageFor("", c.ConversationDeviceContextWithTemplate, func(string) error { return nil }, func() {})
	rows, _ := p.Build()
	if !strings.Contains(rows[0].Value, "nicht vorlesen") || rows[0].Label != "Prompt" {
		t.Fatal("German default/label incorrect")
	}
	say.Use("en")
	rows, _ = p.Build()
	if !strings.Contains(rows[0].Value, "Do not read") {
		t.Fatal("default draft language stale")
	}
	rows[0].Save("Eigener Text\n{{node_id}}")
	say.Use("it")
	rows, _ = p.Build()
	if rows[0].Value != "Eigener Text\n{{node_id}}" {
		t.Fatal("custom draft translated")
	}
	p.Actions[0].Do()
	rows, _ = p.Build()
	if !strings.Contains(rows[0].Value, "Non leggere") {
		t.Fatal("reset used wrong language")
	}
}
