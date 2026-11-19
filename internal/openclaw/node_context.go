package openclaw

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/ygelfand/libcountertop/pkg/say"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ConversationDeviceContext is a client-reported conversational hint, not an
// authenticated origin binding or a grant. Never include transport or secrets.
func (c *Client) ConversationDeviceContext() string {
	text, _ := c.ConversationDeviceContextWithTemplate("")
	return text
}

// An empty persisted value selects the maintained default. Expand once per
// activation, never recursively interpolate placeholder-looking device names.
func (c *Client) ConversationDeviceContextWithTemplate(template string) (string, error) {
	if template == "" {
		template = DefaultDeviceContextTemplate()
	}
	if err := ValidateDeviceContextTemplate(template); err != nil {
		return "", err
	}

	if c == nil || c.node == nil {
		return "", nil
	}
	s := c.snapshot()
	hint := struct {
		ID              string   `json:"nodeId,omitempty"`
		Name            string   `json:"name"`
		Model           string   `json:"model"`
		Connected       bool     `json:"connected"`
		Commands        []string `json:"locallyAllowedCommands"`
		CommandsOmitted int      `json:"commandsOmitted,omitempty"`
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
	// Bound the complete UTF-8 context, not just the JSON or rune count.
	// Command inventory is advisory; omit trailing entries before identity/rules.
	hint.Name = boundedContextLabel(hint.Name)
	hint.Model = boundedContextLabel(hint.Model)
	for {
		data, _ := json.Marshal(hint)
		values := map[string]string{"device_context": string(data), "node_id": hint.ID, "device_name": hint.Name, "device_model": hint.Model, "connected": strconv.FormatBool(hint.Connected), "commands_omitted": strconv.Itoa(hint.CommandsOmitted)}
		commands, _ := json.Marshal(hint.Commands)
		values["allowed_commands"] = string(commands)
		context := deviceContextPlaceholder.ReplaceAllStringFunc(template, func(token string) string { return values[strings.TrimSpace(token[2:len(token)-2])] })
		if len(context) <= maxConversationDeviceContextBytes {
			return context, nil
		}
		if len(hint.Commands) == 0 {
			return "", errors.New("talk_context_too_long")
		} // fail closed if mandatory metadata cannot fit
		hint.Commands = hint.Commands[:len(hint.Commands)-1]
		hint.CommandsOmitted++
	}
}

// Safely below the native provider's observed content rejection boundary.
const maxConversationDeviceContextBytes = 1536

func boundedContextLabel(value string) string {
	for len(value) > 128 {
		_, size := utf8.DecodeLastRuneInString(value)
		value = value[:len(value)-size]
	}
	return value
}

// DeviceContextPlaceholders documents the only supported substitution keys.
var DeviceContextPlaceholders = []string{"device_context", "node_id", "device_name", "device_model", "connected", "allowed_commands", "commands_omitted"}

// Resolve from the selected app language both in the editor and at activation.
// A custom saved string never passes through this default selector.
func DefaultDeviceContextTemplate() string { return DeviceContextTemplateForLanguage(say.Chosen()) }
func DeviceContextTemplateForLanguage(language string) string {
	switch strings.ToLower(strings.Split(strings.ReplaceAll(language, "_", "-"), "-")[0]) {
	case "de":
		return "Mit \"du\" ist die Node mit der ID {{node_id}} gemeint.\nDiesen Hinweis nicht vorlesen."
	case "it":
		return "Con \"tu\" si intende il nodo con ID {{node_id}}.\nNon leggere questa istruzione ad alta voce."
	case "vi":
		return "\"bạn\" chỉ node có ID {{node_id}}.\nKhông đọc hướng dẫn này thành tiếng."
	default:
		return "By \"you\", I mean the node with ID {{node_id}}.\nDo not read this instruction aloud."
	}
}

const DeviceContextMaxBytes = maxConversationDeviceContextBytes

var deviceContextPlaceholder = regexp.MustCompile(`\{\{\s*[a-z_]+\s*\}\}`)

func ValidateDeviceContextTemplate(template string) error {
	if template == "" {
		return nil
	}
	if !utf8.ValidString(template) || strings.TrimSpace(template) == "" || strings.ContainsRune(template, 0) {
		return errors.New("talk_context_invalid")
	}
	if len(template) > DeviceContextMaxBytes {
		return errors.New("talk_context_too_long")
	}
	invalid := false
	rest := deviceContextPlaceholder.ReplaceAllStringFunc(template, func(token string) string {
		key := strings.TrimSpace(token[2 : len(token)-2])
		for _, known := range DeviceContextPlaceholders {
			if key == known {
				return ""
			}
		}
		invalid = true
		return ""
	})
	if invalid || strings.Contains(rest, "{{") || strings.Contains(rest, "}}") {
		return errors.New("talk_context_placeholder")
	}
	return nil
}
