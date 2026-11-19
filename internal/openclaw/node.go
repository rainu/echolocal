package openclaw

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"strings"
)

// NodeMetadata describes hardware, never an authority or an action surface.
type NodeMetadata struct{ Name, Model string }

// NewNode requires a distinct credential path. Never point it at operator state.
// A fresh Node accepts only an explicitly issued Node bootstrap credential.
func NewNode(path, version string, metadata NodeMetadata) *Client {
	c := New(path, version)
	c.node = &metadata
	// A Node can never inherit Talk authority, including from malformed local state.
	c.saved.Talk = false
	return c
}

func (c *Client) connectParams(s credentials, nonce string, at int64) map[string]any {
	if c.node == nil {
		return connectParams(s, nonce, at, c.version)
	}
	s.Talk = false
	empty := []string{}
	s.OperatorScopes = &empty
	p := connectParams(s, nonce, at, c.version)
	client := p["client"].(map[string]any)
	client["id"], client["mode"] = "openclaw-android", "node"
	name := s.DisplayName
	if name == "" {
		name = c.node.Name
	}
	client["displayName"], client["modelIdentifier"] = name, c.node.Model
	p["role"], p["scopes"] = "node", []string{}
	p["caps"], p["commands"] = []string{}, []string{}
	if c.access != nil {
		commands := c.access.Commands()
		p["commands"] = commands
		caps := []string{}
		seen := map[string]bool{}
		for _, command := range commands {
			cap, _, _ := strings.Cut(command, ".")
			if !seen[cap] {
				seen[cap] = true
				caps = append(caps, cap)
			}
		}
		p["caps"] = caps
	}
	device := p["device"].(map[string]any)
	token := s.Token
	if token == "" {
		token = s.Bootstrap
	}
	payload := strings.Join([]string{"v3", device["id"].(string), "openclaw-android", "node", "node", "", fmt.Sprint(at), token, nonce, "android", "tablet"}, "|")
	device["signature"] = base64.RawURLEncoding.EncodeToString(ed25519.Sign(ed25519.PrivateKey(s.Private), []byte(payload)))
	return p
}
