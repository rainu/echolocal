package openclaw

import (
	"context"
	"encoding/json"
	"github.com/gorilla/websocket"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func catalogAccess(t *testing.T) *NodeAccess {
	t.Helper()
	a := NewNodeAccess(filepath.Join(t.TempDir(), "grants"))
	for _, name := range []string{"echolocal.audio.test", "device.info"} {
		group := "audio"
		if name == "device.info" {
			group = "state"
		}
		if err := a.Register(NodeOperation{Command: name, Group: group, Description: "Synthetic test operation.", Parameters: map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}, Run: func(context.Context, json.RawMessage) (any, error) { return true, nil }}); err != nil {
			t.Fatal(err)
		}
	}
	return a
}
func TestNodeCatalogSnapshotsNamesAndRevocation(t *testing.T) {
	a := catalogAccess(t)
	tools := a.Tools()
	if len(tools) != 2 || tools[0].Command != "device.info" {
		t.Fatal(tools)
	}
	for _, tool := range tools {
		if !safeToolName.MatchString(tool.Name) || tool.PluginID != "echolocal" {
			t.Fatal(tool.Name)
		}
	}
	tools[0].Parameters["type"] = "string"
	if a.Tools()[0].Parameters["type"] != "object" {
		t.Fatal("snapshot aliases registry")
	}
	if err := a.SetGrants(map[string]bool{"audio": false}); err != nil {
		t.Fatal(err)
	}
	if len(a.Tools()) != 1 {
		t.Fatal("stale privileged descriptor")
	}
	if _, err := a.Execute(context.Background(), "echolocal.audio.test", nil); err == nil || err.Error() != "permission_denied" {
		t.Fatal(err)
	}
	if err := a.Register(NodeOperation{Command: "device_info", Group: "state", Description: "collision", Parameters: tools[1].Parameters, Run: func(context.Context, json.RawMessage) (any, error) { return nil, nil }}); err == nil {
		t.Fatal("tool name collision accepted")
	}
}
func TestNodeCatalogPublicationLifecycle(t *testing.T) {
	var sockets, publications, skills atomic.Int32
	var filtered atomic.Bool
	srv, endpoint := server(t, func(conn *websocket.Conn) {
		req := challenge(conn)
		if req == nil {
			return
		}
		sockets.Add(1)
		// Handshake still contains command strings only.
		params := req["params"].(map[string]any)
		if len(params["commands"].([]any)) != 2 {
			t.Error("wrong handshake command surface")
		}
		nodeHello(conn, req)
		for {
			var f struct {
				ID, Method string
				Params     json.RawMessage
			}
			if conn.ReadJSON(&f) != nil {
				return
			}
			var payload any = map[string]any{}
			switch f.Method {
			case "node.pluginTools.update":
				var p struct{ Tools []NodeTool }
				json.Unmarshal(f.Params, &p)
				publications.Add(1)
				if len(p.Tools) == 1 {
					filtered.Store(true)
				}
				payload = map[string]any{"tools": p.Tools}
			case "node.skills.update":
				var p struct{ Skills []NodeSkill }
				json.Unmarshal(f.Params, &p)
				if len(p.Skills) != 1 || p.Skills[0].Name != "echolocal-device-tools" {
					t.Error("bad skill")
				}
				skills.Add(1)
				payload = map[string]any{"skills": p.Skills}
			}
			if conn.WriteJSON(map[string]any{"type": "res", "id": f.ID, "ok": true, "payload": payload}) != nil {
				return
			}
		}
	})
	c := NewNode(filepath.Join(t.TempDir(), "node"), "test", NodeMetadata{})
	c.AttachNodeAccess(catalogAccess(t))
	c.dialer.TLSClientConfig = srv.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	if err := c.ConfigureBootstrap(endpoint, "synthetic"); err != nil {
		t.Fatal(err)
	}
	c.Connect()
	t.Cleanup(func() { _ = c.Close() })
	waitUntil := func(fn func() bool) {
		t.Helper()
		until := time.Now().Add(5 * time.Second)
		for !fn() && time.Now().Before(until) {
			time.Sleep(5 * time.Millisecond)
		}
		if !fn() {
			t.Fatal("publication not observed")
		}
	}
	waitUntil(func() bool {
		return c.NodePublication().Tools == "acknowledged" && c.NodePublication().Skill == "acknowledged"
	})
	if err := c.access.SetGrants(map[string]bool{"audio": false}); err != nil {
		t.Fatal(err)
	}
	waitUntil(filtered.Load)
	// Re-enabling through the client reconnects to refresh command ceiling.
	if err := c.SetNodeGrants(map[string]bool{"audio": true}); err != nil {
		t.Fatal(err)
	}
	waitUntil(func() bool {
		return sockets.Load() >= 2 && skills.Load() >= 3 && c.NodePublication().Tools == "acknowledged"
	})
	c.Disconnect()
	if c.NodePublication().Tools != "disconnected" {
		t.Fatal("stale connection status")
	}
	if publications.Load() < 3 {
		t.Fatal("missing replacement")
	}
}
func TestNodeCatalogRejectionDoesNotBreakCommands(t *testing.T) {
	for _, reject := range []bool{true, false} {
		t.Run(map[bool]string{true: "unsupported", false: "policy-disabled"}[reject], func(t *testing.T) {
			var count atomic.Int32
			var invoked atomic.Bool
			srv, endpoint := server(t, func(conn *websocket.Conn) {
				req := challenge(conn)
				if req == nil {
					return
				}
				nodeHello(conn, req)
				for {
					var f struct {
						ID, Method string
						Params     json.RawMessage
					}
					if conn.ReadJSON(&f) != nil {
						return
					}
					if f.Method == "node.invoke.result" {
						invoked.Store(true)
					}
					reply := map[string]any{"type": "res", "id": f.ID, "ok": true, "payload": map[string]any{"tools": []any{}, "skills": []any{}}}
					if strings.HasSuffix(f.Method, ".update") {
						count.Add(1)
						if reject {
							reply["ok"] = false
							reply["error"] = map[string]any{"code": "INVALID_REQUEST", "message": "synthetic unsupported"}
						}
					}
					if conn.WriteJSON(reply) != nil {
						return
					}
					if count.Load() == 2 && !invoked.Load() {
						raw, _ := json.Marshal(map[string]any{"id": "synthetic-invoke", "nodeId": "synthetic-node", "command": "device.info", "paramsJSON": "{}"})
						if conn.WriteJSON(map[string]any{"type": "event", "event": "node.invoke.request", "payload": json.RawMessage(raw)}) != nil {
							return
						}
					}
				}
			})
			c := NewNode(filepath.Join(t.TempDir(), "node"), "test", NodeMetadata{})
			c.AttachNodeAccess(catalogAccess(t))
			c.dialer.TLSClientConfig = srv.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
			c.ConfigureBootstrap(endpoint, "synthetic")
			c.Connect()
			t.Cleanup(func() { _ = c.Close() })
			until := time.Now().Add(5 * time.Second)
			for !invoked.Load() && time.Now().Before(until) {
				time.Sleep(5 * time.Millisecond)
			}
			if !invoked.Load() || c.Status().State != "connected" || count.Load() != 2 {
				t.Fatal("publication broke normal node commands", count.Load(), c.Status())
			}
		})
	}
}
func TestNodeSchemaDiagnosticsNeverEchoInputs(t *testing.T) {
	a := catalogAccess(t)
	for _, raw := range []string{`{"private-user-key":"private-secret"}`, `null`, `[]`} {
		_, err := a.Execute(context.Background(), "device.info", []byte(raw))
		if err == nil || !strings.HasPrefix(err.Error(), "invalid_params:") || strings.Contains(err.Error(), "private-") {
			t.Fatal("unsafe validation", err)
		}
	}
}

