//go:build !board_biscuit

package settings

import (
	"github.com/ygelfand/echolocal/internal/networkmedia"
	"github.com/ygelfand/libcountertop/pkg/display/menu"
	"github.com/ygelfand/libcountertop/pkg/say"
	"path/filepath"
	"testing"
)

func TestSMBNativeSharedConfigurationAndSecretSemantics(t *testing.T) {
	before := say.Chosen()
	t.Cleanup(func() { say.Use(before) })
	for _, lang := range []string{"en", "de", "it", "vi"} {
		say.Use(lang)
		s := networkmedia.New(filepath.Join(t.TempDir(), "source.json"))
		page := smbPageFor(s, func() {})
		rows, taps := page.Build()
		if len(rows) != 9 || rows[7].Kind != menu.Password || rows[7].Value != "" || rows[1].Kind != menu.Toggle {
			t.Fatal("missing native source fields", lang)
		}
		for _, r := range rows {
			if r.Label == "" {
				t.Fatal("missing native label", lang)
			}
		}
		rows[2].Save("server.example")
		rows[4].Save("videos")
		rows[5].Save("reader")
		rows[7].Save("synthetic-secret")
		taps[1](0)
		page.Actions[0].Do()
		if !s.View().Enabled || !s.View().PasswordSet {
			t.Fatal("native save", lang)
		}
		rows, _ = page.Build()
		if rows[7].Value != "" {
			t.Fatal("saved password prefills input")
		}
		u := networkmedia.Update{Settings: s.View().Settings}
		u.Share = "changed-through-owner-api"
		if e := s.Save(u); e != nil {
			t.Fatal(e)
		}
		rows, _ = page.Build()
		if rows[4].Value != u.Share {
			t.Fatal("external update missing after native refresh")
		}
		page.Actions[0].Do()
		if !s.View().PasswordSet {
			t.Fatal("omission erased password")
		}
		page.Actions[1].Do()
		if s.View().Enabled || s.View().PasswordSet {
			t.Fatal("native clear")
		}
	}
}
