package openclaw

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestNodeSignedLeastPrivilegeAndIsolation(t *testing.T) {
	c := NewNode(filepath.Join(t.TempDir(), "node.json"), "node-test", NodeMetadata{"EchoLocal Test", "Athena"})
	if err := c.ConfigureBootstrap("wss://example.test", "synthetic-node-bootstrap"); err != nil {
		t.Fatal(err)
	}
	if c.EnableTalk() == nil {
		t.Fatal("Node elevated to Talk")
	}
	p := c.connectParams(c.saved, "nonce", 123)
	meta := p["client"].(map[string]any)
	if p["role"] != "node" || meta["mode"] != "node" || meta["id"] != "openclaw-android" || meta["modelIdentifier"] != "Athena" || meta["version"] != "node-test" {
		t.Fatal("incorrect metadata")
	}
	for _, k := range []string{"scopes", "caps", "commands"} {
		if len(p[k].([]string)) != 0 {
			t.Fatal("excess authority", k)
		}
	}
	d := p["device"].(map[string]any)
	pub, _ := base64.RawURLEncoding.DecodeString(d["publicKey"].(string))
	sig, _ := base64.RawURLEncoding.DecodeString(d["signature"].(string))
	payload := strings.Join([]string{"v3", d["id"].(string), "openclaw-android", "node", "node", "", "123", "synthetic-node-bootstrap", "nonce", "android", "tablet"}, "|")
	if !ed25519.Verify(pub, []byte(payload), sig) {
		t.Fatal("invalid role signature")
	}
	if p["auth"].(map[string]any)["bootstrapToken"] != "synthetic-node-bootstrap" {
		t.Fatal("bootstrap role auth")
	}
	op := New(filepath.Join(t.TempDir(), "operator.json"), "test")
	op.Configure("wss://example.test", "synthetic-operator")
	if string(op.saved.Private) == string(c.saved.Private) {
		t.Fatal("identity reused")
	}
	if _, e := c.BindTalk(); e == nil {
		t.Fatal("Node acquired Talk binding")
	}
	b, _ := os.ReadFile(c.path)
	if strings.Contains(string(b), "synthetic-operator") {
		t.Fatal("operator credential copied")
	}
}

func TestNodePairReconnectRestartAndDisconnect(t *testing.T) {
	var count atomic.Int32
	var original atomic.Value
	var valid atomic.Bool
	valid.Store(true)
	srv, endpoint := server(t, func(conn *websocket.Conn) {
		req := challenge(conn)
		if req == nil {
			return
		}
		p := req["params"].(map[string]any)
		n := count.Add(1)
		id := p["device"].(map[string]any)["id"]
		if n == 1 {
			original.Store(id)
		}
		if id != original.Load() || p["role"] != "node" || len(p["commands"].([]any)) != 0 || len(p["caps"].([]any)) != 0 {
			valid.Store(false)
		}
		if n > 1 && p["auth"].(map[string]any)["token"] != "synthetic-device-token" {
			valid.Store(false)
		}
		nodeHello(conn, req)
		if n == 1 {
			return
		} // transport loss, not an explicit disconnect
		var f any
		conn.ReadJSON(&f)
	})
	path := filepath.Join(t.TempDir(), "node.json")
	c := NewNode(path, "test", NodeMetadata{"EchoLocal Test", "Athena"})
	c.dialer.TLSClientConfig = srv.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	if err := c.ConfigureBootstrap(endpoint, "synthetic-bootstrap"); err != nil {
		t.Fatal(err)
	}
	c.Connect()
	t.Cleanup(func() { c.Close() })
	until := time.Now().Add(5 * time.Second)
	for count.Load() < 2 && time.Now().Before(until) {
		time.Sleep(10 * time.Millisecond)
	}
	waitState(t, c, "connected")
	if count.Load() < 2 || !valid.Load() {
		t.Fatal("network reconnect failed")
	}
	c.Close()
	restored := NewNode(path, "test", NodeMetadata{"EchoLocal Test", "Athena"})
	restored.dialer.TLSClientConfig = c.dialer.TLSClientConfig.Clone()
	t.Cleanup(func() { restored.Close() })
	restored.Start()
	waitState(t, restored, "connected")
	if !valid.Load() {
		t.Fatal("restart identity/token changed")
	}
	restored.Disconnect()
	disabled := NewNode(path, "test", NodeMetadata{})
	disabled.Start()
	time.Sleep(1100 * time.Millisecond)
	if disabled.Status().Enabled || disabled.Status().State != "disconnected" {
		t.Fatal("manual disconnect resumed")
	}
	var saved credentials
	b, _ := os.ReadFile(path)
	json.Unmarshal(b, &saved)
	if saved.Bootstrap != "" || saved.BootstrapOnly || saved.Talk {
		t.Fatal("bootstrap or Talk authority retained")
	}
	b, _ = json.Marshal(restored.Events())
	if strings.Contains(string(b), "synthetic") || strings.Contains(string(b), endpoint) {
		t.Fatal("diagnostic leak")
	}
}

