package openclaw

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"github.com/gorilla/websocket"
	"strings"
	"testing"
	"time"
)

func TestTalkScopesAreExplicitAndSigned(t *testing.T) {
	c := New(t.TempDir()+"/connection.json", "test")
	if err := c.Configure("wss://synthetic.example", "synthetic-token"); err != nil {
		t.Fatal(err)
	}
	if err := c.EnableTalk(); err != nil {
		t.Fatal(err)
	}
	p := connectParams(c.saved, "synthetic-nonce", 123, "test")
	scopes := p["scopes"].([]string)
	if strings.Join(scopes, ",") != strings.Join(SupportedOperatorScopes(), ",") {
		t.Fatal("wrong scopes")
	}
	d := p["device"].(map[string]any)
	sig, _ := base64.RawURLEncoding.DecodeString(d["signature"].(string))
	pub, _ := base64.RawURLEncoding.DecodeString(d["publicKey"].(string))
	text := strings.Join([]string{"v3", d["id"].(string), "openclaw-linux", "ui", "operator", strings.Join(scopes, ","), "123", "synthetic-token", "synthetic-nonce", "android", "tablet"}, "|")
	if !ed25519.Verify(pub, []byte(text), sig) {
		t.Fatal("scope signature missing")
	}
	if !New(c.path, "test").saved.Talk {
		t.Fatal("talk preference lost")
	}
}
func TestRPCCorrelationRedactionAndLoss(t *testing.T) {
	srv, endpoint := server(t, func(conn *websocket.Conn) {
		req := challenge(conn)
		hello(conn, req)
		var a, b map[string]any
		if conn.ReadJSON(&a) != nil || conn.ReadJSON(&b) != nil {
			return
		}
		conn.WriteJSON(map[string]any{"type": "res", "id": b["id"], "ok": true, "payload": map[string]any{"number": 2}})
		conn.WriteJSON(map[string]any{"type": "res", "id": a["id"], "ok": true, "payload": map[string]any{"number": 1}})
		var rejected map[string]any
		if conn.ReadJSON(&rejected) != nil {
			return
		}
		conn.WriteJSON(map[string]any{"type": "res", "id": rejected["id"], "ok": false, "error": map[string]any{"message": "synthetic-sensitive-error"}})
		var last map[string]any
		conn.ReadJSON(&last)
	})
	c := client(t, srv, endpoint)
	c.EnableTalk()
	c.Connect()
	waitState(t, c, "connected")
	type result struct{ Number int }
	type outcome struct {
		r result
		e error
	}
	done := make(chan outcome, 2)
	for i := 0; i < 2; i++ {
		go func() {
			var r result
			e := c.CallTalk(context.Background(), "talk.catalog", map[string]any{}, &r)
			done <- outcome{r, e}
		}()
	}
	seen := map[int]bool{}
	for i := 0; i < 2; i++ {
		o := <-done
		if o.e != nil {
			t.Fatal(o.e)
		}
		seen[o.r.Number] = true
	}
	if !seen[1] || !seen[2] {
		t.Fatal("responses not correlated")
	}
	if e := c.CallTalk(context.Background(), "talk.catalog", map[string]any{}, nil); e == nil || e.Error() != "rpc_rejected" {
		t.Fatal("raw error leaked")
	}
	if e := c.CallTalk(context.Background(), "talk.catalog", map[string]any{}, nil); e == nil || e.Error() != "rpc_delivery_uncertain" {
		t.Fatalf("loss: %v", e)
	}
	for _, event := range c.Events() {
		if strings.Contains(fmt.Sprint(event), "synthetic-sensitive") {
			t.Fatal("diagnostic leak")
		}
	}
}
func TestRPCAllowlistCancellationAndOptIn(t *testing.T) {
	c := New(t.TempDir()+"/connection.json", "test")
	if e := c.CallTalk(context.Background(), "config.set", nil, nil); e == nil || e.Error() != "rpc_method_not_allowed" {
		t.Fatal("method admitted")
	}
	if e := c.CallTalk(context.Background(), "talk.catalog", nil, nil); e == nil || e.Error() != "talk_authorization_required" {
		t.Fatal("implicit scope upgrade")
	}
	srv, endpoint := server(t, func(conn *websocket.Conn) {
		req := challenge(conn)
		hello(conn, req)
		var f any
		conn.ReadJSON(&f)
		conn.ReadJSON(&f)
	})
	c = client(t, srv, endpoint)
	c.EnableTalk()
	c.Connect()
	waitState(t, c, "connected")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if e := c.CallTalk(ctx, "talk.catalog", map[string]any{}, nil); e == nil || e.Error() != "rpc_cancelled" {
		t.Fatal("cancel failed")
	}
	c.mu.Lock()
	r := c.rpc
	c.mu.Unlock()
	r.mu.Lock()
	n := len(r.pending)
	r.mu.Unlock()
	if n != 0 {
		t.Fatal("pending request leaked")
	}
}
