package avatar

// Text covers all UI languages supported by EchoLocal. Error text never includes
// model contents, arbitrary process output, or a potentially sensitive path.
func Text(lang, key string) string {
	if words := turnWords[lang]; words != nil && words[key] != "" {
		return words[key]
	}
	if value := turnWords["en"][key]; value != "" {
		return value
	}
	if words := placementWords[lang]; words != nil && words[key] != "" {
		return words[key]
	}
	if value := placementWords["en"][key]; value != "" {
		return value
	}
	if words := talkHeaderWords[lang]; words != nil && words[key] != "" {
		return words[key]
	}
	if value := talkHeaderWords["en"][key]; value != "" {
		return value
	}
	if key == "background_video_unsupported" || key == "foreground_video_unsupported" {
		return videoHelp(lang)
	}

	if key == "background_unsupported" || key == "foreground_unsupported" ||
		key == "background_animation_limit" || key == "foreground_animation_limit" {
		return sceneHelp(lang)
	}
	words := map[string]map[string]string{
		"en": {"title": "Avatar Lab", "path": "Custom VRM — path on Echo, not this computer (empty = default)", "model": "Model file", "default": "Use default avatar", "help": "Upload a .vrm using Files or WebDAV, then enter its full device path. Changes apply on the next Start. Empty restores the installed default.", "invalid_path": "Use an absolute device path, at most 255 bytes, without control characters or ../ components.", "custom_unavailable": "Cannot read custom VRM. Check the device path and file permissions, select another file or reset to default. No fallback was loaded.", "custom_invalid": "Custom model rejected. Use a self-contained VRM 0.x/1.0 GLB below 64 MiB; select another file or reset. No fallback was loaded.", "custom_storage": "Cannot stage the model. Free device storage and retry, or reset to default.", "model_invalid": "Renderer rejected the model or its resource budget. Select a compatible model or reset to default.", "starting": "Loading selected avatar…", "selection": "Selected file applies on next Start; the name above identifies the loaded model."},
		"de": {"title": "Avatar-Labor", "path": "Eigenes VRM — Pfad auf dem Echo, nicht auf diesem Computer (leer = Standard)", "model": "Modelldatei", "default": "Standardavatar verwenden", "help": "Eine .vrm-Datei über Dateien oder WebDAV hochladen, dann ihren vollständigen Gerätepfad eingeben. Änderungen gelten beim nächsten Start. Leer stellt den installierten Standard wieder her.", "invalid_path": "Absoluten Gerätepfad mit maximal 255 Bytes eingeben, ohne Steuerzeichen oder ../-Bestandteile.", "custom_unavailable": "Eigenes VRM nicht lesbar. Gerätepfad und Dateirechte prüfen, andere Datei wählen oder Standard wiederherstellen. Kein Ersatzmodell geladen.", "custom_invalid": "Eigenes Modell abgelehnt. Eigenständiges VRM-0.x/1.0-GLB unter 64 MiB verwenden; andere Datei wählen oder Standard wiederherstellen. Kein Ersatzmodell geladen.", "custom_storage": "Modell kann nicht vorbereitet werden. Gerätespeicher freigeben und erneut versuchen oder Standard wählen.", "model_invalid": "Renderer lehnt Modell oder Ressourcenbedarf ab. Kompatibles Modell wählen oder Standard wiederherstellen.", "starting": "Ausgewählter Avatar wird geladen…", "selection": "Dateiauswahl gilt beim nächsten Start; der Name oben bezeichnet das geladene Modell."},
		"it": {"title": "Laboratorio avatar", "path": "VRM personale — percorso su Echo, non su questo computer (vuoto = predefinito)", "model": "File del modello", "default": "Usa avatar predefinito", "help": "Caricare un file .vrm tramite File o WebDAV e inserirne il percorso completo sul dispositivo. Le modifiche valgono al prossimo avvio. Vuoto ripristina il modello installato.", "invalid_path": "Usare un percorso assoluto sul dispositivo di massimo 255 byte, senza caratteri di controllo o componenti ../.", "custom_unavailable": "VRM personale non leggibile. Controllare percorso e permessi, scegliere un altro file o ripristinare il predefinito. Nessun modello sostitutivo caricato.", "custom_invalid": "Modello rifiutato. Usare un GLB VRM 0.x/1.0 autonomo inferiore a 64 MiB; scegliere un altro file o ripristinare. Nessun modello sostitutivo caricato.", "custom_storage": "Impossibile preparare il modello. Liberare spazio sul dispositivo e riprovare o ripristinare.", "model_invalid": "Il renderer rifiuta il modello o il consumo di risorse. Scegliere un modello compatibile o ripristinare.", "starting": "Caricamento dell’avatar selezionato…", "selection": "Il file selezionato vale al prossimo avvio; il nome sopra identifica il modello caricato."},
		"vi": {"title": "Phòng thử avatar", "path": "VRM riêng — đường dẫn trên Echo, không phải máy tính này (trống = mặc định)", "model": "Tệp mô hình", "default": "Dùng avatar mặc định", "help": "Tải tệp .vrm lên bằng Tệp hoặc WebDAV, rồi nhập đường dẫn đầy đủ trên thiết bị. Thay đổi áp dụng ở lần Bắt đầu tiếp theo. Để trống để dùng mô hình mặc định đã cài.", "invalid_path": "Dùng đường dẫn tuyệt đối trên thiết bị, tối đa 255 byte, không có ký tự điều khiển hoặc thành phần ../.", "custom_unavailable": "Không đọc được VRM riêng. Kiểm tra đường dẫn và quyền tệp, chọn tệp khác hoặc khôi phục mặc định. Không nạp mô hình thay thế.", "custom_invalid": "Mô hình bị từ chối. Dùng GLB VRM 0.x/1.0 độc lập dưới 64 MiB; chọn tệp khác hoặc khôi phục. Không nạp mô hình thay thế.", "custom_storage": "Không thể chuẩn bị mô hình. Giải phóng bộ nhớ thiết bị và thử lại hoặc khôi phục mặc định.", "model_invalid": "Trình dựng từ chối mô hình hoặc mức dùng tài nguyên. Chọn mô hình tương thích hoặc khôi phục mặc định.", "starting": "Đang nạp avatar đã chọn…", "selection": "Tệp đã chọn áp dụng ở lần Bắt đầu tiếp theo; tên bên trên là mô hình đang nạp."},
	}
	if motionWords[lang] == nil {
		lang = "en"
	}
	if v := motionWords[lang][key]; v != "" {
		return v
	}
	if v := activityWords[lang][key]; v != "" {
		return v
	}
	if v := fullscreenWords[lang][key]; v != "" {
		return v
	}
	if words[lang] == nil {
		lang = "en"
	}
	if v := mouthWords[lang][key]; v != "" {
		return v
	}
	if v := foregroundWords[lang][key]; v != "" {
		if key == "foreground_help" || key == "foreground_invalid" {
			return v + " " + sceneHelp(lang)
		}
		return v
	}
	if v := backgroundWords[lang][key]; v != "" {
		if key == "background_help" || key == "background_invalid" {
			return v + " " + sceneHelp(lang) + " " + videoHelp(lang)
		}
		return v
	}
	if v := presentationWords[lang][key]; v != "" {
		return v
	}
	return words[lang][key]
}

