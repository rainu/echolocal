package openclaw

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNodeAccessEveryGroup(t *testing.T) {
	p := filepath.Join(t.TempDir(), "permissions.json")
	a := NewNodeAccess(p)
	for _, g := range PermissionGroups {
		if g == "desktop" {
			if a.Granted(g) {
				t.Fatal("desktop default must be off")
			}
			if err := a.SetGrants(map[string]bool{g: true}); err != nil {
				t.Fatal(err)
			}
		}
		if !a.Granted(g) {
			t.Fatal("default", g)
		}
		calls := 0
		if err := a.Register(NodeOperation{Command: g, Group: g, Run: func(context.Context, json.RawMessage) (any, error) { calls++; return true, nil }}); err != nil {
			t.Fatal(err)
		}
		if _, err := a.Execute(context.Background(), g, nil); err != nil {
			t.Fatal(err)
		}
		if err := a.SetGrants(map[string]bool{g: false}); err != nil {
			t.Fatal(err)
		}
		if _, err := a.Execute(context.Background(), g, nil); err == nil || calls != 1 {
			t.Fatal("bypass", g)
		}
		if NewNodeAccess(p).Granted(g) {
			t.Fatal("denial lost", g)
		}
	}
	if len(a.Commands()) != 0 {
		t.Fatal("advertised denied command")
	}
}
func TestNodeAccessUpgrade(t *testing.T) {
	p := filepath.Join(t.TempDir(), "permissions.json")
	os.WriteFile(p, []byte(`{"input":false}`), 0600)
	a := NewNodeAccess(p)
	if a.Granted("input") || !a.Granted("camera") {
		t.Fatal("defaults")
	}
	os.WriteFile(p, []byte(`bad`), 0600)
	if NewNodeAccess(p).Granted("camera") {
		t.Fatal("corruption grants")
	}
}
func TestNodeAccessCancel(t *testing.T) {
	for _, disconnect := range []bool{false, true} {
		a := NewNodeAccess(filepath.Join(t.TempDir(), "permissions.json"))
		started := make(chan struct{})
		done := make(chan error)
		a.Register(NodeOperation{Command: "input.tap", Group: "input", Run: func(ctx context.Context, _ json.RawMessage) (any, error) {
			close(started)
			<-ctx.Done()
			return "stale", nil
		}})
		go func() { _, err := a.Execute(context.Background(), "input.tap", nil); done <- err }()
		<-started
		if disconnect {
			a.Disconnect()
		} else {
			if err := a.SetGrants(map[string]bool{"input": false}); err != nil {
				t.Fatal(err)
			}
		}
		if <-done == nil {
			t.Fatal("stale result")
		}
	}
}
func TestNodeAccessInvalid(t *testing.T) {
	a := NewNodeAccess(filepath.Join(t.TempDir(), "permissions.json"))
	if a.SetGrants(map[string]bool{"shell": true}) == nil {
		t.Fatal("unknown group")
	}
	if a.Register(NodeOperation{}) == nil {
		t.Fatal("invalid operation")
	}
	if _, err := a.Execute(context.Background(), "x", json.RawMessage(`{`)); err == nil {
		t.Fatal("invalid json")
	}
}
