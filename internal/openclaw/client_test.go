package openclaw

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func waitState(t *testing.T, c *Client, state string) {
	t.Helper()
	until := time.Now().Add(4 * time.Second)
	for time.Now().Before(until) {
		if c.Status().State == state {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("wanted %s, got %+v", state, c.Status())
}
func server(t *testing.T, handler func(*websocket.Conn)) (*httptest.Server, string) {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, e := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if e != nil {
			return
		}
		defer conn.Close()
		handler(conn)
	}))
	t.Cleanup(srv.Close)
	return srv, "wss" + strings.TrimPrefix(srv.URL, "https")
}
func client(t *testing.T, srv *httptest.Server, endpoint string) *Client {
	t.Helper()
	c := New(filepath.Join(t.TempDir(), "openclaw.json"), "test")
	c.dialer.TLSClientConfig = srv.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	if e := c.Configure(endpoint, "synthetic-bootstrap"); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { c.Disconnect() })
	return c
}
func challenge(conn *websocket.Conn) map[string]any {
	conn.WriteJSON(map[string]any{"type": "event", "event": "connect.challenge", "payload": map[string]any{"nonce": "synthetic-nonce", "ts": time.Now().UnixMilli()}})
	var req map[string]any
	conn.ReadJSON(&req)
	return req
}
func hello(conn *websocket.Conn, req map[string]any) {
	conn.WriteJSON(map[string]any{"type": "res", "id": req["id"], "ok": true, "payload": map[string]any{"type": "hello-ok", "protocol": Protocol, "auth": map[string]any{"deviceToken": "synthetic-device-token"}}})
}
func TestPairingSuccessPersistenceAndSignature(t *testing.T) {
	var verified atomic.Bool
	srv, endpoint := server(t, func(conn *websocket.Conn) {
		req := challenge(conn)
		p, ok := req["params"].(map[string]any)
		if !ok {
			return
		}
		d := p["device"].(map[string]any)
		pub, _ := base64.RawURLEncoding.DecodeString(d["publicKey"].(string))
		sig, _ := base64.RawURLEncoding.DecodeString(d["signature"].(string))
		hash := sha256.Sum256(pub)
		payload := strings.Join([]string{"v3", hex.EncodeToString(hash[:]), "openclaw-linux", "ui", "operator", "", jsonNumber(d["signedAt"]), "synthetic-bootstrap", "synthetic-nonce", "android", "tablet"}, "|")
		verified.Store(p["role"] == "operator" && len(p["scopes"].([]any)) == 0 && ed25519.Verify(pub, []byte(payload), sig))
		hello(conn, req)
		var f any
		conn.ReadJSON(&f)
	})
	c := client(t, srv, endpoint)
	if e := c.Connect(); e != nil {
		t.Fatal(e)
	}
	waitState(t, c, "connected")
	if !verified.Load() {
		t.Fatal("invalid identity signature or excess privileges")
	}
	if !c.Status().Paired {
		t.Fatal("token not persisted")
	}
	info, e := os.Stat(c.path)
	if e != nil || info.Mode().Perm() != 0600 {
		t.Fatal("credential file permissions")
	}
	c.Disconnect()
	saved := New(c.path, "test")
	if saved.Status().Enabled || !saved.Status().Paired {
		t.Fatal("preference/token did not persist")
	}
	b, _ := json.Marshal(c.Events())
	if strings.Contains(string(b), "synthetic") || strings.Contains(string(b), endpoint) {
		t.Fatal("diagnostic secret leak")
	}
	if e := c.Forget(); e != nil {
		t.Fatal(e)
	}
	if c.Status().Configured || c.Status().Paired {
		t.Fatal("forget failed")
	}
}
func jsonNumber(v any) string { b, _ := json.Marshal(v); return string(b) }
func TestPairingNeedsExplicitRetry(t *testing.T) {
	var attempts atomic.Int32
	srv, endpoint := server(t, func(conn *websocket.Conn) {
		attempts.Add(1)
		req := challenge(conn)
		conn.WriteJSON(map[string]any{"type": "res", "id": req["id"], "ok": false, "error": map[string]any{"details": map[string]any{"code": "PAIRING_REQUIRED"}, "message": "synthetic-private-details"}})
	})
	c := client(t, srv, endpoint)
	c.Connect()
	waitState(t, c, "pairing")
	time.Sleep(1100 * time.Millisecond)
	if attempts.Load() != 1 {
		t.Fatal("pairing retried automatically")
	}
	c.Connect()
	waitState(t, c, "pairing")
	if attempts.Load() != 2 {
		t.Fatal("manual retry failed")
	}
}
func TestReconnectAndExplicitDisconnect(t *testing.T) {
	var attempts atomic.Int32
	srv, endpoint := server(t, func(conn *websocket.Conn) {
		n := attempts.Add(1)
		req := challenge(conn)
		hello(conn, req)
		if n == 1 {
			return
		}
		var f any
		conn.ReadJSON(&f)
	})
	c := client(t, srv, endpoint)
	c.Connect()
	waitState(t, c, "reconnecting")
	waitState(t, c, "connected")
	c.Disconnect()
	n := attempts.Load()
	time.Sleep(1100 * time.Millisecond)
	if attempts.Load() != n || c.Status().State != "disconnected" {
		t.Fatal("explicit disconnect reconnected")
	}
}
func TestUntrustedTLSAndAuthentication(t *testing.T) {
	srv, endpoint := server(t, func(conn *websocket.Conn) {
		req := challenge(conn)
		conn.WriteJSON(map[string]any{"type": "res", "id": req["id"], "ok": false, "error": map[string]any{"details": map[string]any{"code": "AUTH_TOKEN_MISMATCH"}}})
	})
	c := client(t, srv, endpoint)
	c.Connect()
	waitState(t, c, "error")
	if c.Status().Reason != "authentication" {
		t.Fatal(c.Status())
	}
	c.Disconnect()
	c.dialer = &websocket.Dialer{HandshakeTimeout: time.Second}
	c.Connect()
	waitState(t, c, "error")
	if c.Status().Reason != "tls" {
		t.Fatal(c.Status())
	}
}
func TestEndpointValidation(t *testing.T) {
	for _, s := range []string{"ws://example.test", "wss://u:p@example.test", "wss://example.test/?token=x", "wss://example.test/#private", "wss://", "wss://example.test:99999"} {
		if ValidateEndpoint(s) == nil {
			t.Errorf("accepted %s", s)
		}
	}
	if ValidateEndpoint("wss://gateway.example.test/socket") != nil {
		t.Fatal("valid endpoint rejected")
	}
}
func TestMalformedStoredIdentity(t *testing.T) {
	p := filepath.Join(t.TempDir(), "openclaw.json")
	os.WriteFile(p, []byte(`{"url":"wss://example.test","private":"YQ==","enabled":true,"bootstrap":"synthetic"}`), 0600)
	c := New(p, "test")
	c.Start()
	waitState(t, c, "error")
	c.Disconnect()
}
