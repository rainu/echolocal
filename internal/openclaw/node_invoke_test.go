package openclaw

import (
	"context"
	"encoding/json"
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNodeInvokeUsesGatewayRequestResultContract(t *testing.T) {
	received := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, e := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if e != nil {
			return
		}
		defer ws.Close()
		var f map[string]any
		if ws.ReadJSON(&f) != nil {
			return
		}
		received <- f
		_ = ws.WriteJSON(map[string]any{"type": "res", "id": f["id"], "ok": true, "payload": map[string]any{}})
		time.Sleep(50 * time.Millisecond)
	}))
	defer server.Close()
	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	r := newRPCLink(ws)
	defer r.close()
	go func() {
		var f frame
		if ws.ReadJSON(&f) == nil {
			r.deliver(f)
		}
	}()
	a := NewNodeAccess(filepath.Join(t.TempDir(), "grants"))
	if err := a.Register(NodeOperation{Command: "device.info", Group: "state", Run: func(context.Context, json.RawMessage) (any, error) {
		return map[string]string{"version": "synthetic"}, nil
	}}); err != nil {
		t.Fatal(err)
	}
	c := NewNode(filepath.Join(t.TempDir(), "identity"), "test", NodeMetadata{})
	c.AttachNodeAccess(a)
	payload, _ := json.Marshal(map[string]any{"id": "invoke-test", "nodeId": "node-test", "command": "device.info", "paramsJSON": "{}", "timeoutMs": 1000})
	c.nodeEvent(context.Background(), r, frame{Event: "node.invoke.request", Payload: payload})
	select {
	case f := <-received:
		if f["method"] != "node.invoke.result" {
			t.Fatal("incorrect response method")
		}
		p := f["params"].(map[string]any)
		if p["id"] != "invoke-test" || p["ok"] != true || !strings.Contains(p["payloadJSON"].(string), "synthetic") {
			t.Fatal("incorrect result envelope")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no result")
	}
}
