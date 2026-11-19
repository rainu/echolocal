package openclaw

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func connectionRig(t *testing.T) (*Connections, *Client, *Client) {
	t.Helper()
	dir := t.TempDir()
	ui := New(filepath.Join(dir, "ui.json"), "test")
	node := NewNode(filepath.Join(dir, "node.json"), "test", NodeMetadata{"legacy node", "Athena"})
	return NewConnections(filepath.Join(dir, "common.json"), ui, node, "Echo UI", "Echo Node"), ui, node
}
func TestSharedFreshConfigurationKeepsDistinctIdentitiesAndTokens(t *testing.T) {
	g, ui, node := connectionRig(t)
	cfg := g.Editable()
	cfg.URL = "wss://synthetic.example"
	cfg.Token = "synthetic-common-gateway"
	if err := g.Save(cfg); err != nil {
		t.Fatal(err)
	}
	if len(ui.saved.Private) != ed25519.PrivateKeySize || bytes.Equal(ui.saved.Private, node.saved.Private) {
		t.Fatal("shared private identity")
	}
	for _, c := range []*Client{ui, node} {
		s := c.snapshot()
		p := c.connectParams(s, "nonce", 123)
		if p["auth"].(map[string]any)["token"] != cfg.Token {
			t.Fatal("common Gateway credential not used")
		}
		if c.saved.Bootstrap != "" {
			t.Fatal("common credential copied into role storage")
		}
	}
	up, np := ui.saved, node.saved
	up.Token, np.Token = "synthetic-ui-device", "synthetic-node-device"
	up.Talk = true
	if ui.persist(up) != nil || node.persist(np) != nil {
		t.Fatal("persist")
	}
	oldUI, oldNode := append([]byte(nil), up.Private...), append([]byte(nil), np.Private...)
	cfg.UIName, cfg.NodeName = "Living room UI", "Living room Node"
	if err := g.Save(cfg); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ui.saved.Private, oldUI) || !bytes.Equal(node.saved.Private, oldNode) {
		t.Fatal("rename changed identity")
	}
	if !ui.saved.Talk || node.saved.Talk {
		t.Fatal("role authority changed")
	}
	for _, c := range []*Client{ui, node} {
		p := c.connectParams(c.snapshot(), "nonce", 124)
		wantToken := "synthetic-ui-device"
		wantName := cfg.UIName
		if c == node {
			wantToken = "synthetic-node-device"
			wantName = cfg.NodeName
		}
		if p["auth"].(map[string]any)["token"] != wantToken || p["client"].(map[string]any)["displayName"] != wantName {
			t.Fatal("role-specific auth or name lost")
		}
	}
	if node.connectParams(node.snapshot(), "nonce", 1)["role"] != "node" {
		t.Fatal("node role widened")
	}
	cfg.Token = ""
	if err := g.Save(cfg); err != nil {
		t.Fatal(err)
	}
	if ui.snapshot().Bootstrap != "" || node.snapshot().Bootstrap != "" || ui.saved.Token == "" || node.saved.Token == "" {
		t.Fatal("clearing shared bearer broke paired auth or retained old bearer")
	}
	restored := NewConnections(g.path, New(ui.path, "test"), NewNode(node.path, "test", NodeMetadata{}), "Echo UI", "Echo Node")
	if restored.Editable() != cfg {
		t.Fatal("shared settings did not persist")
	}
	data, err := os.ReadFile(g.path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "synthetic-ui-device") || strings.Contains(string(data), "synthetic-node-device") {
		t.Fatal("device token migrated to common form")
	}
}
func TestSharedMigrationNeverPromotesDeviceOrScopedBootstrapTokens(t *testing.T) {
	dir := t.TempDir()
	ui := New(filepath.Join(dir, "ui"), "test")
	node := NewNode(filepath.Join(dir, "node"), "test", NodeMetadata{})
	if ui.Configure("wss://synthetic.example", "") != nil || node.ConfigureBootstrap("wss://other.example", "synthetic-node-bootstrap") != nil {
		t.Fatal("configure")
	}
	u, n := ui.saved, node.saved
	u.Token = "synthetic-ui-device"
	n.Token = "synthetic-node-device"
	if ui.persist(u) != nil || node.persist(n) != nil {
		t.Fatal("persist")
	}
	g := NewConnections(filepath.Join(dir, "common"), ui, node, "UI", "Node")
	if g.Editable().Token != "" || g.Editable().URL != "wss://synthetic.example" {
		t.Fatal("migration promoted a role credential")
	}
	if ui.snapshot().Token != "synthetic-ui-device" || node.snapshot().Token != "" || node.snapshot().Bootstrap != "" {
		t.Fatal("credentials crossed Gateway boundary")
	}
	if !bytes.Equal(node.saved.Private, n.Private) {
		t.Fatal("migration regenerated Node identity")
	}
}
func TestSharedNamesAndStorageFailClosed(t *testing.T) {
	g, ui, node := connectionRig(t)
	cfg := g.Editable()
	cfg.URL = "wss://synthetic.example"
	cfg.Token = "synthetic-common"
	if g.Save(cfg) != nil {
		t.Fatal("configure")
	}
	before := g.Editable()
	for _, name := range []string{"bad\nname", strings.Repeat("x", 81), string([]byte{0xff})} {
		cfg.UIName = name
		if g.Save(cfg) == nil || g.Editable() != before {
			t.Fatal("invalid name changed saved configuration")
		}
	}
	cfg = before
	cfg.UIName, cfg.NodeName = "", ""
	if g.Save(cfg) != nil || g.Editable().UIName != "Echo UI" || g.Editable().NodeName != "Echo Node" {
		t.Fatal("default names not restored")
	}
	if os.WriteFile(g.path, []byte("{broken"), 0600) != nil {
		t.Fatal("write fixture")
	}
	loaded := NewConnections(g.path, New(ui.path, "test"), NewNode(node.path, "test", NodeMetadata{}), "UI", "Node")
	if loaded.UI.Status().Configured || loaded.Node.Status().Configured || loaded.UI.Status().State != "error" {
		t.Fatal("malformed common config used old credentials")
	}
}
func TestSharedRenameReconnectsBothWithStableRoleDevices(t *testing.T) {
	var mu sync.Mutex
	seen := map[string][]map[string]any{}
	srv, endpoint := server(t, func(conn *websocket.Conn) {
		req := challenge(conn)
		if req == nil {
			return
		}
		p, ok := req["params"].(map[string]any)
		if !ok {
			return
		}
		role := p["role"].(string)
		mu.Lock()
		seen[role] = append(seen[role], p)
		mu.Unlock()
		_ = conn.WriteJSON(map[string]any{"type": "res", "id": req["id"], "ok": true, "payload": map[string]any{
			"type": "hello-ok", "protocol": Protocol, "auth": map[string]any{"role": role, "scopes": []string{}, "deviceToken": "synthetic-" + role + "-device"},
		}})
		var f any
		_ = conn.ReadJSON(&f)
	})
	g, ui, node := connectionRig(t)
	for _, c := range []*Client{ui, node} {
		c.dialer.TLSClientConfig = srv.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	}
	defer ui.Close()
	defer node.Close()
	cfg := g.Editable()
	cfg.URL = endpoint
	cfg.Token = "synthetic-common"
	if g.Save(cfg) != nil || g.Connect() != nil {
		t.Fatal("connect")
	}
	waitState(t, ui, "connected")
	waitState(t, node, "connected")
	cfg.UIName, cfg.NodeName = "Renamed UI", "Renamed Node"
	if err := g.Save(cfg); err != nil {
		t.Fatal(err)
	}
	waitState(t, ui, "connected")
	waitState(t, node, "connected")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		ready := len(seen["operator"]) >= 2 && len(seen["node"]) >= 2
		mu.Unlock()
		if ready {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, role := range []string{"operator", "node"} {
		v := seen[role]
		if len(v) < 2 {
			t.Fatal("role did not reconnect", role)
		}
		first, last := v[0], v[len(v)-1]
		if first["device"].(map[string]any)["id"] != last["device"].(map[string]any)["id"] {
			t.Fatal("renaming made a new device")
		}
		if last["auth"].(map[string]any)["token"] != "synthetic-"+role+"-device" {
			t.Fatal("role token not retained")
		}
		want := cfg.UIName
		if role == "node" {
			want = cfg.NodeName
		}
		if last["client"].(map[string]any)["displayName"] != want {
			t.Fatal("new name not advertised")
		}
	}
}
func TestSharedPersistentFileContainsOnlyCommonForm(t *testing.T) {
	g, _, _ := connectionRig(t)
	cfg := g.Editable()
	cfg.URL = "wss://synthetic.example"
	cfg.Token = "synthetic-common"
	if g.Save(cfg) != nil {
		t.Fatal("save")
	}
	info, err := os.Stat(g.path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("private form permissions")
	}
	data, _ := os.ReadFile(g.path)
	var keys map[string]any
	if json.Unmarshal(data, &keys) != nil || len(keys) != 4 {
		t.Fatal("unexpected shared state")
	}
	for _, key := range []string{"private", "deviceToken", "talk"} {
		if _, ok := keys[key]; ok {
			t.Fatal("role-owned state in shared form")
		}
	}
}

func TestSharedSaveFailureRestoresRoleCredentials(t *testing.T) {
	g, ui, node := connectionRig(t)
	cfg := g.Editable()
	cfg.URL = "wss://synthetic.example"
	cfg.Token = "synthetic-common"
	if g.Save(cfg) != nil {
		t.Fatal("save")
	}
	up, np := ui.saved, node.saved
	up.Token, np.Token = "synthetic-ui-device", "synthetic-node-device"
	if ui.persist(up) != nil || node.persist(np) != nil {
		t.Fatal("fixture")
	}
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if os.WriteFile(blocked, []byte("fixture"), 0600) != nil {
		t.Fatal("fixture")
	}
	g.path = filepath.Join(blocked, "common")
	cfg.Token = "synthetic-new-common"
	if g.Save(cfg) == nil {
		t.Fatal("storage failure ignored")
	}
	if g.Editable().Token != "synthetic-common" || ui.saved.Token != "synthetic-ui-device" || node.saved.Token != "synthetic-node-device" {
		t.Fatal("failed save lost paired credentials")
	}
	if !bytes.Equal(up.Private, ui.saved.Private) || !bytes.Equal(np.Private, node.saved.Private) {
		t.Fatal("failed save changed identity")
	}
}
func TestSharedSummaryNeverIncludesCredentialValues(t *testing.T) {
	g, _, _ := connectionRig(t)
	cfg := g.Editable()
	cfg.URL = "wss://synthetic-private.example"
	cfg.Token = "synthetic-common-secret"
	if g.Save(cfg) != nil {
		t.Fatal("save")
	}
	data, err := json.Marshal(g.Summary())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "synthetic-private") || strings.Contains(string(data), "synthetic-common-secret") {
		t.Fatal("status leaked private configuration")
	}
}