var presentationWords = map[string]map[string]string{
	"en": {
		"idle": "Idle movement", "blink": "Blink", "animation": "VRMA animation", "idle_help": "Enable idle movement. Saved for Avatar Lab and standalone Talk.", "blink_help": "Enable automatic blinking. Saved for Avatar Lab and standalone Talk.", "animation_help": "Play the installed VRMA motion. Saved for Avatar Lab and standalone Talk; agent speech mouth movement stays independent.", "toggle_invalid": "Choose on or off.",
		"presentation": "Presentation", "save_presentation": "Save presentation", "reset_presentation": "Reset presentation", "saved": "Presentation saved for Talk.", "save_failed": "Could not save presentation. Retry.", "presentation_invalid": "Enter a whole number within the displayed range.",
		"zoom": "Zoom (%)", "yaw": "Horizontal rotation (°)", "pitch": "Vertical rotation (°)", "x": "Horizontal position (%)", "y": "Vertical position (%)",
		"zoom_help": "50–999%; 100 = fitted view. Shared by Avatar Lab and standalone Talk.", "yaw_help": "−180–180° around the model center; 0 = front.", "pitch_help": "−85–85° around the model center; 0 = upright.", "x_help": "−500–500% of viewport width; positive moves right.", "y_help": "−500–500% of viewport width; positive moves down.",
		"talk_fallback": "Avatar unavailable. Talk still works. Stop, then correct the model or reset presentation in Avatar Lab.",
		"talk_loading":  "Loading avatar; audio connection continues independently.", "talk_running": "Avatar follows agent playback, not your microphone.",
		"renderer_failed": "Avatar renderer stopped unexpectedly.", "renderer_timeout": "Avatar renderer did not become ready.", "missing_renderer": "Avatar renderer is missing.", "assets_unavailable": "No default avatar is installed.", "start_failed": "Avatar renderer could not start.", "gpu_failed": "Avatar graphics are unavailable.", "protocol_error": "Avatar renderer protocol mismatch.", "timeout": "Avatar time limit reached.", "finished": "Avatar stopped.", "stopping": "Stopping avatar…", "ready": "Avatar ready."},
	"de": {
		"idle": "Idle-Bewegung", "blink": "Blinzeln", "animation": "VRMA-Animation", "idle_help": "Idle-Bewegung aktivieren. Gespeichert für Avatar-Labor und eigenständiges Talk.", "blink_help": "Automatisches Blinzeln aktivieren. Gespeichert für Avatar-Labor und eigenständiges Talk.", "animation_help": "Installierte VRMA-Bewegung abspielen. Gespeichert für Avatar-Labor und eigenständiges Talk; die Mundbewegung zur Agentensprache bleibt unabhängig.", "toggle_invalid": "Ein oder Aus wählen.",
		"presentation": "Darstellung", "save_presentation": "Darstellung speichern", "reset_presentation": "Darstellung zurücksetzen", "saved": "Darstellung für Talk gespeichert.", "save_failed": "Darstellung nicht gespeichert. Erneut versuchen.", "presentation_invalid": "Ganze Zahl im angezeigten Bereich eingeben.",
		"zoom": "Zoom (%)", "yaw": "Horizontale Drehung (°)", "pitch": "Vertikale Drehung (°)", "x": "Horizontale Position (%)", "y": "Vertikale Position (%)",
		"zoom_help": "50–999%; 100 = eingepasste Ansicht. Gemeinsam für Avatar-Labor und eigenständiges Talk.", "yaw_help": "−180–180° um die Modellmitte; 0 = vorne.", "pitch_help": "−85–85° um die Modellmitte; 0 = aufrecht.", "x_help": "−500–500% der Ansichtsbreite; positiv bewegt nach rechts.", "y_help": "−500–500% der Ansichtsbreite; positiv bewegt nach unten.",
		"talk_fallback": "Avatar nicht verfügbar. Talk funktioniert weiter. Beenden, dann Modell korrigieren oder Darstellung im Avatar-Labor zurücksetzen.",
		"talk_loading":  "Avatar wird geladen; Audioverbindung läuft unabhängig weiter.", "talk_running": "Avatar folgt der Agentenwiedergabe, nicht deinem Mikrofon.",
		"renderer_failed": "Avatar-Renderer unerwartet beendet.", "renderer_timeout": "Avatar-Renderer wurde nicht bereit.", "missing_renderer": "Avatar-Renderer fehlt.", "assets_unavailable": "Kein Standardavatar installiert.", "start_failed": "Avatar-Renderer konnte nicht starten.", "gpu_failed": "Avatar-Grafik nicht verfügbar.", "protocol_error": "Avatar-Renderer inkompatibel.", "timeout": "Avatar-Zeitlimit erreicht.", "finished": "Avatar beendet.", "stopping": "Avatar wird beendet…", "ready": "Avatar bereit."},
	"it": {
		"idle": "Movimento a riposo", "blink": "Battito delle palpebre", "animation": "Animazione VRMA", "idle_help": "Attiva il movimento a riposo. Salvato per laboratorio e Talk autonomo.", "blink_help": "Attiva il battito automatico. Salvato per laboratorio e Talk autonomo.", "animation_help": "Riproduce il movimento VRMA installato. Salvato per laboratorio e Talk autonomo; la bocca segue la voce indipendentemente.", "toggle_invalid": "Scegliere acceso o spento.",
		"presentation": "Presentazione", "save_presentation": "Salva presentazione", "reset_presentation": "Ripristina presentazione", "saved": "Presentazione salvata per Talk.", "save_failed": "Salvataggio non riuscito. Riprovare.", "presentation_invalid": "Inserire un numero intero nell’intervallo indicato.",
		"zoom": "Zoom (%)", "yaw": "Rotazione orizzontale (°)", "pitch": "Rotazione verticale (°)", "x": "Posizione orizzontale (%)", "y": "Posizione verticale (%)",
		"zoom_help": "50–999%; 100 = vista adattata. Condivisa tra laboratorio e Talk autonomo.", "yaw_help": "−180–180° intorno al centro; 0 = frontale.", "pitch_help": "−85–85° intorno al centro; 0 = verticale.", "x_help": "−500–500% della larghezza; positivo sposta a destra.", "y_help": "−500–500% della larghezza; positivo sposta in basso.",
		"talk_fallback": "Avatar non disponibile. Talk funziona ancora. Terminare, poi correggere il modello o ripristinare la presentazione nel laboratorio.",
		"talk_loading":  "Caricamento avatar; la connessione audio prosegue indipendentemente.", "talk_running": "L’avatar segue la voce dell’agente, non il microfono.",
		"renderer_failed": "Renderer avatar terminato inaspettatamente.", "renderer_timeout": "Renderer avatar non pronto.", "missing_renderer": "Renderer avatar mancante.", "assets_unavailable": "Nessun avatar predefinito installato.", "start_failed": "Impossibile avviare il renderer avatar.", "gpu_failed": "Grafica avatar non disponibile.", "protocol_error": "Protocollo renderer avatar incompatibile.", "timeout": "Limite di tempo avatar raggiunto.", "finished": "Avatar fermato.", "stopping": "Arresto avatar…", "ready": "Avatar pronto."},
	"vi": {
		"idle": "Chuyển động nghỉ", "blink": "Chớp mắt", "animation": "Hoạt ảnh VRMA", "idle_help": "Bật chuyển động nghỉ. Lưu cho phòng thử và Talk độc lập.", "blink_help": "Bật chớp mắt tự động. Lưu cho phòng thử và Talk độc lập.", "animation_help": "Phát chuyển động VRMA đã cài. Lưu cho phòng thử và Talk độc lập; chuyển động miệng theo tiếng trợ lý vẫn độc lập.", "toggle_invalid": "Chọn bật hoặc tắt.",
		"presentation": "Cách hiển thị", "save_presentation": "Lưu cách hiển thị", "reset_presentation": "Đặt lại cách hiển thị", "saved": "Đã lưu cách hiển thị cho Talk.", "save_failed": "Không lưu được. Hãy thử lại.", "presentation_invalid": "Nhập số nguyên trong phạm vi hiển thị.",
		"zoom": "Thu phóng (%)", "yaw": "Xoay ngang (°)", "pitch": "Xoay dọc (°)", "x": "Vị trí ngang (%)", "y": "Vị trí dọc (%)",
		"zoom_help": "50–999%; 100 = vừa khung. Dùng chung cho phòng thử và Talk độc lập.", "yaw_help": "−180–180° quanh tâm mô hình; 0 = phía trước.", "pitch_help": "−85–85° quanh tâm mô hình; 0 = thẳng đứng.", "x_help": "−500–500% chiều rộng khung; dương là sang phải.", "y_help": "−500–500% chiều rộng khung; dương là xuống dưới.",
		"talk_fallback": "Avatar không khả dụng. Talk vẫn hoạt động. Dừng, rồi sửa mô hình hoặc đặt lại cách hiển thị trong phòng thử.",
		"talk_loading":  "Đang nạp avatar; kết nối âm thanh tiếp tục độc lập.", "talk_running": "Avatar theo tiếng phát của trợ lý, không theo micrô.",
		"renderer_failed": "Trình dựng avatar dừng bất ngờ.", "renderer_timeout": "Trình dựng avatar chưa sẵn sàng.", "missing_renderer": "Thiếu trình dựng avatar.", "assets_unavailable": "Chưa cài avatar mặc định.", "start_failed": "Không khởi động được trình dựng avatar.", "gpu_failed": "Đồ họa avatar không khả dụng.", "protocol_error": "Giao thức trình dựng avatar không tương thích.", "timeout": "Đã hết thời gian avatar.", "finished": "Avatar đã dừng.", "stopping": "Đang dừng avatar…", "ready": "Avatar sẵn sàng."}}

