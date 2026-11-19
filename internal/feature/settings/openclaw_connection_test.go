//go:build !board_biscuit

package settings

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ygelfand/echolocal/internal/openclaw"
	"github.com/ygelfand/libcountertop/pkg/display/menu"
	sharedshell "github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/say"
)

func TestCombinedConnectionMenuHasOneSecretFieldAndBothStatuses(t *testing.T) {
	dir := t.TempDir()
	ui := openclaw.New(filepath.Join(dir, "ui"), "test")
	node := openclaw.NewNode(filepath.Join(dir, "node"), "test", openclaw.NodeMetadata{})
	g := openclaw.NewConnections(filepath.Join(dir, "shared"), ui, node, "Echo UI", "Echo Node")
	cfg := g.Editable()
	cfg.URL = "wss://synthetic.example"
	cfg.Token = "synthetic-common"
	if g.Save(cfg) != nil {
		t.Fatal("save")
	}
	before := say.Chosen()
	defer say.Use(before)
	for _, lang := range []string{"de", "en", "it", "vi"} {
		say.Use(lang)
		page := openclawConnectionPage(g)
		rows, acts := page.Build()
		if len(rows) != 9 || len(acts) != 9 {
			t.Fatal("combined form missing rows")
		}
		if rows[0].Value != clawState("disconnected") || rows[1].Value != clawState("disconnected") {
			t.Fatal("both connection states not shown")
		}
		passwords := 0
		for _, r := range rows {
			if r.Kind == menu.Password {
				passwords++
			}
			if strings.HasPrefix(r.Label, "openclaw.") || r.Label == "" {
				t.Fatal("missing label", lang)
			}
		}
		if passwords != 1 || rows[2].Value != cfg.URL || rows[3].Value != cfg.Token {
			t.Fatal("duplicated or unmasked configuration")
		}
		if rows[4].Value != "Echo UI" || rows[5].Value != "Echo Node" || rows[6].Open == nil {
			t.Fatal("names or permissions missing")
		}
		if rows[7].Open == nil || rows[8].Open == nil {
			t.Fatal("separate diagnostics missing")
		}
		for _, index := range []int{3, 4, 5} {
			if rows[index].Hint != "" {
				t.Fatal("unnecessary configuration explanation", lang)
			}
		}
		for _, index := range []int{7, 8} {
			diag := rows[index].Open().(*sharedshell.Page)
			if diag.Title != rows[index].Label {
				t.Fatal("ambiguous diagnostics title", lang)
			}
		}
		permissions := rows[6].Open().(*sharedshell.Page)
		if permissions.Title != connectionText("permissions") {
			t.Fatal("technical permissions title", lang)
		}
		if page.Actions[len(page.Actions)-1].Label != say.T("openclaw.action.save") {
			t.Fatal("missing shared save")
		}
	}
}
func TestCombinedConnectionNameEditUsesSingleSave(t *testing.T) {
	dir := t.TempDir()
	ui := openclaw.New(filepath.Join(dir, "ui"), "test")
	node := openclaw.NewNode(filepath.Join(dir, "node"), "test", openclaw.NodeMetadata{})
	g := openclaw.NewConnections(filepath.Join(dir, "shared"), ui, node, "Echo UI", "Echo Node")
	cfg := g.Editable()
	cfg.URL = "wss://synthetic.example"
	cfg.Token = "synthetic-common"
	if g.Save(cfg) != nil {
		t.Fatal("save")
	}
	page := openclawConnectionPage(g)
	rows, _ := page.Build()
	rows[4].Save("Kitchen UI")
	rows[5].Save("Kitchen Node")
	page.Actions[len(page.Actions)-1].Do()
	if g.Editable().UIName != "Kitchen UI" || g.Editable().NodeName != "Kitchen Node" {
		t.Fatal("names not saved together")
	}
	rows, _ = page.Build()
	if rows[3].Kind != menu.Password || rows[3].Value != "synthetic-common" {
		t.Fatal("name editing altered credential")
	}
}
func TestConnectionLocalLabelsCoverEverySupportedLanguage(t *testing.T) {
	before := say.Chosen()
	defer say.Use(before)
	for _, lang := range say.Languages() {
		say.Use(lang)
		for _, key := range []string{"ui", "node", "uiName", "nodeName", "permissions", "permissionsHint", "diagUI", "diagNode", "forget", "forgetHint", "invalidName"} {
			if connectionText(key) == "" {
				t.Fatal("missing localized connection label", lang, key)
			}
		}
	}
}
