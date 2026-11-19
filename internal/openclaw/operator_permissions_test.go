package openclaw

import (
	"github.com/gorilla/websocket"
	"os"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func TestOperatorDefaultsMigrationAndPersistence(t *testing.T) {
	c := New(t.TempDir()+"/operator.json", "test")
	if !reflect.DeepEqual(c.RequestedOperatorScopes(), SupportedOperatorScopes()) {
		t.Fatal("fresh default")
	}
	if err := c.Configure("wss://synthetic.example", "synthetic"); err != nil {
		t.Fatal(err)
	}
	key := append([]byte{}, c.saved.Private...)
	c.saved.Talk = true
	if err := c.persist(c.saved); err != nil {
		t.Fatal(err)
	}
	c = New(c.path, "test")
	if !reflect.DeepEqual(c.RequestedOperatorScopes(), SupportedOperatorScopes()) {
		t.Fatal("legacy migration")
	}
	for _, scopes := range [][]string{{"operator.read", "operator.talk"}, {}} {
		if err := c.SetOperatorScopes(scopes); err != nil {
			t.Fatal(err)
		}
		c = New(c.path, "test")
		if !reflect.DeepEqual(c.RequestedOperatorScopes(), scopes) {
			t.Fatal("selection lost")
		}
		if !reflect.DeepEqual(c.saved.Private, key) || c.saved.Bootstrap != "synthetic" || !c.saved.Talk {
			t.Fatal("identity/auth/talk changed")
		}
		if got := connectParams(c.saved, "nonce", 123, "test")["scopes"]; !reflect.DeepEqual(got, scopes) {
			t.Fatal("handshake mismatch", got)
		}
	}
	if err := c.SetOperatorScopes([]string{"operator.invented"}); err == nil {
		t.Fatal("accepted unknown")
	}
	b, _ := os.ReadFile(c.path)
	if len(b) == 0 {
		t.Fatal("not persisted")
	}
}
func TestOperatorPermissionsRoleSeparation(t *testing.T) {
	node := NewNode(t.TempDir()+"/node.json", "test", NodeMetadata{})
	if node.SetOperatorScopes(SupportedOperatorScopes()) == nil {
		t.Fatal("node accepted scopes")
	}
	if len(node.RequestedOperatorScopes()) != 0 {
		t.Fatal("node inherited authority")
	}
}
func TestOperatorReauthenticationAndActiveGrants(t *testing.T) {
	srv, endpoint := server(t, func(conn *websocket.Conn) {
		req := challenge(conn)
		scopes := req["params"].(map[string]any)["scopes"]
		conn.WriteJSON(map[string]any{"type": "res", "id": req["id"], "ok": true, "payload": map[string]any{"type": "hello-ok", "protocol": Protocol, "auth": map[string]any{"role": "operator", "scopes": scopes, "deviceToken": "synthetic-device-token"}}})
		var ignored any
		conn.ReadJSON(&ignored)
	})
	c := client(t, srv, endpoint)
	defer c.Disconnect()
	if err := c.Connect(); err != nil {
		t.Fatal(err)
	}
	waitState(t, c, "connected")
	key := append([]byte{}, c.saved.Private...)
	if !reflect.DeepEqual(c.GrantedOperatorScopes(), SupportedOperatorScopes()) {
		t.Fatal("grants missing")
	}
	restricted := []string{"operator.read", "operator.talk"}
	if err := c.SetOperatorScopes(restricted); err != nil {
		t.Fatal(err)
	}
	waitState(t, c, "connected")
	if !reflect.DeepEqual(c.GrantedOperatorScopes(), restricted) {
		t.Fatal("stale grants")
	}
	if !reflect.DeepEqual(key, c.saved.Private) || c.saved.Token != "synthetic-device-token" {
		t.Fatal("identity lost")
	}
	c.Disconnect()
	if len(c.GrantedOperatorScopes()) != 0 {
		t.Fatal("disconnected grants")
	}
}

func TestOperatorUpgradePendingApprovalAndRetry(t *testing.T) {
	var approved atomic.Bool
	srv, endpoint := server(t, func(conn *websocket.Conn) {
		req := challenge(conn)
		if !approved.Load() {
			conn.WriteJSON(map[string]any{"type": "res", "id": req["id"], "ok": false, "error": map[string]any{"code": "NOT_PAIRED", "details": map[string]any{"code": "PAIRING_REQUIRED"}}})
			return
		}
		hello(conn, req)
		var ignored any
		conn.ReadJSON(&ignored)
	})
	c := client(t, srv, endpoint)
	defer c.Disconnect()
	key := append([]byte{}, c.saved.Private...)
	if err := c.Connect(); err != nil {
		t.Fatal(err)
	}
	waitState(t, c, "pairing")
	if len(c.GrantedOperatorScopes()) != 0 {
		t.Fatal("requested authority shown as granted")
	}
	// Approval is performed only by the test fixture, never by the client.
	approved.Store(true)
	if err := c.Connect(); err != nil {
		t.Fatal(err)
	}
	waitState(t, c, "connected")
	if !reflect.DeepEqual(key, c.saved.Private) {
		t.Fatal("pairing reset identity")
	}
}
func TestOperatorScopeChangeDoesNotEnableTalk(t *testing.T) {
	c := New(t.TempDir()+"/operator.json", "test")
	if err := c.SetOperatorScopes([]string{"operator.read"}); err != nil {
		t.Fatal(err)
	}
	if err := c.EnableTalk(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.RequestedOperatorScopes(), []string{"operator.read"}) {
		t.Fatal("Talk widened explicit scopes")
	}
}

func TestOperatorLiveScopeUpgradeProtocol(t *testing.T) {
	for _, decision := range []string{"approved", "rejected", "expired"} {
		t.Run(decision, func(t *testing.T) {
			var attempts atomic.Int32
			srv, endpoint := server(t, func(conn *websocket.Conn) {
				req := challenge(conn)
				p := req["params"].(map[string]any)
				n := attempts.Add(1)
				if n == 1 {
					conn.WriteJSON(map[string]any{"type": "res", "id": req["id"], "ok": false, "error": map[string]any{"details": map[string]any{"code": "AUTH_DEVICE_TOKEN_MISMATCH"}}})
					return
				}
				if n == 2 {
					if len(p["scopes"].([]any)) != 0 {
						t.Error("upgrade fallback retained authority")
					}
					conn.WriteJSON(map[string]any{"type": "res", "id": req["id"], "ok": true, "payload": map[string]any{"type": "hello-ok", "protocol": Protocol, "auth": map[string]any{"role": "operator", "scopes": []string{}}}})
					var call map[string]any
					conn.ReadJSON(&call)
					if call["method"] != "device.scopes.requestUpgrade" {
						t.Error("wrong upgrade method")
					}
					conn.WriteJSON(map[string]any{"type": "res", "id": call["id"], "ok": true, "payload": map[string]any{"requestId": "synthetic-upgrade"}})
					conn.ReadJSON(&call)
					if call["method"] != "device.scopes.waitUpgrade" {
						t.Error("wrong wait method")
					}
					conn.WriteJSON(map[string]any{"type": "res", "id": call["id"], "ok": true, "payload": map[string]any{"status": decision, "deviceToken": "synthetic-upgraded-token", "scopes": SupportedOperatorScopes()}})
					var ignored any
					conn.ReadJSON(&ignored)
					return
				}
				if p["auth"].(map[string]any)["token"] != "synthetic-upgraded-token" || len(p["scopes"].([]any)) != len(SupportedOperatorScopes()) {
					t.Error("approved handshake wrong")
				}
				hello(conn, req)
				var ignored any
				conn.ReadJSON(&ignored)
			})
			c := client(t, srv, endpoint)
			defer c.Disconnect()
			s := c.saved
			s.Token = "synthetic-device-token"
			s.Bootstrap = ""
			if err := c.persist(s); err != nil {
				t.Fatal(err)
			}
			key := append([]byte{}, s.Private...)
			if err := c.Connect(); err != nil {
				t.Fatal(err)
			}
			if decision == "approved" {
				waitState(t, c, "connected")
				// connected may briefly precede the pending state on the zero-scope socket.
				deadline := time.Now().Add(5 * time.Second)
				for attempts.Load() < 3 && time.Now().Before(deadline) {
					time.Sleep(5 * time.Millisecond)
				}
				if attempts.Load() != 3 {
					t.Fatal("did not reconnect with approved token")
				}
			} else {
				waitState(t, c, "error")
				if c.Status().Reason != "scope_upgrade_"+decision {
					t.Fatal(c.Status())
				}
				if len(c.GrantedOperatorScopes()) != 0 {
					t.Fatal("rejection left active grants")
				}
			}
			if !reflect.DeepEqual(key, c.saved.Private) || !reflect.DeepEqual(c.RequestedOperatorScopes(), SupportedOperatorScopes()) {
				t.Fatal("upgrade reset identity or selection")
			}
		})
	}
}
