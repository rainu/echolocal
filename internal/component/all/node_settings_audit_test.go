package all

import (
	"encoding/json"
	"github.com/ygelfand/echolocal/internal/board"
	"github.com/ygelfand/echolocal/internal/component"
	"github.com/ygelfand/echolocal/internal/feature/control"
	setting "github.com/ygelfand/libcountertop/pkg/settings"
	"os"
	"testing"
)

// Run in a fresh process per board; feature registries contain OnceValues.
func TestNodeSettingsCoverageAudit(t *testing.T) {
	target := os.Getenv("NODE_SETTINGS_AUDIT")
	if target == "" {
		t.Skip("explicit isolated all-board metadata audit")
	}
	boards := map[string]board.Board{"athena": board.Athena, "checkers": board.Checkers, "cronos": board.Cronos, "crown": board.Crown, "biscuit": board.Biscuit, "donut": board.Donut, "radar": board.Radar, "rook": board.Rook}
	b, ok := boards[target]
	if !ok {
		t.Fatal("unknown board")
	}
	component.Default().Use(b)
	_ = component.Default().Actions()
	type row struct {
		Domain, Name, ID, Kind, Group, Excluded string
		Available                               bool
	}
	rows := []row{}
	seen := map[string]bool{}
	for _, bound := range component.Settings.All() {
		d, ok := bound.(setting.Inspectable)
		if !ok {
			t.Fatalf("uninspectable bound: %s", bound.Domain())
		}
		for _, name := range bound.Names() {
			key := bound.Domain() + "/" + name
			if seen[key] {
				t.Errorf("duplicate bound identifier: %s", key)
			}
			seen[key] = true
			group, reason := control.NodeSettingAudit(bound.Domain(), name)
			if reason == "UNREVIEWED" {
				t.Errorf("unreviewed bound: %s/%s", bound.Domain(), name)
				continue
			}
			// Never inspect private fields or invoke their dynamic choice providers.
			r := row{Domain: bound.Domain(), Name: name, Group: group, Excluded: reason, Available: control.NodeSettingAvailable(bound.Domain(), name)}
			if reason == "" {
				f, ok := d.Metadata(name)
				if !ok {
					t.Fatal("missing metadata")
				}
				r.ID = f.ID
				r.Kind = f.Kind
				if f.Kind == "secret" || f.Kind == "credential" {
					t.Error("reviewed field became sensitive")
				}
			}
			rows = append(rows, r)
		}
	}
	if p := os.Getenv("NODE_SETTINGS_AUDIT_OUT"); p != "" {
		raw, _ := json.MarshalIndent(rows, "", "  ")
		if err := os.WriteFile(p, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if len(rows) < 20 {
		t.Fatal("registry was not initialized")
	}
}