var backgroundWords = map[string]map[string]string{
	"en": {
		"background":             "Background image / video",
		"background_help":        "Upload a PNG or JPEG using Files or WebDAV and enter its full path on the Echo, not this computer. Maximum 8 MiB, 4096 pixels per side and 4 megapixels decoded. Centered cover: preserve aspect ratio, crop edges to fill the avatar panel. Transparent pixels show the default. Applied on next Start; framing is unchanged.",
		"background_reset":       "Restore default background",
		"background_example":     "/data/local/tmp/background.png",
		"fullscreen":             "Fullscreen preview",
		"preview_only":           "Visual only — no Talk, call, messages or microphone. The right panel uses the exact conversation framing. Exit restores your lab controls; Save presentation to keep placement for Talk.",
		"preview_exit":           "Exit preview",
		"background_invalid":     "Image rejected. Use a complete PNG/JPEG within 8 MiB, 4096 pixels per side and 4 megapixels. The default background is used.",
		"background_unavailable": "Cannot read background. Check the full device path and file permissions, select another file or clear the setting. The default background is used.",
		"background_storage":     "Cannot prepare background. Free device storage and retry. The default background is used.",
	},
	"de": {
		"background":             "Hintergrundbild / Video",
		"background_help":        "PNG oder JPEG über Dateien oder WebDAV hochladen und den vollständigen Pfad auf dem Echo eingeben, nicht auf diesem Computer. Maximal 8 MiB, 4096 Pixel je Seite und 4 Megapixel dekodiert. Zentriert füllend: Seitenverhältnis bleibt erhalten, Ränder werden zugeschnitten. Transparente Pixel zeigen den Standard. Gilt beim nächsten Start; die Platzierung bleibt unverändert.",
		"background_reset":       "Standardhintergrund wiederherstellen",
		"background_example":     "/data/local/tmp/hintergrund.png",
		"fullscreen":             "Vollbildvorschau",
		"preview_only":           "Nur visuell – kein Talk, Anruf, Nachrichten oder Mikrofon. Rechts ist exakt der spätere Gesprächsausschnitt. Beenden stellt die Laborsteuerung wieder her; Darstellung speichern übernimmt die Platzierung für Talk.",
		"preview_exit":           "Vorschau beenden",
		"background_invalid":     "Bild abgelehnt. Vollständiges PNG/JPEG bis 8 MiB, 4096 Pixel je Seite und 4 Megapixel verwenden. Der Standardhintergrund wird verwendet.",
		"background_unavailable": "Hintergrund nicht lesbar. Vollständigen Gerätepfad und Dateirechte prüfen, andere Datei wählen oder Einstellung leeren. Der Standardhintergrund wird verwendet.",
		"background_storage":     "Hintergrund kann nicht vorbereitet werden. Gerätespeicher freigeben und erneut versuchen. Der Standardhintergrund wird verwendet.",
	},
	"it": {
		"background":             "Immagine di sfondo",
		"background_help":        "Caricare PNG o JPEG tramite File o WebDAV e inserire il percorso completo su Echo, non su questo computer. Massimo 8 MiB, 4096 pixel per lato e 4 megapixel decodificati. Riempimento centrato: proporzioni mantenute e bordi ritagliati. I pixel trasparenti mostrano lo sfondo predefinito. Applicato al prossimo avvio; posizione invariata.",
		"background_reset":       "Ripristina sfondo predefinito",
		"background_example":     "/data/local/tmp/sfondo.png",
		"fullscreen":             "Anteprima a schermo intero",
		"preview_only":           "Solo visiva: nessun Talk, chiamata, messaggio o microfono. Il pannello destro usa l’inquadratura esatta della conversazione. Uscire ripristina i controlli; salvare la presentazione conserva la posizione per Talk.",
		"preview_exit":           "Esci dall’anteprima",
		"background_invalid":     "Immagine rifiutata. Usare PNG/JPEG completo entro 8 MiB, 4096 pixel per lato e 4 megapixel. Viene usato lo sfondo predefinito.",
		"background_unavailable": "Sfondo non leggibile. Verificare percorso completo e permessi sul dispositivo, scegliere un altro file o cancellare l’impostazione. Viene usato lo sfondo predefinito.",
		"background_storage":     "Impossibile preparare lo sfondo. Liberare spazio sul dispositivo e riprovare. Viene usato lo sfondo predefinito.",
	},
	"vi": {
		"background":             "Ảnh nền",
		"background_help":        "Tải PNG hoặc JPEG lên bằng Tệp hoặc WebDAV rồi nhập đường dẫn đầy đủ trên Echo, không phải máy tính này. Tối đa 8 MiB, 4096 pixel mỗi cạnh và 4 megapixel sau giải mã. Lấp đầy ở giữa: giữ tỷ lệ, cắt mép để vừa khung avatar. Pixel trong suốt hiển thị nền mặc định. Áp dụng lần Bắt đầu tiếp theo; giữ nguyên vị trí.",
		"background_reset":       "Khôi phục nền mặc định",
		"background_example":     "/data/local/tmp/nen.png",
		"fullscreen":             "Xem trước toàn màn hình",
		"preview_only":           "Chỉ hình ảnh — không Talk, gọi, nhắn tin hoặc micrô. Khung bên phải dùng đúng bố cục cuộc trò chuyện. Thoát khôi phục điều khiển phòng thử; lưu cách hiển thị để giữ vị trí cho Talk.",
		"preview_exit":           "Thoát xem trước",
		"background_invalid":     "Ảnh bị từ chối. Dùng PNG/JPEG hoàn chỉnh tối đa 8 MiB, 4096 pixel mỗi cạnh và 4 megapixel. Sử dụng nền mặc định.",
		"background_unavailable": "Không đọc được nền. Kiểm tra đường dẫn đầy đủ và quyền tệp trên thiết bị, chọn tệp khác hoặc xóa cài đặt. Sử dụng nền mặc định.",
		"background_storage":     "Không chuẩn bị được nền. Giải phóng bộ nhớ thiết bị và thử lại. Sử dụng nền mặc định.",
	},
}

