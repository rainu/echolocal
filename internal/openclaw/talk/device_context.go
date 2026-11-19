package talk

import "strings"

// Match the installed OpenAI provider's public Live versus legacy native wire
// contract. This adds context, never session.update/replacement instructions.
func deviceContextAppend(model, text string) map[string]any {
	if strings.EqualFold(strings.TrimSpace(model), "gpt-live-1") {
		return map[string]any{"type": "session.thinking.append", "delegation_id": nil, "content": text}
	}
	return map[string]any{"type": "session.context.append", "channel": "commentary", "content": []any{map[string]any{"type": "input_text", "text": text}}}
}