func TestNodePairingRequiresExplicitRetry(t *testing.T) {
	srv, endpoint := server(t, func(conn *websocket.Conn) {
		req := challenge(conn)
		conn.WriteJSON(map[string]any{"type": "res", "id": req["id"], "ok": false, "error": map[string]any{"details": map[string]any{"code": "PAIRING_REQUIRED"}}})
	})
	c := NewNode(filepath.Join(t.TempDir(), "node.json"), "test", NodeMetadata{})
	c.dialer.TLSClientConfig = srv.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	c.ConfigureBootstrap(endpoint, "synthetic-bootstrap")
	c.Connect()
	defer c.Close()
	waitState(t, c, "pairing")
	c.PairAgain()
	if NewNode(c.path, "test", NodeMetadata{}).Status().Enabled {
		t.Fatal("pair again resumed")
	}
	c.Forget()
	if c.Status().Configured {
		t.Fatal("forget retained Node configuration")
	}
}

func nodeHello(conn *websocket.Conn, req map[string]any) {
	conn.WriteJSON(map[string]any{"type": "event", "event": "node.pair.resolved", "payload": map[string]any{"decision": "approved"}})

	conn.WriteJSON(map[string]any{"type": "res", "id": req["id"], "ok": true, "payload": map[string]any{"type": "hello-ok", "protocol": Protocol, "auth": map[string]any{"deviceToken": "synthetic-device-token", "role": "node", "scopes": []string{}}}})
}
func TestNodeRejectsUnexpectedHelloAuthority(t *testing.T) {
	srv, endpoint := server(t, func(conn *websocket.Conn) { req := challenge(conn); hello(conn, req) })
	c := NewNode(filepath.Join(t.TempDir(), "node.json"), "test", NodeMetadata{})
	c.dialer.TLSClientConfig = srv.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	c.ConfigureBootstrap(endpoint, "synthetic-bootstrap")
	c.Connect()
	defer c.Close()
	waitState(t, c, "error")
	if c.Status().Paired {
		t.Fatal("accepted operator token")
	}
}

func TestNodeBoundsPreHelloEvents(t *testing.T) {
	srv, endpoint := server(t, func(conn *websocket.Conn) {
		challenge(conn)
		for range 20 {
			conn.WriteJSON(map[string]any{"type": "event", "event": "tick"})
		}
	})
	c := NewNode(filepath.Join(t.TempDir(), "node.json"), "test", NodeMetadata{})
	c.dialer.TLSClientConfig = srv.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	c.ConfigureBootstrap(endpoint, "synthetic-bootstrap")
	c.Connect()
	defer c.Close()
	waitState(t, c, "error")
	if c.Status().Reason != "protocol" {
		t.Fatal("unbounded event stream")
	}
}