var mouthWords = map[string]map[string]string{
	"en": {"mouth_intensity": "Speech mouth intensity (%)", "mouth_intensity_help": "0–100%; default 50%. 0 = closed, 100 = original strength. Visual agent playback only; audio volume and microphone are unchanged.", "mouth_intensity_invalid": "Enter a whole number from 0 to 100.", "speech_preview": "Speech preview (silent)", "speech_preview_help": "Repeating mouth preview from a synthesized speech recording, with pauses. No sound, microphone or agent connection. Adjust live, then Save presentation for Talk.", "mouth_reset": "Reset mouth intensity (50%)"},
	"de": {"mouth_intensity": "Mundintensität bei Sprache (%)", "mouth_intensity_help": "0–100%; Standard 50%. 0 = geschlossen, 100 = bisherige Stärke. Nur visuelle Agentenwiedergabe; Lautstärke und Mikrofon bleiben unverändert.", "mouth_intensity_invalid": "Ganze Zahl von 0 bis 100 eingeben.", "speech_preview": "Sprachvorschau (lautlos)", "speech_preview_help": "Wiederholte Mundvorschau aus einer synthetischen Sprachaufnahme mit Pausen. Kein Ton, Mikrofon oder Agentenverbindung. Live anpassen, dann Darstellung für Talk speichern.", "mouth_reset": "Mundintensität zurücksetzen (50%)"},
	"it": {"mouth_intensity": "Intensità della bocca durante il parlato (%)", "mouth_intensity_help": "0–100%; predefinito 50%. 0 = chiusa, 100 = intensità originale. Solo voce dell’agente nell’avatar; volume e microfono invariati.", "mouth_intensity_invalid": "Inserire un numero intero da 0 a 100.", "speech_preview": "Anteprima parlato (silenziosa)", "speech_preview_help": "Anteprima ripetuta da una registrazione vocale sintetica, con pause. Nessun suono, microfono o connessione agente. Regolare dal vivo e salvare la presentazione per Talk.", "mouth_reset": "Ripristina intensità bocca (50%)"},
	"vi": {"mouth_intensity": "Cường độ miệng khi nói (%)", "mouth_intensity_help": "0–100%; mặc định 50%. 0 = đóng, 100 = cường độ ban đầu. Chỉ hình ảnh tiếng trợ lý; không đổi âm lượng hoặc micrô.", "mouth_intensity_invalid": "Nhập số nguyên từ 0 đến 100.", "speech_preview": "Xem trước lời nói (không tiếng)", "speech_preview_help": "Lặp chuyển động miệng từ bản ghi giọng tổng hợp có khoảng nghỉ. Không âm thanh, micrô hoặc kết nối trợ lý. Chỉnh trực tiếp rồi lưu cách hiển thị cho Talk.", "mouth_reset": "Đặt lại cường độ miệng (50%)"},
}

