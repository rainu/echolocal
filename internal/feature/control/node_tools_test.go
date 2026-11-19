//go:build !board_biscuit

package control

import (
	"context"
	"encoding/json"
	connection "github.com/ygelfand/echolocal/internal/openclaw"
	setting "github.com/ygelfand/libcountertop/pkg/settings"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestNodePayloadAndAuthorityBoundaries(t *testing.T) {
	for _, raw := range []string{`{"unknown":true}`, `{}{}`, `null`} {
		var p struct {
			Args []string `json:"args"`
		}
		if raw == "null" {
			continue
		}
		if decodeNode([]byte(raw), &p) == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	for _, d := range []string{"openclaw", "openclaw-node", "frontend", "network", "certs", "cast"} {
		if nodeDomain(d) {
			t.Fatal("authority domain admitted", d)
		}
	}
	for _, k := range []string{"secret", "credential", "text", "list"} {
		if nodeField(setting.Field{Kind: k}) {
			t.Fatal("unreviewed field admitted", k)
		}
	}
	if nodeActionGroup("call_offer") != "" || nodeActionGroup("logs") != "" || nodeActionGroup("arbitrary_new_action") != "" {
		t.Fatal("unreviewed action admitted")
	}
}
func TestNodeAdaptersDenyEveryAdvertisedCommandAfterAllOff(t *testing.T) {
	if os.Getenv("NODE_ADAPTER_TEST") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestNodeAdaptersDenyEveryAdvertisedCommandAfterAllOff$")
		cmd.Env = append(os.Environ(), "NODE_ADAPTER_TEST=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("isolated adapters: %s", out)
		}
		return
	}
	a := connection.NewNodeAccess(filepath.Join(t.TempDir(), "permissions"))
	registerNodeTools(a)
	commands := a.Commands()
	if len(commands) < 15 {
		t.Fatal("missing registrations", commands)
	}
	grants := map[string]bool{}
	for _, g := range connection.PermissionGroups {
		grants[g] = false
	}
	if err := a.SetGrants(grants); err != nil {
		t.Fatal(err)
	}
	if len(a.Commands()) != 0 {
		t.Fatal("denied commands advertised")
	}
	for _, name := range commands {
		if _, err := a.Execute(context.Background(), name, json.RawMessage("{}")); err == nil || err.Error() != "permission_denied" {
			t.Fatal("denial bypass", name, err)
		}
	}
	for _, payload := range []string{`{"values":{"openclaw":{"permissions":"on"}}}`, `{"values":{"frontend":{"password":"synthetic"}}}`} {
		if err := a.SetGrants(map[string]bool{"settings": true}); err != nil {
			t.Fatal(err)
		}
		if _, err := a.Execute(context.Background(), "echolocal.settings.import", []byte(payload)); err == nil {
			t.Fatal("import bypass")
		}
	}
}
