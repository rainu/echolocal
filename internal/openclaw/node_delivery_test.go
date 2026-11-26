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

type deliveryReceipt struct {
	Accepted bool `json:"accepted"`
	notify   chan bool
}

func (r *deliveryReceipt) NodeResultDelivered(ok bool) { r.notify <- ok }
func TestNodeDeferredWorkRequiresGatewayAcknowledgement(t *testing.T) {
	for _, tc := range []struct {
		name            string
		ack, disconnect bool
	}{{"accepted", true, false}, {"rejected", false, false}, {"disconnected", false, true}} {
		t.Run(tc.name, func(t *testing.T) {
			receipt := &deliveryReceipt{Accepted: true, notify: make(chan bool, 1)}
			sent := make(chan struct{})
			release := make(chan struct{})
			complete := make(chan struct{})
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
				p := f["params"].(map[string]any)
				if p["ok"] != true || p["payloadJSON"] != "{\"accepted\":true}" {
					t.Error("bad receipt")
				}
				close(sent)
				<-release
				if !tc.disconnect {
					_ = ws.WriteJSON(map[string]any{"type": "res", "id": f["id"], "ok": tc.ack, "payload": map[string]any{}, "error": map[string]any{"code": "UNAVAILABLE", "message": "synthetic"}})
					select {
					case <-complete:
					case <-time.After(3 * time.Second):
					}
				}
			}))
			defer server.Close()
			defer close(complete)
			ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer ws.Close()
			r := newRPCLink(ws)
			defer r.close()
			go func() {
				for {
					var f frame
					if ws.ReadJSON(&f) != nil {
						r.close()
						return
					}
					r.deliver(f)
				}
			}()
			a := NewNodeAccess(filepath.Join(t.TempDir(), "permissions"))
			_ = a.Register(NodeOperation{Command: "test.transfer", Group: "talk", Run: func(context.Context, json.RawMessage) (any, error) { return receipt, nil }})
			c := NewNode(filepath.Join(t.TempDir(), "id"), "test", NodeMetadata{})
			c.AttachNodeAccess(a)
			raw, _ := json.Marshal(map[string]any{"id": "test-call", "nodeId": "test-node", "command": "test.transfer", "paramsJSON": "{}", "timeoutMs": 2000})
			c.nodeEvent(context.Background(), r, frame{Event: "node.invoke.request", Payload: raw})
			<-sent
			select {
			case <-receipt.notify:
				t.Fatal("work ran before ack")
			default:
			}
			close(release)
			select {
			case ok := <-receipt.notify:
				if ok != tc.ack {
					t.Fatal("wrong delivery outcome", ok)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("no delivery outcome")
			}
		})
	}
}
func TestNodeDiscardedReceiptIsCancelled(t *testing.T) {
	for _, revoke := range []bool{false, true} {
		a := NewNodeAccess(filepath.Join(t.TempDir(), "permissions"))
		ctx, cancel := context.WithCancel(context.Background())
		receipt := &deliveryReceipt{notify: make(chan bool, 1)}
		_ = a.Register(NodeOperation{Command: "test.transfer", Group: "talk", Run: func(context.Context, json.RawMessage) (any, error) {
			if revoke {
				_ = a.SetGrants(map[string]bool{"talk": false})
			} else {
				cancel()
			}
			return receipt, nil
		}})
		if _, err := a.Execute(ctx, "test.transfer", []byte("{}")); err == nil {
			t.Fatal("discarded result accepted")
		}
		cancel()
		select {
		case ok := <-receipt.notify:
			if ok {
				t.Fatal("discarded result started work")
			}
		default:
			t.Fatal("discarded receipt leaked")
		}
	}
}
