package openclaw

import (
	"context"
	"encoding/json"
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestHealthDeniedNeverCollects(t *testing.T) {
	a := NewNodeAccess(filepath.Join(t.TempDir(), "grants"))
	var calls atomic.Int32
	a.Register(NodeOperation{Command: "echolocal.diagnostics.status", Group: "diagnostics", Run: func(context.Context, json.RawMessage) (any, error) { calls.Add(1); return nil, nil }})
	if e := a.SetGrants(map[string]bool{"diagnostics": false}); e != nil {
		t.Fatal(e)
	}
	c := NewNode(filepath.Join(t.TempDir(), "node"), "test", NodeMetadata{})
	c.AttachNodeAccess(a)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	c.nodeHealth(ctx, nil)
	if calls.Load() != 0 {
		t.Fatal("collected without grant")
	}
}
func TestHealthRevocationCancelsCollection(t *testing.T) {
	a := NewNodeAccess(filepath.Join(t.TempDir(), "grants"))
	entered := make(chan struct{})
	stopped := make(chan struct{})
	a.Register(NodeOperation{Command: "echolocal.diagnostics.status", Group: "diagnostics", Run: func(ctx context.Context, _ json.RawMessage) (any, error) {
		close(entered)
		<-ctx.Done()
		close(stopped)
		return map[string]any{"hostStats": map[string]any{}}, nil
	}})
	c := NewNode(filepath.Join(t.TempDir(), "node"), "test", NodeMetadata{})
	c.AttachNodeAccess(a)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { c.nodeHealth(ctx, nil); close(done) }()
	<-entered
	if e := a.SetGrants(map[string]bool{"diagnostics": false}); e != nil {
		t.Fatal(e)
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("ongoing collection not cancelled")
	}
	cancel()
	<-done
}

func TestHealthUsesExistingNodeEventContract(t *testing.T) {
	received := make(chan map[string]any, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ws, e := (&websocket.Upgrader{}).Upgrade(w, req, nil)
		if e != nil {
			return
		}
		defer ws.Close()
		var f map[string]any
		if ws.ReadJSON(&f) != nil {
			return
		}
		received <- f
		ws.WriteJSON(map[string]any{"type": "res", "id": f["id"], "ok": true, "payload": map[string]any{"handled": true}})
	}))
	defer srv.Close()
	ws, _, e := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if e != nil {
		t.Fatal(e)
	}
	defer ws.Close()
	link := newRPCLink(ws)
	defer link.close()
	go func() {
		var f frame
		if ws.ReadJSON(&f) == nil {
			link.deliver(f)
		}
	}()
	a := NewNodeAccess(filepath.Join(t.TempDir(), "grants"))
	a.Register(NodeOperation{Command: "echolocal.diagnostics.status", Group: "diagnostics", Run: func(context.Context, json.RawMessage) (any, error) {
		return map[string]any{"hostStats": map[string]any{"cpuCount": 4, "memoryTotalBytes": 1024, "memoryFreeBytes": 512}}, nil
	}})
	c := NewNode(filepath.Join(t.TempDir(), "node"), "test", NodeMetadata{})
	c.AttachNodeAccess(a)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { c.nodeHealth(ctx, link); close(done) }()
	select {
	case f := <-received:
		if f["method"] != "node.event" {
			t.Fatal(f)
		}
		p := f["params"].(map[string]any)
		if p["event"] != "node.host.stats" {
			t.Fatal(p)
		}
		var stats map[string]any
		if json.Unmarshal([]byte(p["payloadJSON"].(string)), &stats) != nil || stats["memoryFreeBytes"] != float64(512) {
			t.Fatal(p)
		}
	case <-time.After(time.Second):
		t.Fatal("missing stats event")
	}
	cancel()
	<-done
}

func TestHealthSamplesOftenButPublishesAtMostOncePerMinute(t *testing.T) {
	var samples, sent atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ws, e := (&websocket.Upgrader{}).Upgrade(w, req, nil)
		if e != nil {
			return
		}
		defer ws.Close()
		for {
			var f map[string]any
			if ws.ReadJSON(&f) != nil {
				return
			}
			sent.Add(1)
			if ws.WriteJSON(map[string]any{"type": "res", "id": f["id"], "ok": true, "payload": map[string]any{"handled": true}}) != nil {
				return
			}
		}
	}))
	defer srv.Close()
	ws, _, e := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if e != nil {
		t.Fatal(e)
	}
	defer ws.Close()
	link := newRPCLink(ws)
	defer link.close()
	go func() {
		for {
			var f frame
			if ws.ReadJSON(&f) != nil {
				return
			}
			link.deliver(f)
		}
	}()
	a := NewNodeAccess(filepath.Join(t.TempDir(), "grants"))
	a.Register(NodeOperation{Command: "echolocal.diagnostics.status", Group: "diagnostics", Run: func(context.Context, json.RawMessage) (any, error) {
		samples.Add(1)
		return map[string]any{"hostStats": map[string]any{"cpuCount": 4, "memoryTotalBytes": 1024, "memoryFreeBytes": 512}}, nil
	}})
	c := NewNode(filepath.Join(t.TempDir(), "node"), "test", NodeMetadata{})
	c.AttachNodeAccess(a)
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	c.nodeHealth(ctx, link)
	if samples.Load() != 2 || sent.Load() != 1 {
		t.Fatalf("samples=%d events=%d", samples.Load(), sent.Load())
	}
}