var fullscreenWords = map[string]map[string]string{
	"en": {"fullscreen_position_help": "−500–500% of the smaller fullscreen dimension; default 0. Positive X moves right, positive Y down. Uses the same square-reference composition as the Lab.", "fullscreen_mode": "Fullscreen", "split_screen": "Split-screen", "talk_mode": "Default Talk view", "talk_mode_help": "split_screen or fullscreen; default split_screen. Applies to the next wake-word conversation.", "fullscreen_background": "Fullscreen background", "switch_fullscreen": "Switch to fullscreen", "switch_split": "Switch to split-screen", "end_talk": "End conversation", "editing_mode": "Editing / previewing", "reset_fullscreen": "Reset fullscreen presentation"},
	"de": {"fullscreen_position_help": "−500–500 % der kleineren Vollbildseite; Standard 0. Positives X bewegt nach rechts, positives Y nach unten. Gleicher quadratischer Bezugsrahmen wie im Labor.", "fullscreen_mode": "Vollbild", "split_screen": "Geteilter Bildschirm", "talk_mode": "Standard-Talk-Ansicht", "talk_mode_help": "split_screen oder fullscreen; Standard split_screen. Gilt für das nächste Wake-Word-Gespräch.", "fullscreen_background": "Vollbild-Hintergrund", "switch_fullscreen": "Zu Vollbild wechseln", "switch_split": "Zu geteilter Ansicht wechseln", "end_talk": "Gespräch beenden", "editing_mode": "Bearbeiten / Vorschau", "reset_fullscreen": "Vollbild-Darstellung zurücksetzen"},
	"it": {"fullscreen_position_help": "−500–500% del lato minore a schermo intero; predefinito 0. X positivo sposta a destra, Y positivo in basso. Stesso riferimento quadrato del laboratorio.", "fullscreen_mode": "Schermo intero", "split_screen": "Schermo diviso", "talk_mode": "Vista Talk predefinita", "talk_mode_help": "split_screen o fullscreen; predefinito split_screen. Vale per la prossima conversazione.", "fullscreen_background": "Sfondo a schermo intero", "switch_fullscreen": "Passa a schermo intero", "switch_split": "Passa allo schermo diviso", "end_talk": "Termina conversazione", "editing_mode": "Modifica / anteprima", "reset_fullscreen": "Ripristina presentazione a schermo intero"},
	"vi": {"fullscreen_position_help": "−500–500% cạnh ngắn hơn của toàn màn hình; mặc định 0. X dương sang phải, Y dương xuống dưới. Cùng khung vuông tham chiếu như phòng thử.", "fullscreen_mode": "Toàn màn hình", "split_screen": "Chia màn hình", "talk_mode": "Giao diện Talk mặc định", "talk_mode_help": "split_screen hoặc fullscreen; mặc định split_screen. Áp dụng cho cuộc trò chuyện tiếp theo.", "fullscreen_background": "Nền toàn màn hình", "switch_fullscreen": "Chuyển sang toàn màn hình", "switch_split": "Chuyển sang chia màn hình", "end_talk": "Kết thúc trò chuyện", "editing_mode": "Chỉnh sửa / xem trước", "reset_fullscreen": "Đặt lại bố cục toàn màn hình"},
}

