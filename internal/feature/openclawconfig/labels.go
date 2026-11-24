//go:build !board_biscuit

package openclawconfig

import "github.com/ygelfand/libcountertop/pkg/say"

// Labels follow the device language; never translate canonical stored values.
func Labels() map[string]string {
	out := map[string]string{}
	for key, id := range map[string]string{
		"deviceSession": "clawtalk.device_context", "confirmHistory": "clawtalk.delete_hint", "wakeWord": "assistants.word", "stopTalk": "clawtalk.end",
		"title": "openclaw.title", "connection": "openclaw.connection", "endpoint": "openclaw.endpoint.label", "token": "openclaw.token.label",
		"save": "openclaw.action.save", "connect": "openclaw.action.connect", "disconnect": "openclaw.action.disconnect", "pair_again": "openclaw.action.pair_again", "forget": "openclaw.action.forget",
		"saved": "openclaw.notice.saved", "agent": "clawtalk.agent", "model": "clawtalk.model", "context": "clawtalk.context", "resume": "clawtalk.resume", "resumeHint": "clawtalk.resume_hint",
		"newContext": "clawtalk.new_context", "default": "clawtalk.gateway_default", "startTone": "clawtalk.start_tone", "endTone": "clawtalk.end_tone", "timeout": "clawtalk.timeout",
		"preview": "clawtalk.tone_preview", "history": "clawtalk.history", "historyHint": "clawtalk.history_local", "empty": "clawtalk.caption_empty", "deleteAll": "clawtalk.delete_all", "deleteOne": "clawtalk.delete_one",
		"target": "clawtalk.target", "modelHint": "clawtalk.model_hint",
	} {
		out[key] = say.T(id)
	}
	keys := []string{"refresh", "permissions", "operator", "node", "requested", "granted", "permissionsHint", "tokenHint", "clearToken", "uiName", "nodeName", "assistant", "session", "loadChoices", "progressTone", "ending", "phrase", "position", "timing", "contains", "start", "end", "turn_end", "immediate", "greeting", "devicePrompt", "restoreDefaults", "confirm", "error", "diagnostics", "clearEvents", "nextConversation", "seconds", "promptHint"}
	values := map[string][]string{
		"en": {"Refresh", "Permissions", "Operator permissions", "Node permissions", "Requested", "Granted on active connection", "Permission changes reconnect. Upgrades may require approval at the Gateway. Reductions do not revoke previously stored Gateway authorizations.", "Leave blank to keep the stored credential. Enter a replacement or explicitly clear it. No stored secret is returned.", "Clear stored shared credential", "Controller UI device name", "Node device name", "Assistant", "Session", "Load choices", "Connection tone", "End conversation", "End phrase", "Match position", "Evaluate", "Contains", "Start", "End", "End of turn", "Immediately", "Greeting prompt", "Device context prompt", "Restore defaults", "Confirm this operation? Pair again/forget removes local authorization; Gateway approvals remain.", "Could not apply settings. Check the input, connection and permissions, then refresh.", "Diagnostics", "Clear diagnostics", "Applies to the next conversation", "Seconds", "Empty selects the localized default. Use the listed placeholders only; preview validates the expanded 1536-byte limit."},
		"de": {"Aktualisieren", "Berechtigungen", "Operator-Berechtigungen", "Node-Berechtigungen", "Angefordert", "Auf aktiver Verbindung gewährt", "Berechtigungsänderungen verbinden neu. Erweiterungen brauchen ggf. Gateway-Freigabe. Einschränkungen widerrufen keine zuvor gespeicherten Gateway-Freigaben.", "Leer lassen, um den gespeicherten Zugang zu behalten. Zum Ersetzen einen neuen Wert eingeben oder ausdrücklich löschen. Gespeicherte Secrets werden nie zurückgegeben.", "Gespeicherten gemeinsamen Zugang löschen", "Gerätename Controller UI", "Gerätename Node", "Assistent", "Sitzung", "Auswahl laden", "Verbindungston", "Gespräch beenden", "Abschlussphrase", "Trefferposition", "Auswertung", "Enthält", "Am Anfang", "Am Ende", "Am Turn-Ende", "Sofort", "Begrüßungsprompt", "Gerätekontext-Prompt", "Standard wiederherstellen", "Vorgang bestätigen? Erneutes Koppeln/Vergessen entfernt lokale Autorisierung; Gateway-Freigaben bleiben bestehen.", "Einstellungen konnten nicht angewendet werden. Eingabe, Verbindung und Berechtigungen prüfen und aktualisieren.", "Diagnose", "Diagnose leeren", "Gilt ab dem nächsten Gespräch", "Sekunden", "Leer verwendet den lokalisierten Standard. Nur aufgeführte Platzhalter verwenden; Vorschau prüft die Grenze von 1536 ausgefüllten Bytes."},
		"it": {"Aggiorna", "Autorizzazioni", "Autorizzazioni operatore", "Autorizzazioni Node", "Richieste", "Concesse sulla connessione attiva", "Le modifiche riconnettono. Gli ampliamenti possono richiedere approvazione Gateway. Le riduzioni non revocano le autorizzazioni Gateway memorizzate.", "Lasciare vuoto per mantenere la credenziale. Inserire un nuovo valore o cancellarla esplicitamente. I segreti salvati non vengono restituiti.", "Cancella credenziale condivisa", "Nome dispositivo Controller UI", "Nome dispositivo Node", "Assistente", "Sessione", "Carica opzioni", "Tono di connessione", "Termina conversazione", "Frase di chiusura", "Posizione", "Valutazione", "Contiene", "Inizio", "Fine", "Fine turno", "Subito", "Prompt di saluto", "Prompt contesto dispositivo", "Ripristina predefiniti", "Confermare? Associare di nuovo/dimenticare rimuove l'autorizzazione locale; le approvazioni Gateway restano.", "Impossibile applicare le impostazioni. Verificare valori, connessione e autorizzazioni, quindi aggiornare.", "Diagnostica", "Cancella diagnostica", "Dalla prossima conversazione", "Secondi", "Vuoto usa il predefinito localizzato. Usare solo i segnaposto elencati; l'anteprima verifica il limite di 1536 byte espansi."},
		"vi": {"Làm mới", "Quyền truy cập", "Quyền Operator", "Quyền Node", "Yêu cầu", "Được cấp trên kết nối hiện tại", "Thay đổi sẽ kết nối lại. Quyền mở rộng có thể cần Gateway phê duyệt. Giảm quyền không thu hồi phê duyệt Gateway đã lưu.", "Để trống để giữ thông tin xác thực. Nhập giá trị mới hoặc xóa rõ ràng. Không trả về bí mật đã lưu.", "Xóa thông tin xác thực dùng chung", "Tên thiết bị Controller UI", "Tên thiết bị Node", "Trợ lý", "Phiên", "Tải lựa chọn", "Âm kết nối", "Kết thúc cuộc trò chuyện", "Cụm từ kết thúc", "Vị trí khớp", "Thời điểm kiểm tra", "Có chứa", "Đầu câu", "Cuối câu", "Cuối lượt", "Ngay lập tức", "Lời nhắc chào hỏi", "Lời nhắc ngữ cảnh thiết bị", "Khôi phục mặc định", "Xác nhận? Ghép nối lại/quên sẽ xóa quyền cục bộ; phê duyệt Gateway vẫn còn.", "Không thể áp dụng cài đặt. Kiểm tra dữ liệu, kết nối và quyền, rồi làm mới.", "Chẩn đoán", "Xóa chẩn đoán", "Áp dụng cho cuộc trò chuyện tiếp theo", "Giây", "Để trống dùng mặc định theo ngôn ngữ. Chỉ dùng biến được liệt kê; xem trước kiểm tra giới hạn 1536 byte."},
	}
	v := values[say.Chosen()]
	if v == nil {
		v = values["en"]
	}
	for i, key := range keys {
		out[key] = v[i]
	}
	for _, state := range []string{"connected", "connecting", "reconnecting", "pairing", "error", "disconnected"} {
		out["state."+state] = say.T("openclaw.state." + state)
	}
	groups := []string{"screen", "input", "display", "settings", "audio", "alarms", "intercom", "camera", "state", "diagnostics", "maintenance", "tones", "talk"}
	names := map[string][]string{
		"en": {"Screenshots", "Remote input", "Display and messages", "Non-secret settings", "Audio and media", "Alarms and timers", "Calls and broadcasts", "Camera", "Device state", "Diagnostics", "Maintenance", "Custom tones", "Talk"},
		"de": {"Bildschirmaufnahmen", "Fernbedienung", "Anzeige und Nachrichten", "Nicht geheime Einstellungen", "Audio und Medien", "Wecker und Timer", "Anrufe und Durchsagen", "Kamera", "Gerätezustand", "Diagnose", "Wartung", "Eigene Töne", "Sprachmodus"},
		"it": {"Schermate", "Input remoto", "Display e messaggi", "Impostazioni non segrete", "Audio e media", "Sveglie e timer", "Chiamate e annunci", "Fotocamera", "Stato dispositivo", "Diagnostica", "Manutenzione", "Toni personalizzati", "Voce"},
		"vi": {"Ảnh màn hình", "Điều khiển từ xa", "Hiển thị và thông báo", "Cài đặt không bí mật", "Âm thanh và đa phương tiện", "Báo thức và hẹn giờ", "Cuộc gọi và phát thông báo", "Camera", "Trạng thái thiết bị", "Chẩn đoán", "Bảo trì", "Âm tùy chỉnh", "Giọng nói"},
	}
	groupNames := names[say.Chosen()]
	if groupNames == nil {
		groupNames = names["en"]
	}
	for i, g := range groups {
		out["group."+g] = groupNames[i]
	}
	desktopNames := map[string]string{"en": "Live screen sharing (default off)", "de": "Live-Bildschirmfreigabe (standardmäßig aus)", "it": "Condivisione schermo live (disattivata)", "vi": "Chia sẻ màn hình trực tiếp (mặc định tắt)"}
	out["group.desktop"] = desktopNames[say.Chosen()]
	if out["group.desktop"] == "" {
		out["group.desktop"] = desktopNames["en"]
	}
	scopeKeys := []string{"operator.admin", "operator.read", "operator.write", "operator.sessions.read", "operator.sessions.write", "operator.approvals", "operator.questions", "operator.pairing", "operator.talk", "operator.talk.secrets"}
	scopeNames := map[string][]string{
		"en": {"Administration", "Read Gateway data", "Perform actions", "Read sessions", "Modify sessions", "Manage approvals", "Answer questions", "Manage device pairing", "Use Talk", "Access Talk provider credentials"},
		"de": {"Administration", "Gateway-Daten lesen", "Aktionen ausführen", "Sitzungen lesen", "Sitzungen ändern", "Freigaben verwalten", "Fragen beantworten", "Gerätekopplung verwalten", "Sprachmodus verwenden", "Sprachanbieter-Zugangsdaten abrufen"},
		"it": {"Amministrazione", "Leggi dati Gateway", "Esegui azioni", "Leggi sessioni", "Modifica sessioni", "Gestisci approvazioni", "Rispondi alle domande", "Gestisci associazioni dispositivi", "Usa voce", "Accedi alle credenziali del fornitore vocale"},
		"vi": {"Quản trị", "Đọc dữ liệu Gateway", "Thực hiện hành động", "Đọc phiên", "Sửa phiên", "Quản lý phê duyệt", "Trả lời câu hỏi", "Quản lý ghép nối thiết bị", "Dùng giọng nói", "Truy cập thông tin xác thực nhà cung cấp giọng nói"},
	}
	scopes := scopeNames[say.Chosen()]
	if scopes == nil {
		scopes = scopeNames["en"]
	}
	for i, key := range scopeKeys {
		out["scope."+key] = scopes[i]
	}
	promptHints := map[string]string{
		"en": "Empty selects the localized default. Device context expands the listed placeholders; the greeting is plain text. Maximum 1536 UTF-8 bytes.",
		"de": "Leer verwendet den lokalisierten Standard. Gerätekontext ersetzt die aufgeführten Platzhalter; die Begrüßung ist reiner Text. Maximal 1536 UTF-8-Bytes.",
		"it": "Vuoto usa il predefinito localizzato. Il contesto espande i segnaposto elencati; il saluto è testo semplice. Massimo 1536 byte UTF-8.",
		"vi": "Để trống dùng mặc định theo ngôn ngữ. Ngữ cảnh thay thế các biến được liệt kê; lời chào là văn bản thuần. Tối đa 1536 byte UTF-8.",
	}
	out["promptHint"] = promptHints[say.Chosen()]
	if out["promptHint"] == "" {
		out["promptHint"] = promptHints["en"]
	}

	for _, code := range []string{"invalid_endpoint", "credentials_missing", "approval_required", "authentication", "tls", "dns", "timeout", "network", "unavailable", "protocol", "identity", "storage", "settings"} {
		out["error."+code] = say.T("openclaw.error." + code)
	}
	out["error.invalid_name"] = out["error"] + " (80)"
	out["error.talk_timeout_invalid"] = out["timeout"] + ": 15–900 " + out["seconds"]
	out["error.selection_unavailable"] = say.T("clawtalk.unavailable") + " · " + out["connect"]
	out["error.talk_target_missing"] = say.T("clawtalk.select_agent")
	out["error.talk_target_mismatch"] = out["agent"] + " / " + out["session"] + " · " + out["loadChoices"]
	for _, code := range []string{"talk_context_too_long", "talk_context_placeholder", "talk_context_invalid", "talk_greeting_too_long", "talk_greeting_invalid"} {
		out["error."+code] = out["promptHint"]
	}
	for _, code := range []string{"talk_end_phrase_invalid", "talk_end_position_invalid", "talk_end_timing_invalid"} {
		out["error."+code] = out["ending"] + " · " + out["error"]
	}
	return out
}
