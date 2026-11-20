//go:build !board_biscuit

package settings

import (
	"fmt"
	"github.com/ygelfand/echolocal/internal/feature/clawtalk"
	featureclaw "github.com/ygelfand/echolocal/internal/feature/openclaw"
	"github.com/ygelfand/echolocal/internal/feature/shell"
	"github.com/ygelfand/echolocal/internal/openclaw"
	"github.com/ygelfand/libcountertop/pkg/display/menu"
	sharedshell "github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/say"
	"sync"
)

func promptText(key string) string {
	messages := map[string]map[string]string{
		"en": {"session": "Session", "newline": "New line", "title": "Device context prompt", "template": "Prompt", "default": "Default", "custom": "Custom", "next": "Applies to the next conversation", "help": "Insert placeholder", "reset": "Restore default", "pending": "Default restored in editor. Save to apply.", "tooLong": "Expanded prompt is too long (maximum 1536 UTF-8 bytes). Shorten the text.", "placeholder": "Unknown or incomplete placeholder.", "invalid": "Enter a valid template or restore the default.", "bytes": "Expanded: %d / %d bytes", "device_context": "Complete device metadata (JSON)", "node_id": "Exact Node ID", "device_name": "Device name", "device_model": "Hardware model", "connected": "Connected: true or false", "allowed_commands": "Partial allowed-command list (JSON)", "commands_omitted": "Number of omitted commands", "insert": "Tap to append a placeholder to the draft."},
		"de": {"session": "Sitzung", "newline": "Zeilenumbruch", "title": "Gerätekontext-Prompt", "template": "Prompt", "default": "Standard", "custom": "Angepasst", "next": "Gilt ab dem nächsten Gespräch", "help": "Platzhalter einfügen", "reset": "Standard wiederherstellen", "pending": "Standard im Editor wiederhergestellt. Zum Übernehmen speichern.", "tooLong": "Ausgefüllter Prompt zu lang (maximal 1536 UTF-8-Bytes). Text kürzen.", "placeholder": "Unbekannter oder unvollständiger Platzhalter.", "invalid": "Gültige Vorlage eingeben oder Standard wiederherstellen.", "bytes": "Ausgefüllt: %d / %d Bytes", "device_context": "Vollständige Gerätedaten (JSON)", "node_id": "Exakte Node-ID", "device_name": "Gerätename", "device_model": "Hardwaremodell", "connected": "Verbunden: true oder false", "allowed_commands": "Unvollständige Liste erlaubter Kommandos (JSON)", "commands_omitted": "Anzahl ausgelassener Kommandos", "insert": "Antippen fügt den Platzhalter am Ende des Entwurfs ein."},
		"it": {"session": "Sessione", "newline": "Nuova riga", "title": "Prompt contesto dispositivo", "template": "Prompt", "default": "Predefinito", "custom": "Personalizzato", "next": "Dalla prossima conversazione", "help": "Inserisci segnaposto", "reset": "Ripristina predefinito", "pending": "Predefinito ripristinato. Salvare per applicare.", "tooLong": "Prompt compilato troppo lungo (massimo 1536 byte UTF-8). Ridurre il testo.", "placeholder": "Segnaposto sconosciuto o incompleto.", "invalid": "Inserire un modello valido o ripristinare il predefinito.", "bytes": "Compilato: %d / %d byte", "device_context": "Metadati completi (JSON)", "node_id": "ID Node esatto", "device_name": "Nome dispositivo", "device_model": "Modello hardware", "connected": "Connesso: true o false", "allowed_commands": "Elenco parziale comandi consentiti (JSON)", "commands_omitted": "Numero comandi omessi", "insert": "Toccare per aggiungere un segnaposto alla fine."},
		"vi": {"session": "Phiên", "newline": "Xuống dòng", "title": "Lời nhắc ngữ cảnh thiết bị", "template": "Lời nhắc", "default": "Mặc định", "custom": "Tùy chỉnh", "next": "Áp dụng cho cuộc trò chuyện tiếp theo", "help": "Chèn biến", "reset": "Khôi phục mặc định", "pending": "Đã khôi phục mẫu. Lưu để áp dụng.", "tooLong": "Lời nhắc quá dài (tối đa 1536 byte UTF-8). Hãy rút ngắn.", "placeholder": "Biến không hợp lệ hoặc chưa hoàn chỉnh.", "invalid": "Nhập mẫu hợp lệ hoặc khôi phục mặc định.", "bytes": "Đã điền: %d / %d byte", "device_context": "Toàn bộ dữ liệu thiết bị (JSON)", "node_id": "ID Node chính xác", "device_name": "Tên thiết bị", "device_model": "Mẫu phần cứng", "connected": "Đã kết nối: true hoặc false", "allowed_commands": "Danh sách lệnh được phép một phần (JSON)", "commands_omitted": "Số lệnh bị lược bỏ", "insert": "Chạm để thêm biến vào cuối bản nháp."},
	}
	lang := say.Chosen()
	if messages[lang] == nil {
		lang = "en"
	}
	return messages[lang][key]
}
func promptError(err error) string {
	if err == nil {
		return ""
	}
	switch err.Error() {
	case "talk_context_too_long", "talk_greeting_too_long":
		return promptText("tooLong")
	case "talk_context_placeholder":
		return promptText("placeholder")
	case "talk_context_invalid", "talk_greeting_invalid":
		return promptText("invalid")
	default:
		return say.T("openclaw.error.storage")
	}
}

