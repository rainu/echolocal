//go:build !board_biscuit

package settings

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ygelfand/echolocal/internal/openclaw"
	"github.com/ygelfand/libcountertop/pkg/display/menu"
	"github.com/ygelfand/libcountertop/pkg/say"
)

func TestOpenClawEditorLocalizesWithoutLosingCredentials(t *testing.T) {
	before := say.Chosen()
	t.Cleanup(func() { say.Use(before) })
	c := openclaw.New(filepath.Join(t.TempDir(), "connection.json"), "test")
	if err := c.Configure("wss://gateway.example.test", "synthetic-secret"); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct{ tag, save, disconnected, received string }{
		{"en", "Save", "Disconnected", "Received"},
		{"de", "Speichern", "Getrennt", "Empfangen"},
		{"it", "Salva", "Disconnesso", "Ricevuto"},
		{"vi", "Lưu", "Đã ngắt kết nối", "Nhận"},
	} {
		say.Use(tt.tag)
		page := openclawClientPage(c)
		rows, _ := page.Build()
		if rows[0].Value != tt.disconnected || page.Actions[len(page.Actions)-1].Label != tt.save {
			t.Errorf("%s editor did not localize", tt.tag)
		}
		if rows[2].Value != "wss://gateway.example.test" || rows[3].Value != "synthetic-secret" || rows[3].Kind != menu.Password {
			t.Fatal("localization altered or unmasked credential fields")
		}
		if got := clawDirection("in"); got != tt.received {
			t.Errorf("%s direction is %q", tt.tag, got)
		}
		for _, row := range rows {
			if strings.HasPrefix(row.Label, "openclaw.") || strings.HasPrefix(row.Hint, "openclaw.") {
				t.Errorf("%s unresolved message identifier", tt.tag)
			}
		}
		if got := clawResult("error:wss://private.example.test/?token=synthetic-secret"); strings.Contains(got, "private") || strings.Contains(got, "secret") {
			t.Fatal("diagnostics exposed raw error text")
		}
	}
}
