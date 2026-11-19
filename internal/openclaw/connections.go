package openclaw

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// ConnectionSettings is private on-device configuration, never a diagnostic,
// Node export, or remotely writable API. Token is a shared Gateway credential,
// NEVER a role-scoped device token returned by a connection handshake.
type ConnectionSettings struct {
	URL      string `json:"url"`
	Token    string `json:"token,omitempty"`
	UIName   string `json:"uiName"`
	NodeName string `json:"nodeName"`
}
type Connections struct {
	UI, Node   *Client
	op         sync.Mutex
	mu         sync.RWMutex
	path       string
	saved      ConnectionSettings
	defaults   ConnectionSettings
	storageErr bool
}

func NewConnections(path string, ui, node *Client, uiName, nodeName string) *Connections {
	g := &Connections{UI: ui, Node: node, path: path, defaults: ConnectionSettings{UIName: uiName, NodeName: nodeName}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		g.saved = g.defaults
		// Migrate transport settings only. Role identities, device tokens, scoped
		// bootstrap tokens, preferences and permission files stay separate.
		primary := ui.snapshot()
		secondary := node.snapshot()
		if primary.URL == "" {
			primary = secondary
		}
		g.saved.URL = primary.URL
		if !primary.BootstrapOnly && primary.Bootstrap != primary.Token && primary.Bootstrap != secondary.Token {
			g.saved.Token = primary.Bootstrap
		}
		if g.saved.URL != "" && ValidateEndpoint(g.saved.URL) == nil {
			if g.write(g.saved) != nil {
				g.storageErr = true
			}
		}
	} else if err != nil || json.Unmarshal(data, &g.saved) != nil ||
		(g.saved.URL != "" && ValidateEndpoint(g.saved.URL) != nil) ||
		!validDisplayName(g.saved.UIName) || !validDisplayName(g.saved.NodeName) {
		g.storageErr = true
		g.saved = g.defaults
	}
	if g.saved.UIName == "" {
		g.saved.UIName = uiName
	}
	if g.saved.NodeName == "" {
		g.saved.NodeName = nodeName
	}
	// Upgrade only the exact former generated defaults; preserve custom names.
	renamed := false
	if g.saved.UIName == "EchoLocal · "+uiName {
		g.saved.UIName = uiName
		renamed = true
	}
	if g.saved.NodeName == "EchoLocal · "+nodeName {
		g.saved.NodeName = nodeName
		renamed = true
	}
	if renamed && !g.storageErr && g.write(g.saved) != nil {
		g.storageErr = true
	}
	ui.shared, node.shared = g, g
	if g.storageErr {
		ui.mark("error", "storage", "local", "settings")
		node.mark("error", "storage", "local", "settings")
	}
	return g
}
func validDisplayName(s string) bool {
	if !utf8.ValidString(s) || utf8.RuneCountInString(s) > 80 {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func (g *Connections) Editable() ConnectionSettings {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.saved
}
func (g *Connections) write(s ConnectionSettings) error {
	if os.MkdirAll(filepath.Dir(g.path), 0700) != nil {
		return errors.New("storage")
	}
	data, err := json.Marshal(s)
	if err != nil {
		return errors.New("storage")
	}
	f, err := os.CreateTemp(filepath.Dir(g.path), ".openclaw-connection-*")
	if err != nil {
		return errors.New("storage")
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	ce := f.Close()
	if err == nil {
		err = ce
	}
	if err == nil {
		err = os.Rename(f.Name(), g.path)
	}
	if err != nil {
		return errors.New("storage")
	}
	return nil
}
func (c *Client) snapshot() credentials {
	c.mu.Lock()
	s := c.saved
	c.mu.Unlock()
	if c.shared != nil {
		c.shared.mu.RLock()
		common, bad := c.shared.saved, c.shared.storageErr
		c.shared.mu.RUnlock()
		if bad {
			s.URL, s.Bootstrap, s.Token = "", "", ""
			return s
		}
		if s.URL != common.URL {
			s.Token, s.Bootstrap, s.BootstrapOnly = "", "", false
		}
		s.URL = common.URL
		if common.Token != "" {
			s.Bootstrap, s.BootstrapOnly = common.Token, false
		}
		s.DisplayName = common.UIName
		if c.node != nil {
			s.DisplayName = common.NodeName
		}
	}
	return s
}

// Save applies one transport form to both roles while retaining distinct
// private keys/device tokens. Previously enabled connections reconnect.
func (g *Connections) Save(next ConnectionSettings) error {
	next.UIName, next.NodeName = strings.TrimSpace(next.UIName), strings.TrimSpace(next.NodeName)
	if next.UIName == "" {
		next.UIName = g.defaults.UIName
	}
	if next.NodeName == "" {
		next.NodeName = g.defaults.NodeName
	}
	if ValidateEndpoint(next.URL) != nil {
		return errors.New("invalid_endpoint")
	}
	if !validDisplayName(next.UIName) || !validDisplayName(next.NodeName) || len(next.Token) > 16384 {
		return errors.New("invalid_name")
	}
	g.op.Lock()
	defer g.op.Unlock()
	old := g.Editable()
	g.mu.RLock()
	bad := g.storageErr
	g.mu.RUnlock()
	if !bad && next == old {
		return nil
	}
	g.UI.op.Lock()
	defer g.UI.op.Unlock()
	g.Node.op.Lock()
	defer g.Node.op.Unlock()
	g.UI.stopLocked()
	g.Node.stopLocked()
	clients := []*Client{g.UI, g.Node}
	oldRoles := make([]credentials, 2)
	nextRoles := make([]credentials, 2)
	for i, c := range clients {
		c.mu.Lock()
		state := c.saved
		c.mu.Unlock()
		oldRoles[i] = state
		if state.URL != next.URL || (next.Token != "" && next.Token != old.Token) {
			state.Token, state.Bootstrap, state.BootstrapOnly = "", "", false
		}
		state.URL = next.URL
		if old.Token != "" || next.Token != "" {
			state.Bootstrap, state.BootstrapOnly = "", false
		}
		if len(state.Private) == 0 {
			_, key, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				return errors.New("identity")
			}
			state.Private = key
		}
		if c.node != nil {
			state.Talk = false
		}
		nextRoles[i] = state
	}
	rollback := func() {
		failed := false
		for i, c := range clients {
			if c.persist(oldRoles[i]) != nil {
				failed = true
			}
		}
		if failed {
			g.mu.Lock()
			g.storageErr = true
			g.mu.Unlock()
			return
		}
		if !bad {
			if g.UI.Status().Enabled {
				g.UI.startLocked()
			}
			if g.Node.Status().Enabled {
				g.Node.startLocked()
			}
		}
	}
	// Both sockets are stopped and both role operation locks are held.
	// Commit the common source last; failed saves restore role credentials.
	for i, c := range clients {
		if err := c.persist(nextRoles[i]); err != nil {
			rollback()
			return err
		}
	}
	if err := g.write(next); err != nil {
		rollback()
		return err
	}
	g.mu.Lock()
	g.saved = next
	g.storageErr = false
	g.mu.Unlock()
	if g.UI.Status().Enabled {
		g.UI.startLocked()
	}
	if g.Node.Status().Enabled {
		g.Node.startLocked()
	}
	return nil
}
func (g *Connections) Connect() error {
	g.op.Lock()
	defer g.op.Unlock()
	common := g.Editable()
	if ValidateEndpoint(common.URL) != nil {
		return errors.New("invalid_endpoint")
	}
	// Migration from an operator-only setup needs a fresh, distinct Node key,
	// not a copy of the operator identity and not another credential form.
	for _, c := range []*Client{g.UI, g.Node} {
		c.op.Lock()
		c.mu.Lock()
		state := c.saved
		c.mu.Unlock()
		if len(state.Private) == 0 {
			_, key, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				c.op.Unlock()
				return errors.New("identity")
			}
			state.Private = key
			state.URL = common.URL
			if c.node != nil {
				state.Talk = false
			}
			if err := c.persist(state); err != nil {
				c.op.Unlock()
				return err
			}
		}
		c.op.Unlock()
	}
	return errors.Join(g.UI.Connect(), g.Node.Connect())
}
func (g *Connections) Disconnect() error {
	g.op.Lock()
	defer g.op.Unlock()
	return errors.Join(g.UI.Disconnect(), g.Node.Disconnect())
}
func (g *Connections) PairAgain() error {
	g.op.Lock()
	defer g.op.Unlock()
	return errors.Join(g.UI.PairAgain(), g.Node.PairAgain())
}
func (g *Connections) Forget() error {
	g.op.Lock()
	defer g.op.Unlock()
	if err := errors.Join(g.UI.Forget(), g.Node.Forget()); err != nil {
		return err
	}
	next := g.defaults
	if err := g.write(next); err != nil {
		return err
	}
	g.mu.Lock()
	g.saved = next
	g.storageErr = false
	g.mu.Unlock()
	return nil
}

// ConnectionSummary is safe for local status diagnostics: no endpoint,
// bearer, private key, device token or conversation content is returned.
type ConnectionSummary struct {
	ControllerUI                                   Status
	Node                                           Status
	UIName, NodeName                               string
	SharedAddressConfigured, SharedTokenConfigured bool
}

func (g *Connections) Summary() ConnectionSummary {
	cfg := g.Editable()
	return ConnectionSummary{g.UI.Status(), g.Node.Status(), cfg.UIName, cfg.NodeName, cfg.URL != "", cfg.Token != ""}
}