// TalkPromptEditor uses the app's native multiline renderer while retaining
// normal settings-page navigation, actions and protected-menu recognition.
type TalkPromptEditor struct {
	*sharedshell.Page
	Leading func() ([]menu.Row, []func(int))
}

func (p *TalkPromptEditor) SettingsPage() *sharedshell.Page { return p.Page }

func talkDevicePromptPage(slot int) *TalkPromptEditor {
	return &TalkPromptEditor{Page: talkPromptPageFor(clawtalk.Get().Store.Get(slot).DeviceContextTemplate,
		featureclaw.Get().Node.ConversationDeviceContextWithTemplate,
		func(value string) error {
			return saveTalk(slot, func(v *clawtalk.Slot) { v.DeviceContextTemplate = value })
		},
		func() { shell.Get().Redraw() })}
}
func talkPromptPageFor(initial string, render func(string) (string, error), save func(string) error, redraw func()) *sharedshell.Page {
	return talkPromptPageWithDefault(initial, render, save, redraw, promptText("title"), openclaw.DefaultDeviceContextTemplate, true)
}
func talkPromptPageWithDefault(initial string, render func(string) (string, error), save func(string) error, redraw func(), title string, defaultText func() string, placeholders bool) *sharedshell.Page {
	var mu sync.Mutex
	draft := initial
	useDefault := initial == ""
	notice := ""
	page := &sharedshell.Page{Title: title}
	page.Build = func() ([]menu.Row, []func(int)) {
		mu.Lock()
		value, note := draft, notice
		if useDefault {
			value = defaultText()
		}
		mu.Unlock()
		expanded, err := render(value)
		hint := fmt.Sprintf(promptText("bytes"), len(expanded), openclaw.DeviceContextMaxBytes)
		if err != nil {
			hint = promptError(err)
		}
		rows := []menu.Row{
			{Label: promptText("template"), Value: value, Hint: hint, Kind: menu.Field, Save: func(v string) {
				mu.Lock()
				draft = v
				useDefault = false
				notice = ""
				mu.Unlock()
				redraw()
			}},
			{Label: promptText("help"), Hint: promptText("insert"), Kind: menu.Chevron, Open: func() menu.View {
				return &sharedshell.Page{Title: promptText("help"), Build: func() ([]menu.Row, []func(int)) {
					rows := []menu.Row{}
					acts := []func(int){}
					for _, key := range openclaw.DeviceContextPlaceholders {
						token := "{{" + key + "}}"
						rows = append(rows, menu.Row{Label: token, Hint: promptText(key)})
						acts = append(acts, func(int) {
							mu.Lock()
							if useDefault {
								draft = defaultText()
							}
							useDefault = false
							draft += token
							notice = ""
							mu.Unlock()
							shell.Get().Pop()
							redraw()
						})
					}
					return rows, acts
				}}
			}},
			{Label: promptText("next"), Kind: menu.Plain},
		}
		// The greeting is plain provider text, not a device template.
		if !placeholders {
			rows = append(rows[:1], rows[2:]...)
		}
		if note != "" {
			rows = append(rows, menu.Row{Label: note, Kind: menu.Plain})
		}
		return rows, make([]func(int), len(rows))
	}
	page.Actions = []sharedshell.Action{
		{Label: promptText("reset"), MinHeight: 64, Do: func() {
			mu.Lock()
			draft = ""
			useDefault = true
			notice = promptText("pending")
			mu.Unlock()
			redraw()
		}},
		{Label: say.T("openclaw.action.save"), MinHeight: 64, Role: sharedshell.Primary, Do: func() {
			mu.Lock()
			value := draft
			if useDefault {
				value = defaultText()
			}
			mu.Unlock()
			_, err := render(value)
			stored := value
			if err == nil {
				if stored == defaultText() {
					stored = ""
				}
				err = save(stored)
			}
			mu.Lock()
			notice = promptError(err)
			if err == nil {
				notice = say.T("openclaw.notice.saved")
				useDefault = stored == ""
			}
			mu.Unlock()
			redraw()
		}},
	}
	return page
}

func (p *TalkPromptEditor) NewlineLabel() string { return promptText("newline") }
