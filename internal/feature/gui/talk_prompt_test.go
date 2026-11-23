//go:build !board_biscuit

package gui

import (
	gogui "github.com/go-gui-org/go-gui/gui"
	"github.com/ygelfand/libcountertop/pkg/display/menu"
	"testing"
)

func TestPromptInputPreservesMultilineEditsInDraft(t *testing.T) {
	draft := "First line\nSecond line"
	cfg := talkPromptInput(menu.Row{Value: draft, Save: func(v string) { draft = v }}, 180)
	if cfg.Mode != gogui.InputMultiline || !cfg.Scrollable || cfg.Height < 120 || cfg.Text != draft {
		t.Fatal("prompt is not a scrollable multiline input")
	}
	cfg.OnTextChanged("Erste Zeile\n\nDritte Zeile: {{device_name}} ä", gogui.EventCtx{})
	if draft != "Erste Zeile\n\nDritte Zeile: {{device_name}} ä" {
		t.Fatal("line breaks or unicode lost")
	}
}

func TestInlinePromptsHaveIndependentStableInputTargets(t *testing.T) {
	if talkPromptFieldID(0) != talkPromptInputID || talkPromptFieldID(0) == talkPromptFieldID(1) {
		t.Fatal("prompt input identities collide")
	}
}
