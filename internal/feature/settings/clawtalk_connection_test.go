//go:build !board_biscuit

package settings

import (
	"github.com/ygelfand/echolocal/internal/feature/clawtalk"
	"github.com/ygelfand/libcountertop/pkg/say"
	"testing"
)

func TestConnectionToneHasIndependentLocalizedSelection(t *testing.T) {
	old := say.Chosen()
	defer say.Use(old)
	for _, lang := range []string{"en", "de", "it", "vi"} {
		say.Use(lang)
		rows, acts, _ := talkOptionsRowsFor(0, clawtalk.Slot{OpenClaw: true, ProgressTone: "none", StartTone: "ding"})
		found := false
		if len(rows) != len(acts) {
			t.Fatal("row/action mismatch")
		}
		for _, row := range rows {
			if row.Label == progressToneTitle() {
				found = true
				if row.Open == nil || row.Value != talkToneLabel("none") {
					t.Fatal("connection setting not independent")
				}
			}
		}
		if !found {
			t.Fatal("connection picker missing", lang)
		}
	}
}
