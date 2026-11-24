package talk

import (
	"errors"
	"strings"
	"unicode/utf8"
)

const GreetingMaxBytes = 1536

func ValidateGreetingPrompt(text string) error {
	if text == "" {
		return nil
	}
	if !utf8.ValidString(text) || strings.TrimSpace(text) == "" || strings.ContainsRune(text, 0) {
		return errors.New("talk_greeting_invalid")
	}
	if len(text) > GreetingMaxBytes {
		return errors.New("talk_greeting_too_long")
	}
	return nil
}

// Same provider contract as OpenClaw's triggerGreeting: an explicit speakable
// update. Device identity stays on its separate silent first-speech path.
func greetingAppend(model, text string) map[string]any {
	if strings.EqualFold(strings.TrimSpace(model), "gpt-live-1") {
		return map[string]any{"type": "session.commentary.append", "delegation_id": nil, "content": text}
	}
	return map[string]any{"type": "session.context.append", "channel": "speakable", "content": []any{map[string]any{"type": "input_text", "text": text}}}
}

func GreetingForLanguage(language string) string {
	switch strings.ToLower(strings.Split(strings.ReplaceAll(language, "_", "-"), "-")[0]) {
	case "de":
		return "Grüß mich kurz auf Deutsch."
	case "it":
		return "Salutami brevemente in italiano."
	case "vi":
		return "Chào tôi ngắn gọn bằng tiếng Việt."
	default:
		return "Greet me briefly in English."
	}
}