var motionWords = map[string]map[string]string{
	"en": {
		"motion_label":           "Preview animation (Lab only)",
		"motion_choose":          "Choose a VRMA animation",
		"motion_none":            "No Lab animation",
		"motion_removed":         "removed",
		"motion_refresh":         "Refresh",
		"motion_play":            "Play",
		"motion_pause":           "Pause",
		"motion_help":            "/data/misc/echolocal/avatar/animations/*.vrma",
		"motion_empty":           "No animations found. Add a .vrma file to the folder, then Refresh.",
		"motion_directory_error": "Cannot read the animation folder. Check its permissions; symbolic links are not allowed.",
		"motion_limit":           "Too many folder entries (maximum 8192). Remove unused entries, then Refresh.",
		"motion_unavailable":     "Cannot read this animation. Refresh after checking the file and its permissions, or choose another.",
		"motion_invalid":         "Animation rejected. Use a self-contained VRMA 1.0 with one supported humanoid animation. Choose another file.",
		"motion_incompatible":    "No supported animated bones match this avatar. Choose another animation or model.",
		"motion_storage":         "Cannot prepare animation. Free device storage and retry.",
		"motion_loading":         "Loading animation…",
		"motion_ready":           "Animation loaded. Play or pause the preview.",
		"motion_none_status":     "No Lab animation.",
	},
	"de": {
		"motion_label":           "Animationsvorschau (nur Labor)",
		"motion_choose":          "VRMA-Animation wählen",
		"motion_none":            "Keine Laboranimation",
		"motion_removed":         "entfernt",
		"motion_refresh":         "Aktualisieren",
		"motion_play":            "Abspielen",
		"motion_pause":           "Pause",
		"motion_help":            "/data/misc/echolocal/avatar/animations/*.vrma",
		"motion_empty":           "Keine Animationen gefunden. Eine .vrma-Datei im Ordner ablegen, dann aktualisieren.",
		"motion_directory_error": "Animationsordner nicht lesbar. Dateirechte prüfen; symbolische Links sind nicht erlaubt.",
		"motion_limit":           "Zu viele Ordnereinträge (maximal 8192). Nicht benötigte Einträge entfernen, dann aktualisieren.",
		"motion_unavailable":     "Animation nicht lesbar. Datei und Rechte prüfen, aktualisieren oder eine andere wählen.",
		"motion_invalid":         "Animation abgelehnt. Eigenständiges VRMA 1.0 mit einer unterstützten Humanoid-Animation verwenden. Andere Datei wählen.",
		"motion_incompatible":    "Keine unterstützten animierten Knochen passen zum Avatar. Andere Animation oder anderes Modell wählen.",
		"motion_storage":         "Animation kann nicht vorbereitet werden. Gerätespeicher freigeben und erneut versuchen.",
		"motion_loading":         "Animation wird geladen…",
		"motion_ready":           "Animation geladen. Vorschau abspielen oder pausieren.",
		"motion_none_status":     "Keine Laboranimation.",
	},
	"it": {
		"motion_label":           "Anteprima animazione (solo laboratorio)",
		"motion_choose":          "Scegli animazione VRMA",
		"motion_none":            "Nessuna animazione laboratorio",
		"motion_removed":         "rimosso",
		"motion_refresh":         "Aggiorna",
		"motion_play":            "Riproduci",
		"motion_pause":           "Pausa",
		"motion_help":            "/data/misc/echolocal/avatar/animations/*.vrma",
		"motion_empty":           "Nessuna animazione. Aggiungere un file .vrma nella cartella, poi Aggiorna.",
		"motion_directory_error": "Cartella animazioni non leggibile. Controllare i permessi; link simbolici non consentiti.",
		"motion_limit":           "Troppe voci nella cartella (massimo 8192). Rimuovere quelle inutili e aggiornare.",
		"motion_unavailable":     "Animazione non leggibile. Controllare file e permessi, aggiornare o sceglierne un’altra.",
		"motion_invalid":         "Animazione rifiutata. Usare VRMA 1.0 autonomo con una sola animazione umanoide supportata. Scegliere un altro file.",
		"motion_incompatible":    "Nessun osso animato supportato corrisponde all’avatar. Scegliere un’altra animazione o modello.",
		"motion_storage":         "Impossibile preparare l’animazione. Liberare spazio e riprovare.",
		"motion_loading":         "Caricamento animazione…",
		"motion_ready":           "Animazione caricata. Riprodurre o sospendere l’anteprima.",
		"motion_none_status":     "Nessuna animazione laboratorio.",
	},
	"vi": {
		"motion_label":           "Xem trước hoạt ảnh (chỉ phòng thử)",
		"motion_choose":          "Chọn hoạt ảnh VRMA",
		"motion_none":            "Không có hoạt ảnh phòng thử",
		"motion_removed":         "đã xóa",
		"motion_refresh":         "Làm mới",
		"motion_play":            "Phát",
		"motion_pause":           "Tạm dừng",
		"motion_help":            "/data/misc/echolocal/avatar/animations/*.vrma",
		"motion_empty":           "Không tìm thấy hoạt ảnh. Thêm tệp .vrma vào thư mục rồi Làm mới.",
		"motion_directory_error": "Không đọc được thư mục hoạt ảnh. Kiểm tra quyền; không cho phép liên kết tượng trưng.",
		"motion_limit":           "Quá nhiều mục trong thư mục (tối đa 8192). Xóa mục không dùng rồi làm mới.",
		"motion_unavailable":     "Không đọc được hoạt ảnh. Kiểm tra tệp và quyền, làm mới hoặc chọn tệp khác.",
		"motion_invalid":         "Hoạt ảnh bị từ chối. Dùng VRMA 1.0 độc lập với một hoạt ảnh hình người được hỗ trợ. Chọn tệp khác.",
		"motion_incompatible":    "Không có xương hoạt ảnh được hỗ trợ khớp với avatar. Chọn hoạt ảnh hoặc mô hình khác.",
		"motion_storage":         "Không chuẩn bị được hoạt ảnh. Giải phóng bộ nhớ và thử lại.",
		"motion_loading":         "Đang nạp hoạt ảnh…",
		"motion_ready":           "Đã nạp hoạt ảnh. Phát hoặc tạm dừng xem trước.",
		"motion_none_status":     "Không có hoạt ảnh phòng thử.",
	},
}

