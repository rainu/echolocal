//go:build !board_biscuit

package settings

import (
	"testing"

	"github.com/ygelfand/libcountertop/pkg/say"
)

func TestYouTubeConnectionLabelsAndServiceEntryAreLocalized(t *testing.T) {
	before := say.Chosen()
	t.Cleanup(func() { say.Use(before) })
	for _, lang := range []string{"en", "de", "it", "vi"} {
		say.Use(lang)
		for _, key := range []string{"checking", "disconnected", "linking", "connected", "expired", "error", "status", "premium", "code", "hint", "connect", "refresh", "cancel", "disconnect", "confirm", "remove", "back"} {
			if youtubeText(key) == "" {
				t.Errorf("missing %s/%s", lang, key)
			}
		}
		if say.T("features.youtube") != "YouTube" {
			t.Errorf("service entry missing for %s", lang)
		}
	}
}
