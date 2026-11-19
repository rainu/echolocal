package openclaw

import (
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"testing"
)

func TestConversationDeviceContextIsLocalAndSecretFree(t *testing.T) {
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	c := NewNode(t.TempDir()+"/node.json", "test", NodeMetadata{Name: "Synthetic Echo", Model: "synthetic"})
	c.saved.Private = key
	c.saved.Token = "secret-token-sentinel"
	c.saved.URL = "wss://private.invalid"
	c.saved.DisplayName = "Synthetic custom node"
	c.state = "connected"
	hint := c.ConversationDeviceContext()
	for _, want := range []string{`"nodeId":`, `"name":"Synthetic custom node"`, `"connected":true`, "NOT authorization", "Explicitly named other devices", "assistant identity stays unchanged"} {
		if !strings.Contains(hint, want) {
			t.Fatalf("missing %s", want)
		}
	}
	for _, secret := range []string{"secret-token-sentinel", "private.invalid", string(key)} {
		if strings.Contains(hint, secret) {
			t.Fatal("private state escaped")
		}
	}
	c.state = "disconnected"
	if !strings.Contains(c.ConversationDeviceContext(), `"connected":false`) {
		t.Fatal("stale connectivity")
	}
	_, otherKey, _ := ed25519.GenerateKey(rand.Reader)
	c.saved.Private = otherKey
	if c.ConversationDeviceContext() == hint {
		t.Fatal("stale identity")
	}
}
func TestConversationDeviceContextMissingIdentityAndOperator(t *testing.T) {
	var absent *Client
	if absent.ConversationDeviceContext() != "" || New("", "test").ConversationDeviceContext() != "" {
		t.Fatal("non-node context")
	}
	c := NewNode(t.TempDir()+"/node", "test", NodeMetadata{Name: "Synthetic", Model: "test"})
	if strings.Contains(c.ConversationDeviceContext(), `"nodeId"`) {
		t.Fatal("invented identity")
	}
}

func TestConversationDeviceContextTracksPermissions(t *testing.T) {
	c := NewNode(t.TempDir()+"/node", "test", NodeMetadata{Name: "Synthetic", Model: "test"})
	a := NewNodeAccess(t.TempDir() + "/grants")
	a.operations["audio.status"] = NodeOperation{Command: "audio.status", Group: "audio"}
	c.AttachNodeAccess(a)
	if !strings.Contains(c.ConversationDeviceContext(), "audio.status") {
		t.Fatal("missing command")
	}
	if err := a.SetGrants(map[string]bool{"audio": false}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(c.ConversationDeviceContext(), "audio.status") {
		t.Fatal("revoked command in new activation context")
	}
}