var foregroundWords = map[string]map[string]string{
	"en": {"foreground": "Foreground image", "fullscreen_foreground": "Fullscreen foreground", "foreground_reset": "Clear foreground", "foreground_help": "PNG/JPEG, up to 8 MiB, 4096 pixels per side and 4 megapixels. Alpha is preserved. Normalized to the background canvas, centered cover in the selected panel; use matching canvas proportions for registered cutouts. Applies on next Start.", "foreground_unavailable": "Cannot read foreground. Select another file or clear it. The rest of the scene is unchanged.", "foreground_invalid": "Foreground rejected. Use a complete PNG/JPEG within the image limits. The rest of the scene is unchanged.", "foreground_storage": "Cannot prepare foreground. Free device storage and retry.", "foreground_example": "/data/local/tmp/foreground.png"},
	"de": {"foreground": "Vordergrundbild", "fullscreen_foreground": "Vollbild-Vordergrund", "foreground_reset": "Vordergrund entfernen", "foreground_help": "PNG/JPEG, bis 8 MiB, 4096 Pixel je Seite und 4 Megapixel. Transparenz bleibt erhalten. Auf die Hintergrundfläche normiert, zentriert füllend im gewählten Bereich; für passende Ausschnitte gleiche Bildproportionen verwenden. Gilt beim nächsten Start.", "foreground_unavailable": "Vordergrund nicht lesbar. Andere Datei wählen oder entfernen. Die übrige Szene bleibt unverändert.", "foreground_invalid": "Vordergrund abgelehnt. Vollständiges PNG/JPEG innerhalb der Bildgrenzen verwenden. Die übrige Szene bleibt unverändert.", "foreground_storage": "Vordergrund nicht vorbereitet. Gerätespeicher freigeben und erneut versuchen.", "foreground_example": "/data/local/tmp/foreground.png"},
	"it": {"foreground": "Immagine in primo piano", "fullscreen_foreground": "Primo piano a schermo intero", "foreground_reset": "Rimuovi primo piano", "foreground_help": "PNG/JPEG, massimo 8 MiB, 4096 pixel per lato e 4 megapixel. Trasparenza preservata. Normalizzato allo sfondo, riempimento centrato; usare proporzioni uguali per ritagli allineati. Vale al prossimo avvio.", "foreground_unavailable": "Primo piano non leggibile. Scegliere un altro file o rimuoverlo. Il resto della scena non cambia.", "foreground_invalid": "Primo piano rifiutato. Usare PNG/JPEG completo entro i limiti. Il resto della scena non cambia.", "foreground_storage": "Impossibile preparare il primo piano. Liberare spazio e riprovare.", "foreground_example": "/data/local/tmp/foreground.png"},
	"vi": {"foreground": "Ảnh tiền cảnh", "fullscreen_foreground": "Tiền cảnh toàn màn hình", "foreground_reset": "Xóa tiền cảnh", "foreground_help": "PNG/JPEG, tối đa 8 MiB, 4096 pixel mỗi cạnh và 4 megapixel. Giữ độ trong suốt. Chuẩn hóa theo nền và cắt giữa để lấp đầy; dùng cùng tỷ lệ ảnh để căn khớp. Áp dụng ở lần Bắt đầu tiếp theo.", "foreground_unavailable": "Không đọc được tiền cảnh. Chọn tệp khác hoặc xóa. Phần còn lại không đổi.", "foreground_invalid": "Tiền cảnh bị từ chối. Dùng PNG/JPEG hoàn chỉnh trong giới hạn. Phần còn lại không đổi.", "foreground_storage": "Không thể chuẩn bị tiền cảnh. Giải phóng bộ nhớ và thử lại.", "foreground_example": "/data/local/tmp/foreground.png"},
}

