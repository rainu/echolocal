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

func TestGreetingPromptValidationCountsUTF8BytesAndDoesNotExpandTemplates(t *testing.T) {
	for _, text := range []string{"", "Sag Hallo.\nDann zuhören.", "{{node_id}}", strings.Repeat("x", GreetingMaxBytes)} {
		if err := ValidateGreetingPrompt(text); err != nil {
			t.Fatal("valid plain prompt rejected", err)
		}
	}
	for _, text := range []string{" \n", string([]byte{0}), string([]byte{0xff}), strings.Repeat("x", GreetingMaxBytes+1), strings.Repeat("ü", GreetingMaxBytes/2+1)} {
		if ValidateGreetingPrompt(text) == nil {
			t.Fatal("invalid or oversized prompt accepted")
		}
	}
}
