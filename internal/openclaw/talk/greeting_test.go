package talk

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGreetingUsesExplicitSpeakableContractWithoutIdentityOrFakeUser(t *testing.T) {
	for _, model := range []string{"gpt-live-1", "gpt-live-1-codex"} {
		event := greetingAppend(model, GreetingForLanguage("de"))
		raw, _ := json.Marshal(event)
		for _, forbidden := range []string{"nodeId", "node_id", "session.update", "response.create", "conversation.item.create"} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatal("greeting leaks identity or replaces conversation", forbidden)
			}
		}
		if model == "gpt-live-1" && event["type"] != "session.commentary.append" {
			t.Fatal("public greeting is not speakable")
		}
		if model != "gpt-live-1" && (event["type"] != "session.context.append" || event["channel"] != "speakable") {
			t.Fatal("legacy greeting is not speakable")
		}
	}
	for _, lang := range []string{"en", "de", "it", "vi"} {
		if GreetingForLanguage(lang) == "" || GreetingForLanguage(lang) != GreetingForLanguage(lang+"-XX") {
			t.Fatal("missing language fallback", lang)
		}
	}
	if GreetingForLanguage("de") == GreetingForLanguage("en") {
		t.Fatal("greeting language ignored")
	}
}