// Kept identical for both layers and all owner-facing settings surfaces.
func sceneHelp(lang string) string {
	switch lang {
	case "de":
		return "Auch WebP (statisch/animiert): bis 1024 Pixel je Seite, 120 Frames, 32 MiB dekodiert, 30 s pro Schleife; 40–10000 ms je Frame. Alpha und Schleifenzahl bleiben erhalten. APNG, GIF und Video werden nicht unterstützt."
	case "it":
		return "Anche WebP (statico/animato): fino a 1024 pixel per lato, 120 fotogrammi, 32 MiB decodificati, 30 s per ciclo; 40–10000 ms per fotogramma. Alpha e numero di cicli preservati. APNG, GIF e video non supportati."
	case "vi":
		return "Cũng hỗ trợ WebP (tĩnh/động): tối đa 1024 pixel mỗi cạnh, 120 khung hình, 32 MiB giải mã, 30 giây mỗi vòng; 40–10000 ms mỗi khung hình. Giữ alpha và số vòng lặp. Không hỗ trợ APNG, GIF và video."
	default:
		return "Also WebP (static/animated): up to 1024 pixels per side, 120 frames, 32 MiB decoded, 30 s per loop; 40–10000 ms per frame. Alpha and loop count preserved. APNG, GIF and video are unsupported."
	}
}

// Video is intentionally background-only; alpha video is not advertised.
func videoHelp(lang string) string {
	switch lang {
	case "de":
		return "Nur Hintergrund: lokales MP4/H.264, 8-Bit-YUV420, Pixel-Seitenverhältnis wird berücksichtigt, keine Rotation; bis 64 MiB, 1280×800, 30 fps und 120 s. Endlosschleife ohne Ton. Softwaredecoder; kein Vordergrundvideo."
	case "it":
		return "Solo sfondo: MP4/H.264 locale, YUV420 a 8 bit, proporzioni dei pixel rispettate, senza rotazione; massimo 64 MiB, 1280×800, 30 fps e 120 s. Ciclo continuo senza audio. Decoder software; video in primo piano non supportato."
	case "vi":
		return "Chỉ nền: MP4/H.264 cục bộ, YUV420 8 bit, giữ tỷ lệ pixel, không xoay; tối đa 64 MiB, 1280×800, 30 fps và 120 giây. Lặp vô hạn, không âm thanh. Giải mã phần mềm; không hỗ trợ video tiền cảnh."
	default:
		return "Background only: local MP4/H.264, 8-bit YUV420, pixel aspect ratio respected, no rotation; up to 64 MiB, 1280×800, 30 fps and 120 s. Silent infinite loop. Software decoding; foreground video is not supported."
	}
}

var activityWords = map[string]map[string]string{
	"en": {"agent_working": "Agent working"},
	"de": {"agent_working": "Agent arbeitet"},
	"it": {"agent_working": "Agente al lavoro"},
	"vi": {"agent_working": "Tác tử đang xử lý"},
}

func init() {
	for lang, labels := range map[string][2]string{
		"en": {"Tap to pause dot animation", "Tap to resume dot animation"},
		"de": {"Antippen, um die Punktanimation anzuhalten", "Antippen, um die Punktanimation fortzusetzen"},
		"it": {"Tocca per fermare l’animazione dei punti", "Tocca per riprendere l’animazione dei punti"},
		"vi": {"Chạm để tạm dừng hoạt ảnh dấu chấm", "Chạm để tiếp tục hoạt ảnh dấu chấm"},
	} {
		activityWords[lang]["activity_pause"] = labels[0]
		activityWords[lang]["activity_resume"] = labels[1]
	}
}

var turnWords = map[string]map[string]string{
	"en": {"turn_listening": "Listening to you", "turn_speaking": "Assistant speaking", "turn_muted": "Microphone muted", "turn_waiting": "Waiting", "turn_connecting": "Connecting", "turn_disconnected": "Disconnected"},
	"de": {"turn_listening": "Hört dir zu", "turn_speaking": "Assistent spricht", "turn_muted": "Mikrofon stumm", "turn_waiting": "Wartet", "turn_connecting": "Verbindung wird hergestellt", "turn_disconnected": "Getrennt"},
	"it": {"turn_listening": "In ascolto", "turn_speaking": "L’assistente parla", "turn_muted": "Microfono disattivato", "turn_waiting": "In attesa", "turn_connecting": "Connessione in corso", "turn_disconnected": "Disconnesso"},
	"vi": {"turn_listening": "Đang nghe bạn", "turn_speaking": "Trợ lý đang nói", "turn_muted": "Micrô tắt tiếng", "turn_waiting": "Đang chờ", "turn_connecting": "Đang kết nối", "turn_disconnected": "Đã ngắt kết nối"},
}
