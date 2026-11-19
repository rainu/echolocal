package talk

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDeviceContextWireIsSilentAndDoesNotReplaceInstructions(t *testing.T) {
	for _, model := range []string{"gpt-live-1", "gpt-live-1-codex"} {
		event := deviceContextAppend(model, "synthetic node context")
		raw, _ := json.Marshal(event)
		for _, forbidden := range []string{"session.update", "instructions", "response.create", "conversation.item.create"} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatal("context changed conversation ownership")
			}
		}
		if model == "gpt-live-1" && event["type"] != "session.thinking.append" {
			t.Fatal("public wire")
		}
		if model != "gpt-live-1" && (event["type"] != "session.context.append" || event["channel"] != "commentary") {
			t.Fatal("legacy wire")
		}
	}
	s := NewSession(nil, "synthetic", nil)
	if s.DeviceContext != "" {
		t.Fatal("unrelated session inherited context")
	}
}
