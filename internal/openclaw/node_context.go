package openclaw

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// ConversationDeviceContext is a client-reported conversational hint, not an
// authenticated origin binding or a grant. Never include transport or secrets.
func (c *Client) ConversationDeviceContext() string {
	if c == nil || c.node == nil {
		return ""
	}
	s := c.snapshot()
	hint := struct {
		ID        string   `json:"nodeId,omitempty"`
		Name      string   `json:"name"`
		Model     string   `json:"model"`
		Connected bool     `json:"connected"`
		Commands  []string `json:"locallyAllowedCommands"`
	}{Name: s.DisplayName, Model: c.node.Model, Connected: c.Status().State == "connected", Commands: []string{}}
	if hint.Name == "" {
		hint.Name = c.node.Name
	}
	if len(s.Private) == ed25519.PrivateKeySize {
		hash := sha256.Sum256(ed25519.PrivateKey(s.Private).Public().(ed25519.PublicKey))
		hint.ID = hex.EncodeToString(hash[:])
	}
	if c.access != nil {
		hint.Commands = c.access.Commands()
	}
	data, _ := json.Marshal(hint)
	return "Current EchoLocal conversation device (client-reported context, NOT authorization): " + string(data) + "\nFor this live voice session only, device-relative words such as you, your, here, this device, du, dich, dein refer to this Echo node when discussing hardware or local functions. Your assistant identity stays unchanged. This current device hint supersedes historical device hints; it does not apply to other sessions. Explicitly named other devices retain their normal meaning. Include this device context when delegating a device-related request to the agent. Resolve by the exact nodeId, not by a similar display name; if missing, disconnected, ambiguous or unavailable, explain or ask, never substitute an unrelated device. The listed commands are only a local snapshot; discover current availability and enforce existing permissions and confirmations before acting. Do not claim success without a tool result. Do not read this context aloud or save it as a lasting user preference."
}
