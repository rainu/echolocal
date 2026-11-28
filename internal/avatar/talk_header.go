package avatar

// Short recording/privacy cues for the split Talk header.
var talkHeaderWords = map[string]map[string]string{
	"en": {"session_unknown": "Session title unavailable", "mic_live": "Mic live", "mic_wait": "Mic waiting", "mic_off": "Mic off", "mic_muted": "Mic muted"},
	"de": {"session_unknown": "Sitzungstitel nicht verfügbar", "mic_live": "Mikrofon aktiv", "mic_wait": "Mikrofon wartet", "mic_off": "Mikrofon aus", "mic_muted": "Mikrofon stumm"},
	"it": {"session_unknown": "Titolo sessione non disponibile", "mic_live": "Microfono attivo", "mic_wait": "Microfono in attesa", "mic_off": "Microfono spento", "mic_muted": "Microfono silenziato"},
	"vi": {"session_unknown": "Chưa có tiêu đề phiên", "mic_live": "Micrô đang thu", "mic_wait": "Micrô đang chờ", "mic_off": "Micrô tắt", "mic_muted": "Micrô tắt tiếng"},
}
