//go:build !board_biscuit

package settings

import (
	"github.com/ygelfand/echolocal/internal/openclaw/talk"
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
	if p.Title != "Begrüßungsprompt" || rows[0].Value != talk.GreetingForLanguage("de") || len(rows) != 2 || len(saved) != 0 {
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
