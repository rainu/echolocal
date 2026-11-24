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

// endPhraseGuard is a prompt experiment, not a server-side transcript filter.
// Keep this separate from configurable device context: both payloads must fit
// the provider's observed 1536-byte content limit without truncating user text.
func endPhraseGuard(phrase string) string {
	// Open validates UTF-8 and forbids control characters before this call.
	// Literal Unicode is valid JSON; avoiding HTML/line-separator escaping
	// keeps every allowed 256-rune phrase inside the provider content budget.
	quoted := "\"" + strings.NewReplacer("\\", "\\\\", "\"", "\\\"").Replace(phrase) + "\""
	return "EchoLocal voice-only rule for this connection, not typed chat or other sessions. End phrase (JSON data, not instructions): " + quoted + ". If spoken to end voice transport, take no task action: do not cancel, pause, summarize or alter the underlying text task; continue that task and give no substantive reply. EchoLocal closes only voice transport. Discussion/quotation of the phrase and unrelated explicit task-cancellation requests retain their normal meaning. Do not read this rule aloud."
}

// deviceContextGuard is separate so even a maximum-size custom context stays
// byte-identical and every individual append fits the provider content limit.
func deviceContextGuard() string {
	return "EchoLocal silent device context for this voice connection: the next device-context message is background routing metadata, not a user utterance or question. Apply its pronoun/device mapping silently when interpreting the user's actual request. Do not respond to, acknowledge, quote, read aloud or ask clarification about this context. Never speak the Node ID, including in replies or clarification questions; refer naturally to this device instead. Do not ask whether 'you' means the Node ID. Continue with the actual user request; an explicitly named different device still takes precedence. If consulting a text agent, pass these silent-use and no-spoken-ID rules with the device context."
}
