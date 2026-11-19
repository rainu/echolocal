//go:build !board_biscuit

package all

import (
	"fmt"
	"github.com/ygelfand/echolocal/internal/board"
	"github.com/ygelfand/echolocal/internal/component"
	setting "github.com/ygelfand/libcountertop/pkg/settings"
	"os"
	"strings"
	"testing"
)

func TestNodeRegistryInventory(t *testing.T) {
	if os.Getenv("ECHLOCAL_NODE_INVENTORY") != "1" {
		t.Skip("explicit metadata inventory")
	}
	was := component.Board()
	defer component.Default().Use(was)
	component.Default().Use(board.Athena)
	var b strings.Builder
	b.WriteString("# Node registry inventory\n\nMetadata only; no setting values or live state.\n\n## Actions\n\n|Action|Arguments|\n|---|---|\n")
	for _, a := range component.Default().Actions() {
		var args []string
		for _, v := range a.Args {
			args = append(args, v.Name)
		}
		fmt.Fprintf(&b, "|%s|%s|\n", a.Name, strings.Join(args, ", "))
	}
	b.WriteString("\n## Settings\n\n|Domain|Field|Kind|\n|---|---|---|\n")
	for _, v := range component.Settings.All() {
		if d, ok := v.(setting.Described); ok {
			for _, f := range d.Fields() {
				fmt.Fprintf(&b, "|%s|%s|%s|\n", v.Domain(), f.Name, f.Kind)
			}
		}
	}
	b.WriteString("\n## Entities\n\n|Object ID|Type|\n|---|---|\n")
	for _, e := range component.Default().Entities() {
		fmt.Fprintf(&b, "|%s|%T|\n", objectID(e), e)
	}
	if e := os.WriteFile("../../../docs/openclaw-node-registry.md", []byte(b.String()), 0644); e != nil {
		t.Fatal(e)
	}
}
