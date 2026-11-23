package avatar

// Text covers all UI languages supported by EchoLocal. Error text never includes
// model contents, arbitrary process output, or a potentially sensitive path.
func Text(lang, key string) string {
	words := map[string]map[string]string{
		"en": {"title": "Avatar Lab", "path": "Custom VRM — path on Echo, not this computer (empty = default)", "model": "Model file", "default": "Use default avatar", "help": "Upload a .vrm using Files or WebDAV, then enter its full device path. Changes apply on the next Start. Empty restores the installed default.", "invalid_path": "Use an absolute device path, at most 255 bytes, without control characters or ../ components.", "custom_unavailable": "Cannot read custom VRM. Check the device path and file permissions, select another file or reset to default. No fallback was loaded.", "custom_invalid": "Custom model rejected. Use a self-contained VRM 0.x/1.0 GLB below 64 MiB; select another file or reset. No fallback was loaded.", "custom_storage": "Cannot stage the model. Free device storage and retry, or reset to default.", "model_invalid": "Renderer rejected the model or its resource budget. Select a compatible model or reset to default.", "starting": "Loading selected avatar…", "selection": "Selected file applies on next Start; the name above identifies the loaded model."},
		"de": {"title": "Avatar-Labor", "path": "Eigenes VRM — Pfad auf dem Echo, nicht auf diesem Computer (leer = Standard)", "model": "Modelldatei", "default": "Standardavatar verwenden", "help": "Eine .vrm-Datei über Dateien oder WebDAV hochladen, dann ihren vollständigen Gerätepfad eingeben. Änderungen gelten beim nächsten Start. Leer stellt den installierten Standard wieder her.", "invalid_path": "Absoluten Gerätepfad mit maximal 255 Bytes eingeben, ohne Steuerzeichen oder ../-Bestandteile.", "custom_unavailable": "Eigenes VRM nicht lesbar. Gerätepfad und Dateirechte prüfen, andere Datei wählen oder Standard wiederherstellen. Kein Ersatzmodell geladen.", "custom_invalid": "Eigenes Modell abgelehnt. Eigenständiges VRM-0.x/1.0-GLB unter 64 MiB verwenden; andere Datei wählen oder Standard wiederherstellen. Kein Ersatzmodell geladen.", "custom_storage": "Modell kann nicht vorbereitet werden. Gerätespeicher freigeben und erneut versuchen oder Standard wählen.", "model_invalid": "Renderer lehnt Modell oder Ressourcenbedarf ab. Kompatibles Modell wählen oder Standard wiederherstellen.", "starting": "Ausgewählter Avatar wird geladen…", "selection": "Dateiauswahl gilt beim nächsten Start; der Name oben bezeichnet das geladene Modell."},
		"it": {"title": "Laboratorio avatar", "path": "VRM personale — percorso su Echo, non su questo computer (vuoto = predefinito)", "model": "File del modello", "default": "Usa avatar predefinito", "help": "Caricare un file .vrm tramite File o WebDAV e inserirne il percorso completo sul dispositivo. Le modifiche valgono al prossimo avvio. Vuoto ripristina il modello installato.", "invalid_path": "Usare un percorso assoluto sul dispositivo di massimo 255 byte, senza caratteri di controllo o componenti ../.", "custom_unavailable": "VRM personale non leggibile. Controllare percorso e permessi, scegliere un altro file o ripristinare il predefinito. Nessun modello sostitutivo caricato.", "custom_invalid": "Modello rifiutato. Usare un GLB VRM 0.x/1.0 autonomo inferiore a 64 MiB; scegliere un altro file o ripristinare. Nessun modello sostitutivo caricato.", "custom_storage": "Impossibile preparare il modello. Liberare spazio sul dispositivo e riprovare o ripristinare.", "model_invalid": "Il renderer rifiuta il modello o il consumo di risorse. Scegliere un modello compatibile o ripristinare.", "starting": "Caricamento dell’avatar selezionato…", "selection": "Il file selezionato vale al prossimo avvio; il nome sopra identifica il modello caricato."},
		"vi": {"title": "Phòng thử avatar", "path": "VRM riêng — đường dẫn trên Echo, không phải máy tính này (trống = mặc định)", "model": "Tệp mô hình", "default": "Dùng avatar mặc định", "help": "Tải tệp .vrm lên bằng Tệp hoặc WebDAV, rồi nhập đường dẫn đầy đủ trên thiết bị. Thay đổi áp dụng ở lần Bắt đầu tiếp theo. Để trống để dùng mô hình mặc định đã cài.", "invalid_path": "Dùng đường dẫn tuyệt đối trên thiết bị, tối đa 255 byte, không có ký tự điều khiển hoặc thành phần ../.", "custom_unavailable": "Không đọc được VRM riêng. Kiểm tra đường dẫn và quyền tệp, chọn tệp khác hoặc khôi phục mặc định. Không nạp mô hình thay thế.", "custom_invalid": "Mô hình bị từ chối. Dùng GLB VRM 0.x/1.0 độc lập dưới 64 MiB; chọn tệp khác hoặc khôi phục. Không nạp mô hình thay thế.", "custom_storage": "Không thể chuẩn bị mô hình. Giải phóng bộ nhớ thiết bị và thử lại hoặc khôi phục mặc định.", "model_invalid": "Trình dựng từ chối mô hình hoặc mức dùng tài nguyên. Chọn mô hình tương thích hoặc khôi phục mặc định.", "starting": "Đang nạp avatar đã chọn…", "selection": "Tệp đã chọn áp dụng ở lần Bắt đầu tiếp theo; tên bên trên là mô hình đang nạp."},
	}
	if words[lang] == nil {
		lang = "en"
	}
	if v := pickerWords[lang][key]; v != "" {
		return v
	}
	if v := backgroundWords[lang][key]; v != "" {
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
		"background":             "Background image",
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
		"background":             "Hintergrundbild",
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

var pickerWords = map[string]map[string]string{
	"en": {"choose_image": "Choose image on Echo", "picker_title": "Image files on Echo", "picker_cancel": "Cancel", "picker_folder": "Device folder", "picker_uploads": "Uploads", "picker_up": "Up", "picker_refresh": "Refresh", "picker_loading": "Loading folder…", "picker_empty": "No matching images on this page.", "picker_previous": "Previous page", "picker_next": "Next page", "picker_limit": "Directory limit reached. Choose a more specific folder.", "picker_error": "Cannot read this folder. Check access permissions, go up or choose Uploads.", "picker_go": "Go"},
	"de": {"choose_image": "Bild auf Echo auswählen", "picker_title": "Bilddateien auf dem Echo", "picker_cancel": "Abbrechen", "picker_folder": "Geräteordner", "picker_uploads": "Uploads", "picker_up": "Übergeordneter Ordner", "picker_refresh": "Aktualisieren", "picker_loading": "Ordner wird geladen…", "picker_empty": "Keine passenden Bilder auf dieser Seite.", "picker_previous": "Vorherige Seite", "picker_next": "Nächste Seite", "picker_limit": "Ordnergrenze erreicht. Einen genaueren Ordner wählen.", "picker_error": "Ordner nicht lesbar. Zugriffsrechte prüfen, nach oben wechseln oder Uploads wählen.", "picker_go": "Öffnen"},
	"it": {"choose_image": "Scegli immagine su Echo", "picker_title": "Immagini su Echo", "picker_cancel": "Annulla", "picker_folder": "Cartella sul dispositivo", "picker_uploads": "Caricamenti", "picker_up": "Cartella superiore", "picker_refresh": "Aggiorna", "picker_loading": "Caricamento cartella…", "picker_empty": "Nessuna immagine compatibile in questa pagina.", "picker_previous": "Pagina precedente", "picker_next": "Pagina successiva", "picker_limit": "Limite della cartella raggiunto. Scegliere una cartella più specifica.", "picker_error": "Cartella non leggibile. Verificare i permessi, risalire o scegliere Caricamenti.", "picker_go": "Apri"},
	"vi": {"choose_image": "Chọn ảnh trên Echo", "picker_title": "Tệp ảnh trên Echo", "picker_cancel": "Hủy", "picker_folder": "Thư mục thiết bị", "picker_uploads": "Tệp tải lên", "picker_up": "Thư mục cha", "picker_refresh": "Làm mới", "picker_loading": "Đang tải thư mục…", "picker_empty": "Không có ảnh phù hợp trên trang này.", "picker_previous": "Trang trước", "picker_next": "Trang tiếp theo", "picker_limit": "Đã đạt giới hạn thư mục. Chọn thư mục cụ thể hơn.", "picker_error": "Không đọc được thư mục. Kiểm tra quyền, trở về thư mục cha hoặc chọn Tệp tải lên.", "picker_go": "Mở"},
}
