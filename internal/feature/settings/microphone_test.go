//go:build !board_biscuit

package settings

import (
	"github.com/ygelfand/libcountertop/pkg/display/menu"
	sharedshell "github.com/ygelfand/libcountertop/pkg/display/shell"
	"testing"
)

func TestMicrophoneTestViewExposesPageToRenderer(t *testing.T) {
	p := &microphoneTestView{Page: &sharedshell.Page{Title: "Microphone test"}}
	if sharedshell.SettingsPage(p) != p.Page {
		t.Fatal("wrapped test page is invisible to renderer")
	}
}
func TestMicrophonePageKeepsProcessingKnobsNextToTest(t *testing.T) {
	rows, taps := microphonePage().Build()
	if len(rows) != 6 || len(taps) != len(rows) || rows[0].Open == nil {
		t.Fatalf("rows=%d taps=%d", len(rows), len(taps))
	}
	sliders, toggles := 0, 0
	for _, r := range rows {
		if r.Kind == menu.Slider {
			sliders++
		}
		if r.Kind == menu.Toggle {
			toggles++
		}
	}
	if sliders != 2 || toggles != 2 {
		t.Fatalf("sliders=%d toggles=%d", sliders, toggles)
	}
}