func TestFormerGeneratedNamesUpgradeWithoutChangingCustomNamesOrIdentity(t *testing.T) {
	for _, custom := range []bool{false, true} {
		t.Run(fmtBool(custom), func(t *testing.T) {
			g, ui, node := connectionRig(t)
			cfg := g.Editable()
			cfg.URL = "wss://synthetic.example"
			cfg.UIName, cfg.NodeName = "EchoLocal · Echo UI", "EchoLocal · Echo Node"
			if custom {
				cfg.UIName = "Kitchen"
				cfg.NodeName = "EchoLocal · Custom Node"
			}
			if err := g.Save(cfg); err != nil {
				t.Fatal(err)
			}
			beforeUI, beforeNode := ui.snapshot(), node.snapshot()
			reloaded := NewConnections(g.path, ui, node, "Echo UI", "Echo Node")
			got := reloaded.Editable()
			wantUI, wantNode := "Echo UI", "Echo Node"
			if custom {
				wantUI, wantNode = cfg.UIName, cfg.NodeName
			}
			if got.UIName != wantUI || got.NodeName != wantNode {
				t.Fatal("incorrect default upgrade")
			}
			if !bytes.Equal(beforeUI.Private, ui.snapshot().Private) || !bytes.Equal(beforeNode.Private, node.snapshot().Private) {
				t.Fatal("identity changed")
			}
			if beforeUI.Token != ui.snapshot().Token || beforeNode.Token != node.snapshot().Token {
				t.Fatal("device token changed")
			}
			var persisted ConnectionSettings
			data, err := os.ReadFile(g.path)
			if err != nil || json.Unmarshal(data, &persisted) != nil || persisted.UIName != wantUI || persisted.NodeName != wantNode {
				t.Fatal("name upgrade not persisted")
			}
		})
	}
}
func fmtBool(b bool) string {
	if b {
		return "custom"
	}
	return "generated"
}
