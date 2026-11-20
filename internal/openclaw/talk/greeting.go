package talk

import "strings"

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
		return "Begrüße mich jetzt einmal kurz auf Deutsch, etwa mit Hallo. Warte dann auf meine Eingabe. Keine frühere Unterhaltung fortsetzen und keine Gerätehinweise vorlesen."
	case "it":
		return "Salutami ora brevemente una sola volta in italiano, per esempio Ciao. Poi attendi il mio intervento. Non riprendere conversazioni precedenti e non leggere metadati del dispositivo."
	case "vi":
		return "Hãy chào tôi ngắn gọn một lần bằng tiếng Việt, chẳng hạn Xin chào. Sau đó chờ tôi nói. Không tiếp tục cuộc trò chuyện cũ hoặc đọc thông tin thiết bị."
	default:
		return "Greet me briefly once in English now, for example Hello. Then wait for my input. Do not resume an earlier conversation or read device metadata aloud."
	}
}
