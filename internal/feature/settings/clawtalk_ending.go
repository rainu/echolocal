//go:build !board_biscuit

package settings

import (
	"github.com/ygelfand/echolocal/internal/feature/clawtalk"
	"github.com/ygelfand/echolocal/internal/feature/shell"
	"github.com/ygelfand/libcountertop/pkg/display/menu"
	sharedshell "github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/say"
	"sync"
)

func endText(key string) string {
	messages := map[string]map[string]string{
		"en": {"title": "End conversation", "phrase": "End phrase", "position": "Match position", "timing": "Evaluate", "exact": "Exact", "start": "Start", "end": "End", "turn_end": "End of turn", "immediate": "Immediately", "risk": "Interim recognition can change. Immediate matching may close early by mistake; the current transcript end is provisional.", "transport": "Immediate mode requires interim user transcripts. A final-only transport cannot close early.", "invalid": "Enter a phrase of 1–256 characters containing a word (one line).", "next": "Applies to the next conversation"},
		"de": {"title": "Gespräch beenden", "phrase": "Abschlussphrase", "position": "Trefferposition", "timing": "Auswertung", "exact": "Exakt", "start": "Am Anfang", "end": "Am Ende", "turn_end": "Am Turn-Ende", "immediate": "Sofort", "risk": "Zwischenerkennung kann sich ändern. Sofortige Treffer können irrtümlich früh schließen; das aktuelle Textende ist vorläufig.", "transport": "Sofort benötigt vorläufige Nutzertranskripte. Ein Transport nur mit Endergebnissen kann nicht früh schließen.", "invalid": "Phrase mit 1–256 Zeichen und mindestens einem Wort eingeben (eine Zeile).", "next": "Gilt ab dem nächsten Gespräch"},
		"it": {"title": "Termina conversazione", "phrase": "Frase di chiusura", "position": "Posizione", "timing": "Valutazione", "exact": "Esatta", "start": "Inizio", "end": "Fine", "turn_end": "Fine turno", "immediate": "Subito", "risk": "Il riconoscimento provvisorio può cambiare. La chiusura immediata può essere involontaria; la fine del testo è provvisoria.", "transport": "La modalità immediata richiede trascrizioni utente provvisorie. Un trasporto solo finale non può chiudere prima.", "invalid": "Inserire una frase di 1–256 caratteri con almeno una parola (una riga).", "next": "Dalla prossima conversazione"},
		"vi": {"title": "Kết thúc cuộc trò chuyện", "phrase": "Cụm từ kết thúc", "position": "Vị trí khớp", "timing": "Thời điểm kiểm tra", "exact": "Chính xác", "start": "Đầu câu", "end": "Cuối câu", "turn_end": "Cuối lượt", "immediate": "Ngay lập tức", "risk": "Nhận dạng tạm thời có thể thay đổi. Khớp ngay có thể đóng nhầm sớm; cuối văn bản hiện tại chỉ là tạm thời.", "transport": "Chế độ ngay cần bản chép lời người dùng tạm thời. Kênh chỉ có kết quả cuối không thể đóng sớm.", "invalid": "Nhập cụm từ 1–256 ký tự có ít nhất một từ (một dòng).", "next": "Áp dụng cho cuộc trò chuyện tiếp theo"},
	}
	lang := say.Chosen()
	if messages[lang] == nil {
		lang = "en"
	}
	return messages[lang][key]
}

func talkEndPage(slot int) *sharedshell.Page {
	return talkEndPageFor(func() clawtalk.Slot { return clawtalk.Get().Store.Get(slot) }, func(change func(*clawtalk.Slot)) error { return saveTalk(slot, change) }, func() { shell.Get().Redraw() })
}

func talkEndPageFor(read func() clawtalk.Slot, save func(func(*clawtalk.Slot)) error, redraw func()) *sharedshell.Page {
	// Field edits save atomically like the other single-line settings. Failed
	// validation leaves the stored phrase unchanged and displays a notice.
	var noticeMu sync.Mutex
	notice := ""
	commit := func(change func(*clawtalk.Slot)) {
		err := save(change)
		noticeMu.Lock()
		if err != nil {
			if err.Error() == "talk_end_phrase_invalid" {
				notice = endText("invalid")
			} else {
				notice = say.T("openclaw.error.storage")
			}
		} else {
			notice = ""
		}
		noticeMu.Unlock()
		redraw()
	}
	return &sharedshell.Page{Title: endText("title"), Build: func() ([]menu.Row, []func(int)) {
		s := read().Ending.Defaults(say.Chosen())
		rows := []menu.Row{
			{Label: endText("phrase"), Value: s.Phrase, Kind: menu.Field, Save: func(value string) { commit(func(v *clawtalk.Slot) { v.Ending.Phrase = value }) }},
			{Label: endText("position"), Value: endText(s.Position), Kind: menu.Chevron, Open: func() menu.View {
				return endChoicePage("position", []string{"exact", "start", "end"}, func() string { return read().Ending.Defaults(say.Chosen()).Position }, func(value string) { commit(func(v *clawtalk.Slot) { v.Ending.Position = value }) })
			}},
			{Label: endText("timing"), Value: endText(s.Timing), Kind: menu.Chevron, Open: func() menu.View {
				return endChoicePage("timing", []string{"turn_end", "immediate"}, func() string { return read().Ending.Defaults(say.Chosen()).Timing }, func(value string) { commit(func(v *clawtalk.Slot) { v.Ending.Timing = value }) })
			}},
			{Label: endText("risk"), Kind: menu.Plain},
			{Label: endText("transport"), Kind: menu.Plain},
			{Label: endText("next"), Kind: menu.Plain},
		}
		noticeMu.Lock()
		defer noticeMu.Unlock()
		if notice != "" {
			rows = append(rows, menu.Row{Label: notice, Kind: menu.Plain})
		}
		return rows, make([]func(int), len(rows))
	}}
}

func endChoicePage(key string, choices []string, selected func() string, choose func(string)) *sharedshell.Page {
	return &sharedshell.Page{Title: endText(key), Build: func() ([]menu.Row, []func(int)) {
		rows := []menu.Row{}
		acts := []func(int){}
		for _, value := range choices {
			value := value
			rows = append(rows, menu.Row{Label: endText(value), Chosen: selected() == value})
			acts = append(acts, func(int) { choose(value) })
		}
		return rows, acts
	}}
}
