//go:build !board_biscuit

package control

import (
	"context"
	"encoding/json"
	connection "github.com/ygelfand/echolocal/internal/openclaw"
	esphome "github.com/ygelfand/go-esphome-device"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNodeCatalogRegistryCoverage(t *testing.T) {
	if !catalogIsolated(t) {
		return
	}
	a := connection.NewNodeAccess(filepath.Join(t.TempDir(), "permissions"))
	registerNodeTools(a)
	tools := a.Tools()
	if len(tools) != len(a.Commands()) || len(tools) < 35 {
		t.Fatal("undocumented command")
	}
	seen := map[string]bool{}
	for _, tool := range tools {
		if seen[tool.Name] || tool.Description == "" || tool.Parameters["type"] != "object" {
			t.Fatal("invalid descriptor", tool.Command)
		}
		seen[tool.Name] = true
	}
	defer func() {
		if recover() == nil {
			t.Error("undocumented registry addition silently accepted")
		}
	}()
	_ = nodeRegister(a, connection.NodeOperation{Command: "future.undocumented", Group: "state", Run: func(context.Context, json.RawMessage) (any, error) { return nil, nil }})
}
func TestNodeActionCatalogAndOriginalYouTubeMistake(t *testing.T) {
	a := connection.NewNodeAccess(filepath.Join(t.TempDir(), "permissions"))
	calls := 0
	action := &esphome.Action{Name: "youtube_tv", Args: []esphome.Arg{{Name: "operation", Type: esphome.ArgString}, {Name: "channel", Type: esphome.ArgString}, {Name: "device", Type: esphome.ArgString}}}
	description, parameters := nodeActionContract(action)
	if err := nodeRegister(a, connection.NodeOperation{Command: "action.youtube_tv", Group: "audio", Description: description, Parameters: parameters, Run: func(context.Context, json.RawMessage) (any, error) { calls++; return true, nil }}); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`{"url":"private-user-url"}`, `{"args":{"url":"private-user-url"}}`, `{"args":{"channel":7}}`, `{"args":[]}`, `{"args":{"operation":"open_url"}}`} {
		_, err := a.Execute(context.Background(), "echolocal.action.youtube_tv", []byte(raw))
		if err == nil || !strings.HasPrefix(err.Error(), "invalid_params:") || strings.Contains(err.Error(), "private-user") {
			t.Fatal(raw, err)
		}
	}
	if calls != 0 {
		t.Fatal("invalid call performed action")
	}
	for _, raw := range []string{`{"args":{"operation":"watch","channel":"ESPN","device":""}}`, `{"args":{"operation":"whats_on","channel":"ESPN"}}`} {
		if _, err := a.Execute(context.Background(), "echolocal.action.youtube_tv", []byte(raw)); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 2 {
		t.Fatal(calls)
	}
}
func TestNodePublishedExamplesMatchContracts(t *testing.T) {
	if !catalogIsolated(t) {
		return
	}
	// Every execution below uses no-effect stubs retaining the real metadata.
	a := connection.NewNodeAccess(filepath.Join(t.TempDir(), "permissions"))
	registerNodeTools(a)
	tools := map[string]connection.NodeTool{}
	for _, tool := range a.Tools() {
		tools[tool.Name] = tool
	}
	n := 0
	// Replace actual Run handlers with no-effect stubs while retaining metadata.
	checks := connection.NewNodeAccess(filepath.Join(t.TempDir(), "checks"))
	for _, tool := range tools {
		if err := checks.Register(connection.NodeOperation{Command: tool.Command, Group: "state", Description: tool.Description, Parameters: tool.Parameters, Run: func(context.Context, json.RawMessage) (any, error) { return nil, nil }}); err != nil {
			t.Fatal(err)
		}
	}
	for _, line := range strings.Split(connection.NodeUsageSkill().Content, "\n") {
		if !strings.HasPrefix(line, "- echolocal_") {
			continue
		}
		name, raw, ok := strings.Cut(strings.TrimPrefix(line, "- "), ": ")
		tool, found := tools[name]
		if !ok || !found {
			t.Fatal("example has no descriptor", name)
		}
		if _, err := checks.Execute(context.Background(), tool.Command, []byte(raw)); err != nil {
			t.Fatal("invalid published example", name, err)
		}
		n++
	}
	if n < 10 {
		t.Fatal("missing guidance examples")
	}
}

// Registry setup initializes feature singletons; isolate it from unrelated
// control-package tests just like the existing full adapter permission test.
func catalogIsolated(t *testing.T) bool {
	t.Helper()
	if os.Getenv("NODE_CATALOG_TEST") == t.Name() {
		return true
	}
	cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$")
	cmd.Env = append(os.Environ(), "NODE_CATALOG_TEST="+t.Name())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("isolated catalog: %s", out)
	}
	return false
}
