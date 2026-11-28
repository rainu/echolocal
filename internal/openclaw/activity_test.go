package openclaw

import (
	"context"
	"encoding/json"
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSessionActivityWireProjection(t *testing.T) {
	for _, tc := range []struct {
		name, row string
		ok, work  bool
	}{
		{"idle", `{"key":"agent:test:one","sessionId":"generation1","hasActiveRun":false}`, true, false},
		{"reply_or_tool", `{"key":"agent:test:one","sessionId":"generation1","hasActiveRun":true,"message":"private","reasoning":"private","activeRunIds":["private"]}`, true, true},
		{"child_background", `{"key":"agent:test:one","sessionId":"generation1","hasActiveRun":false,"hasActiveSubagentRun":true}`, true, true},
		{"unknown", `{"key":"agent:test:one","sessionId":"generation1"}`, false, false},
		{"wrong_target", `{"key":"agent:test:two","sessionId":"generation1","hasActiveRun":true}`, false, false},
		{"missing_incarnation", `{"key":"agent:test:one","hasActiveRun":true}`, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				ws, err := (&websocket.Upgrader{}).Upgrade(w, req, nil)
				if err != nil {
					return
				}
				defer ws.Close()
				var f struct {
					ID, Method string
					Params     map[string]any
				}
				if ws.ReadJSON(&f) != nil {
					return
				}
				if f.Method != "sessions.describe" || len(f.Params) != 1 || f.Params["key"] != "agent:test:one" {
					t.Error("non-observational request", f.Method, f.Params)
					return
				}
				ws.WriteJSON(map[string]any{"type": "res", "id": f.ID, "ok": true, "payload": json.RawMessage(`{"session":` + tc.row + `}`)})
			}))
			defer srv.Close()
			ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer ws.Close()
			c := New(t.TempDir()+"/operator.json", "test")
			c.saved.Talk = true
			c.granted = []string{"operator.read"}
			c.rpc = newRPCLink(ws)
			go func() {
				var f frame
				if ws.ReadJSON(&f) == nil {
					c.rpc.deliver(f)
				}
			}()
			b, err := c.BindTalk()
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			got, err := b.ReadSessionActivity(ctx, "agent:test:one")
			if (err == nil) != tc.ok || got.Working != tc.work {
				t.Fatal(got, err)
			}
			if tc.ok {
				data, _ := json.Marshal(got)
				if strings.Contains(string(data), "private") {
					t.Fatal("content escaped projection")
				}
			}
		})
	}
}
func TestSessionActivityCannotBorrowTalkScopeOrReplacementSocket(t *testing.T) {
	c := New(t.TempDir()+"/operator.json", "test")
	c.saved.Talk = true
	c.rpc = newRPCLink(nil)
	c.granted = []string{"operator.talk"}
	b, _ := c.BindTalk()
	if _, err := b.ReadSessionActivity(context.Background(), "agent:test:one"); err == nil || err.Error() != "app_scope_required" {
		t.Fatal(err)
	}
	c.rpc = newRPCLink(nil)
	c.granted = []string{"operator.admin"}
	if _, err := b.ReadSessionActivity(context.Background(), "agent:test:one"); err == nil || err.Error() != "talk_connection_changed" {
		t.Fatal(err)
	}
}
