//go:build !board_biscuit

package settings

import (
	"github.com/ygelfand/libcountertop/pkg/display/menu"
	sharedshell "github.com/ygelfand/libcountertop/pkg/display/shell"
	"testing"
)

// The native screen router accepts *shell.Page, not just SettingsPage providers.
func TestMicrophoneTestUsesConcreteNativePage(t *testing.T) {
	var view sharedshell.View = microphoneTestPage()
	page, ok := view.(*sharedshell.Page)
	if !ok || page.Build == nil || page.OnOpen == nil {
		t.Fatal("test page must use the concrete native page renderer")
	}
	rows, taps := page.Build()
	if len(rows) != len(taps) || len(rows) < 6 {
		t.Fatal("test controls missing")
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
