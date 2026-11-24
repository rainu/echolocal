package openclaw

import (
	"context"
	"encoding/json"
	"testing"
)

func TestAppCannotBorrowNodeOrTalkAuthority(t *testing.T) {
	c := NewNode(t.TempDir()+"/node.json", "test", NodeMetadata{})
	c.rpc = newRPCLink(nil)
	c.granted = []string{"operator.admin"}
	if _, err := c.BindApp(); err == nil {
		t.Fatal("Node became operator")
	}
	c = New(t.TempDir()+"/ui.json", "test")
	c.rpc = newRPCLink(nil)
	c.granted = []string{"operator.sessions.read"}
	b, err := c.BindApp()
	if err != nil {
		t.Fatal(err)
	}
	if err = b.Call(context.Background(), "chat.send", nil, nil); err == nil || err.Error() != "app_scope_required" {
		t.Fatal("read grant permitted write")
	}
	if err = b.Call(context.Background(), "config.set", nil, nil); err == nil || err.Error() != "rpc_method_not_allowed" {
		t.Fatal("method fence")
	}
	c.rpc = newRPCLink(nil)
	if err = b.Call(context.Background(), "sessions.list", nil, nil); err == nil || err.Error() != "app_connection_changed" {
		t.Fatal("reconnect transferred request")
	}
}
func TestAppInvalidationsContainNoPayloadAndCloseWithSocket(t *testing.T) {
	c := New(t.TempDir()+"/ui.json", "test")
	c.rpc = newRPCLink(nil)
	b, _ := c.BindApp()
	updates, done, err := b.Updates()
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	c.rpc.event(frame{Event: "session.message", Payload: json.RawMessage(`{"text":"synthetic-private-content"}`)})
	select {
	case <-updates:
	default:
		t.Fatal("missing invalidation")
	}
	c.rpc.close()
	if _, ok := <-updates; ok {
		t.Fatal("subscription survived disconnect")
	}
}
