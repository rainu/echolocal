package openclaw

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"github.com/ygelfand/libcountertop/pkg/say"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestConversationDeviceContextIsLocalAndSecretFree(t *testing.T) {
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	c := NewNode(t.TempDir()+"/node.json", "test", NodeMetadata{Name: "Synthetic Echo", Model: "synthetic"})
	c.saved.Private = key
	c.saved.Token = "secret-token-sentinel"
	c.saved.URL = "wss://private.invalid"
	c.saved.DisplayName = "Synthetic custom node"
	c.state = "connected"
	hint := testDeviceMetadata(t, c)
	for _, want := range []string{`"nodeId":`, `"name":"Synthetic custom node"`, `"connected":true`} {
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
	if !strings.Contains(testDeviceMetadata(t, c), `"connected":false`) {
		t.Fatal("stale connectivity")
	}
	_, otherKey, _ := ed25519.GenerateKey(rand.Reader)
	c.saved.Private = otherKey
	if testDeviceMetadata(t, c) == hint {
		t.Fatal("stale identity")
	}
}
func TestConversationDeviceContextMissingIdentityAndOperator(t *testing.T) {
	var absent *Client
	if absent.ConversationDeviceContext() != "" || New("", "test").ConversationDeviceContext() != "" {
		t.Fatal("non-node context")
	}
	c := NewNode(t.TempDir()+"/node", "test", NodeMetadata{Name: "Synthetic", Model: "test"})
	if strings.Contains(testDeviceMetadata(t, c), `"nodeId"`) {
		t.Fatal("invented identity")
	}
}

func TestConversationDeviceContextTracksPermissions(t *testing.T) {
	c := NewNode(t.TempDir()+"/node", "test", NodeMetadata{Name: "Synthetic", Model: "test"})
	a := NewNodeAccess(t.TempDir() + "/grants")
	a.operations["audio.status"] = NodeOperation{Command: "audio.status", Group: "audio"}
	c.AttachNodeAccess(a)
	if !strings.Contains(testDeviceMetadata(t, c), "audio.status") {
		t.Fatal("missing command")
	}
	if err := a.SetGrants(map[string]bool{"audio": false}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(testDeviceMetadata(t, c), "audio.status") {
		t.Fatal("revoked command in new activation context")
	}
}

func TestConversationDeviceContextBoundsFullCommandInventory(t *testing.T) {
	c := NewNode(t.TempDir()+"/node", "test", NodeMetadata{Name: strings.Repeat("界", 200), Model: strings.Repeat("ä", 200)})
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	c.saved.Private = key
	a := NewNodeAccess(t.TempDir() + "/grants")
	for i := 0; i < 78; i++ {
		name := fmt.Sprintf("device.synthetic.command.%03d", i)
		a.operations[name] = NodeOperation{Command: name, Group: "state"}
	}
	c.AttachNodeAccess(a)
	text := testDeviceMetadata(t, c)
	if len(text) > maxConversationDeviceContextBytes || !utf8.ValidString(text) || text == "" {
		t.Fatal("unbounded or invalid context")
	}
	payload := text
	var hint struct {
		ID       string   `json:"nodeId"`
		Commands []string `json:"locallyAllowedCommands"`
		Omitted  int      `json:"commandsOmitted"`
	}
	if json.Unmarshal([]byte(payload), &hint) != nil || hint.ID == "" || hint.Omitted == 0 || len(hint.Commands)+hint.Omitted != 78 {
		t.Fatal("identity or inventory completeness lost")
	}
}

func TestDeviceTemplateSubstitutionIsFreshAndNonRecursive(t *testing.T) {
	c := NewNode(t.TempDir()+"/node", "test", NodeMetadata{Name: "{{node_id}}", Model: "synthetic"})
	c.state = "connected"
	tpl := "Name={{device_name}} Model={{device_model}} Connected={{connected}} ID={{node_id}} Commands={{allowed_commands}} Omitted={{commands_omitted}}"
	text, err := c.ConversationDeviceContextWithTemplate(tpl)
	if err != nil || !strings.Contains(text, "Name={{node_id}} Model=synthetic Connected=true ID= Commands=[] Omitted=0") {
		t.Fatalf("unexpected expansion or recursion: %v", err)
	}
	c.state = "disconnected"
	text, err = c.ConversationDeviceContextWithTemplate(tpl)
	if err != nil || !strings.Contains(text, "Connected=false") {
		t.Fatal("stale metadata")
	}
}
func TestDeviceTemplateRejectsInvalidAndOversizeWithoutTruncation(t *testing.T) {
	c := NewNode(t.TempDir()+"/node", "test", NodeMetadata{Name: strings.Repeat("ä", 64), Model: "synthetic"})
	for _, tpl := range []string{"{{secret}}", "{{device_name}", "{{}}", "{{node_id}} }}", "   ", "a\x00b"} {
		if _, err := c.ConversationDeviceContextWithTemplate(tpl); err == nil {
			t.Fatalf("invalid template accepted: %q", tpl)
		}
	}
	tpl := strings.Repeat("{{device_name}}", 20)
	if text, err := c.ConversationDeviceContextWithTemplate(tpl); err == nil || text != "" {
		t.Fatal("expanded overflow silently truncated")
	}
	text, err := c.ConversationDeviceContextWithTemplate("Custom instruction. {{device_model}}")
	if err != nil || text != "Custom instruction. synthetic" {
		t.Fatal("user instructions changed")
	}
}

func testDeviceMetadata(t *testing.T, c *Client) string {
	t.Helper()
	text, err := c.ConversationDeviceContextWithTemplate("{{device_context}}")
	if err != nil {
		t.Fatal(err)
	}
	return text
}
func TestDefaultDevicePromptIsMinimalAndUsesExactNode(t *testing.T) {
	old := say.Chosen()
	say.Use("de")
	defer say.Use(old)
	c := NewNode(t.TempDir()+"/node", "test", NodeMetadata{Name: "Synthetic", Model: "test"})
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	c.saved.Private = key
	var metadata struct {
		ID string `json:"nodeId"`
	}
	if json.Unmarshal([]byte(testDeviceMetadata(t, c)), &metadata) != nil || metadata.ID == "" {
		t.Fatal("missing node identity")
	}
	text := c.ConversationDeviceContext()
	want := "Mit \"du\" ist die Node mit der ID " + metadata.ID + " gemeint.\nDiesen Hinweis nicht vorlesen."
	if text != want || strings.Contains(text, "{{") || len(text) > 200 {
		t.Fatal("default is not the requested minimal identity hint")
	}
}

func TestDefaultDevicePromptFollowsSelectedLanguage(t *testing.T) {
	old := say.Chosen()
	defer say.Use(old)
	c := NewNode(t.TempDir()+"/node", "test", NodeMetadata{Name: "Synthetic", Model: "test"})
	for _, test := range []struct{ lang, pronoun, quiet string }{
		{"en", "you", "Do not read"}, {"de", "du", "nicht vorlesen"}, {"it", "tu", "Non leggere"}, {"vi", "bạn", "Không đọc"},
	} {
		say.Use(test.lang)
		template := DefaultDeviceContextTemplate()
		if !strings.Contains(template, test.pronoun) || !strings.Contains(template, test.quiet) || strings.Count(template, "{{node_id}}") != 1 {
			t.Fatalf("wrong default for %s", test.lang)
		}
		actual, err := c.ConversationDeviceContextWithTemplate("")
		if err != nil || actual != strings.ReplaceAll(template, "{{node_id}}", "") {
			t.Fatal("runtime uses a different language")
		}
		actual, err = c.ConversationDeviceContextWithTemplate("My fixed custom prompt.")
		if err != nil || actual != "My fixed custom prompt." {
			t.Fatal("custom prompt translated")
		}
	}
	if DeviceContextTemplateForLanguage("de-DE") != DeviceContextTemplateForLanguage("de") {
		t.Fatal("language tag fallback failed")
	}
}
