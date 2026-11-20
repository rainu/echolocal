//go:build !board_biscuit

package settings

import (
	"errors"
	"github.com/ygelfand/echolocal/internal/feature/clawtalk"
	"github.com/ygelfand/libcountertop/pkg/display/menu"
	sharedshell "github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/say"
	"testing"
)

func TestEndSettingsUIIsLocalizedSingleInputAndIndependentChoices(t *testing.T) {
	before := say.Chosen()
	say.Use("de")
	if endText("exact") != "Enthält" {
		t.Fatal("misleading whole-utterance label")
	}
	defer say.Use(before)
	for _, lang := range say.Languages() {
		say.Use(lang)
		stored := clawtalk.Slot{}
		p := talkEndPageFor(func() clawtalk.Slot { return stored }, func(change func(*clawtalk.Slot)) error {
			next := stored
			next.Ending = next.Ending.Defaults(lang)
			change(&next)
			if err := next.Ending.Validate(); err != nil {
				return err
			}
			stored = next
			return nil
		}, func() {})
		rows, acts := p.Build()
		fields := 0
		for _, r := range rows {
			if r.Kind == menu.Field {
				fields++
			}
			if r.Label == "" {
				t.Fatal("missing translation", lang)
			}
		}
		if len(rows) != 5 {
			t.Fatal("unexpected help text rows", len(rows))
		}
		if fields != 1 || len(rows) != len(acts) || rows[0].Value == "" {
			t.Fatal("missing single initialized input")
		}
		rows[0].Save("custom phrase")
		for _, position := range []string{"exact", "start", "end"} {
			rows, _ = p.Build()
			picker := rows[1].Open().(*sharedshell.Page)
			options, actions := picker.Build()
			for i, k := range []string{"exact", "start", "end"} {
				if k == position {
					actions[i](i)
					if options[i].Label == "" {
						t.Fatal("position translation")
					}
				}
			}
			rows, _ = p.Build()
			timing := rows[2].Open().(*sharedshell.Page)
			_, actions = timing.Build()
			actions[1](1)
			if stored.Ending.Phrase != "custom phrase" || stored.Ending.Position != position || stored.Ending.Timing != "immediate" {
				t.Fatal("choices coupled")
			}
		}
		rows, _ = p.Build()
		rows[0].Save("")
		rows, _ = p.Build()
		if stored.Ending.Phrase != "custom phrase" || rows[len(rows)-1].Label != endText("invalid") {
			t.Fatal("invalid edit lost phrase or feedback")
		}
	}
	p := talkEndPageFor(func() clawtalk.Slot { return clawtalk.Slot{} }, func(func(*clawtalk.Slot)) error { return errors.New("talk_storage") }, func() {})
	rows, _ := p.Build()
	rows[0].Save("custom")
	rows, _ = p.Build()
	if rows[len(rows)-1].Label != say.T("openclaw.error.storage") {
		t.Fatal("storage failure hidden")
	}
}