func TestNodeCatalogPendingPublicationCancelledAndReconnected(t *testing.T) {
	var connections atomic.Int32
	var firstPending atomic.Bool
	srv, endpoint := server(t, func(conn *websocket.Conn) {
		req := challenge(conn)
		if req == nil {
			return
		}
		n := connections.Add(1)
		nodeHello(conn, req)
		for {
			var f struct {
				ID, Method string
				Params     json.RawMessage
			}
			if conn.ReadJSON(&f) != nil {
				return
			}
			if n == 1 && f.Method == "node.pluginTools.update" {
				firstPending.Store(true)
				continue
			}
			var payload map[string]any
			if json.Unmarshal(f.Params, &payload) != nil {
				return
			}
			if conn.WriteJSON(map[string]any{"type": "res", "id": f.ID, "ok": true, "payload": payload}) != nil {
				return
			}
		}
	})
	c := NewNode(filepath.Join(t.TempDir(), "node"), "test", NodeMetadata{})
	c.AttachNodeAccess(catalogAccess(t))
	c.dialer.TLSClientConfig = srv.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	c.ConfigureBootstrap(endpoint, "synthetic")
	c.Connect()
	t.Cleanup(func() { _ = c.Close() })
	until := time.Now().Add(3 * time.Second)
	for !firstPending.Load() && time.Now().Before(until) {
		time.Sleep(5 * time.Millisecond)
	}
	if !firstPending.Load() {
		t.Fatal("missing pending publication")
	}
	at := time.Now()
	if err := c.Disconnect(); err != nil {
		t.Fatal(err)
	}
	if time.Since(at) > time.Second {
		t.Fatal("pending publisher blocked disconnect")
	}
	if err := c.Connect(); err != nil {
		t.Fatal(err)
	}
	until = time.Now().Add(3 * time.Second)
	for c.NodePublication().Skill != "acknowledged" && time.Now().Before(until) {
		time.Sleep(5 * time.Millisecond)
	}
	if connections.Load() != 2 || c.NodePublication().Tools != "acknowledged" || c.NodePublication().Skill != "acknowledged" {
		t.Fatal("new socket did not own publication", c.NodePublication())
	}
}
func TestNodeCatalogRejectsInvalidMetadataAndCopiesSchema(t *testing.T) {
	a := NewNodeAccess(filepath.Join(t.TempDir(), "permissions"))
	schema := map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}
	op := NodeOperation{Command: "test.valid", Group: "state", Description: "Synthetic metadata", Parameters: schema, Run: func(context.Context, json.RawMessage) (any, error) { return nil, nil }}
	if err := a.Register(op); err != nil {
		t.Fatal(err)
	}
	schema["type"] = "string"
	if a.Tools()[0].Parameters["type"] != "object" {
		t.Fatal("registration retained mutable schema")
	}
	for _, name := range []string{"invalid name", strings.Repeat("x", 70)} {
		op.Command = name
		if a.Register(op) == nil {
			t.Fatal("unsafe tool name admitted")
		}
	}
	op.Command = "test.large"
	op.Parameters = map[string]any{"description": strings.Repeat("x", 17000)}
	if a.Register(op) == nil {
		t.Fatal("unbounded schema admitted")
	}
}

func TestNodeCatalogPayloadBound(t *testing.T) {
	tools := catalogAccess(t).Tools()
	if _, large := boundedNodeToolPayload(tools); large {
		t.Fatal("small catalog rejected")
	}
	for _, list := range [][]NodeTool{make([]NodeTool, 129), {{Description: strings.Repeat("x", 96*1024)}}} {
		payload, large := boundedNodeToolPayload(list)
		if !large || len(payload["tools"].([]NodeTool)) != 0 {
			t.Fatal("oversized replacement not withdrawn")
		}
	}
}

func TestNodePublishedSkillMatchesGatewayFrontmatter(t *testing.T) {
	skill := NodeUsageSkill()
	if skill.Name == "" || skill.Description == "" || len(skill.Description) > 1024 || len(skill.Content) > 65536 {
		t.Fatal("invalid skill bounds")
	}
	if !strings.Contains(skill.Content, "\nname: "+skill.Name+"\n") || !strings.Contains(skill.Content, "\ndescription: "+skill.Description+"\n") {
		t.Fatal("Gateway drops descriptors whose frontmatter does not match exactly")
	}
}
