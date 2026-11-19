//go:build !board_biscuit

package settings

import (
	"github.com/ygelfand/echolocal/internal/feature/shell"
	"github.com/ygelfand/echolocal/internal/openclaw"
	"github.com/ygelfand/libcountertop/pkg/display/menu"
	sharedshell "github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/say"
	"strings"
	"sync"
)

func operatorText(key string) string {
	texts := map[string]map[string]string{
		"en": {"title": "Operator permissions", "hint": "Authority to act through the Gateway, not local Node functions.", "all": "All permissions", "read": "Read and Talk only", "requested": "Requested", "granted": "Granted on active connection", "none": "None", "notice": "Changes reconnect with the same identity. Upgrades may need Gateway approval; use Connect after approval. A rejected request remains disconnected. If device-token authentication rejects an expansion, configure a suitable Gateway credential in Connection and retry. Reductions apply to the new connection; revoke broader stored device authorization at the Gateway too.", "error": "Could not save permissions"},
		"de": {"title": "Operator-Berechtigungen", "hint": "Rechte zum Handeln über den Gateway, keine lokalen Node-Funktionen.", "all": "Alle Berechtigungen", "read": "Nur Lesen und Sprechen", "requested": "Angefordert", "granted": "Auf aktiver Verbindung gewährt", "none": "Keine", "notice": "Änderungen verbinden dieselbe Identität neu. Erweiterungen brauchen ggf. Gateway-Freigabe; danach Verbinden drücken. Abgelehnte Anfragen bleiben getrennt. Bei abgewiesener Gerätetoken-Anmeldung für Erweiterungen einen geeigneten Gateway-Zugang unter Verbindung eintragen und erneut versuchen. Einschränkungen gelten für die neue Verbindung; breitere gespeicherte Gerätefreigaben auch am Gateway widerrufen.", "error": "Berechtigungen konnten nicht gespeichert werden"},
		"it": {"title": "Autorizzazioni operatore", "hint": "Autorità tramite Gateway, non funzioni Node locali.", "all": "Tutte le autorizzazioni", "read": "Solo lettura e voce", "requested": "Richieste", "granted": "Concesse sulla connessione attiva", "none": "Nessuna", "notice": "Le modifiche riconnettono la stessa identità. Gli ampliamenti possono richiedere approvazione Gateway: poi premere Connetti. Revocare sul Gateway anche le autorizzazioni più ampie memorizzate.", "error": "Salvataggio non riuscito"},
		"vi": {"title": "Quyền Operator", "hint": "Quyền qua Gateway, không phải chức năng Node cục bộ.", "all": "Tất cả quyền", "read": "Chỉ đọc và giọng nói", "requested": "Yêu cầu", "granted": "Được cấp trên kết nối hiện tại", "none": "Không có", "notice": "Thay đổi kết nối lại với cùng danh tính. Quyền mở rộng có thể cần Gateway phê duyệt; sau đó nhấn Kết nối. Thu hồi quyền rộng hơn đã lưu tại Gateway.", "error": "Không thể lưu quyền"},
	}
	lang := say.Chosen()
	if texts[lang] == nil {
		lang = "en"
	}
	return texts[lang][key]
}
func operatorPermissionsPage(c *openclaw.Client) *sharedshell.Page {
	var mu sync.Mutex
	notice := ""
	apply := func(scopes []string) {
		err := c.SetOperatorScopes(scopes)
		mu.Lock()
		notice = ""
		if err != nil {
			notice = operatorText("error")
		}
		mu.Unlock()
		shell.Get().Redraw()
	}
	format := func(scopes []string) string {
		if len(scopes) == 0 {
			return operatorText("none")
		}
		return strings.Join(scopes, ", ")
	}
	return &sharedshell.Page{Title: operatorText("title"), Build: func() ([]menu.Row, []func(int)) {
		requested := c.RequestedOperatorScopes()
		chosen := map[string]bool{}
		for _, scope := range requested {
			chosen[scope] = true
		}
		state := c.Status()
		rows := []menu.Row{
			{Label: operatorText("hint"), Kind: menu.Plain},
			{Label: operatorText("requested"), Hint: format(requested), Kind: menu.Plain},
			{Label: operatorText("granted"), Hint: format(c.GrantedOperatorScopes()), Kind: menu.Plain},
			{Label: clawState(state.State), Hint: clawReason(state.Reason), Kind: menu.Plain},
			{Label: operatorText("all"), On: len(requested) == len(openclaw.SupportedOperatorScopes()), Kind: menu.Toggle},
		}
		acts := []func(int){nil, nil, nil, nil, func(int) {
			if len(requested) == len(openclaw.SupportedOperatorScopes()) {
				apply([]string{})
			} else {
				apply(openclaw.SupportedOperatorScopes())
			}
		}}
		for _, scope := range openclaw.SupportedOperatorScopes() {
			rows = append(rows, menu.Row{Label: operatorScopeLabel(scope), Hint: scope, Kind: menu.Toggle, On: chosen[scope]})
			acts = append(acts, func(int) {
				next := []string{}
				for _, v := range openclaw.SupportedOperatorScopes() {
					if (v == scope && !chosen[v]) || (v != scope && chosen[v]) {
						next = append(next, v)
					}
				}
				apply(next)
			})
		}
		rows = append(rows, menu.Row{Label: operatorText("notice"), Kind: menu.Plain})
		acts = append(acts, nil)
		mu.Lock()
		n := notice
		mu.Unlock()
		if n != "" {
			rows = append(rows, menu.Row{Label: n, Kind: menu.Plain})
			acts = append(acts, nil)
		}
		return rows, acts
	}, Actions: []sharedshell.Action{
		{Label: operatorText("all"), MinHeight: 64, Do: func() { apply(openclaw.SupportedOperatorScopes()) }},
		{Label: operatorText("read"), MinHeight: 64, Do: func() { apply([]string{"operator.read", "operator.talk"}) }},
		{Label: say.T("openclaw.action.connect"), MinHeight: 64, Do: func() {
			if c.Connect() != nil {
				mu.Lock()
				notice = operatorText("error")
				mu.Unlock()
			}
			shell.Get().Redraw()
		}},
	}}
}

// Labels describe capability; the adjacent protocol scope remains inspectable.
func operatorScopeLabel(scope string) string {
	en := map[string]string{"operator.admin": "Administration", "operator.read": "Read Gateway data", "operator.write": "Perform actions", "operator.sessions.read": "Read sessions", "operator.sessions.write": "Modify sessions", "operator.approvals": "Manage approvals", "operator.questions": "Answer questions", "operator.pairing": "Manage device pairing", "operator.talk": "Use Talk", "operator.talk.secrets": "Access Talk provider credentials"}
	de := map[string]string{"operator.admin": "Administration", "operator.read": "Gateway-Daten lesen", "operator.write": "Aktionen ausführen", "operator.sessions.read": "Sitzungen lesen", "operator.sessions.write": "Sitzungen ändern", "operator.approvals": "Freigaben verwalten", "operator.questions": "Fragen beantworten", "operator.pairing": "Gerätekopplung verwalten", "operator.talk": "Sprachmodus verwenden", "operator.talk.secrets": "Sprachanbieter-Zugangsdaten abrufen"}
	if say.Chosen() == "de" {
		return de[scope]
	}
	return en[scope]
}
