//go:build !board_biscuit

package settings

import (
	"sync"

	"github.com/ygelfand/echolocal/internal/feature/shell"
	"github.com/ygelfand/echolocal/internal/openclaw"
	"github.com/ygelfand/libcountertop/pkg/display/menu"
	sharedshell "github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/say"
)

func connectionText(key string) string {
	messages := map[string]map[string]string{
		"en": {"ui": "Controller UI", "node": "Node", "uiName": "Controller UI device name", "nodeName": "Node device name", "permissions": "Permissions", "permissionsHint": "Allowed device functions", "diagUI": "Controller UI diagnostics", "diagNode": "Node diagnostics", "forget": "Forget both connections", "forgetHint": "Remove both local identities and the shared Gateway configuration? Gateway approvals are not revoked.", "invalidName": "Use a name of up to 80 characters without control characters."},
		"de": {"ui": "Controller UI", "node": "Node", "uiName": "Gerätename Controller UI", "nodeName": "Gerätename Node", "permissions": "Berechtigungen", "permissionsHint": "Erlaubte Gerätefunktionen", "diagUI": "Diagnose Controller UI", "diagNode": "Diagnose Node", "forget": "Beide Verbindungen vergessen", "forgetHint": "Beide lokalen Identitäten und die gemeinsame Gateway-Konfiguration entfernen? Gateway-Freigaben bleiben bestehen.", "invalidName": "Name mit höchstens 80 Zeichen ohne Steuerzeichen verwenden."},
		"it": {"ui": "Controller UI", "node": "Nodo", "uiName": "Nome dispositivo Controller UI", "nodeName": "Nome dispositivo nodo", "permissions": "Autorizzazioni", "permissionsHint": "Funzioni dispositivo consentite", "diagUI": "Diagnostica Controller UI", "diagNode": "Diagnostica nodo", "forget": "Dimentica entrambe le connessioni", "forgetHint": "Rimuovere entrambe le identità locali e la configurazione Gateway condivisa? Le approvazioni Gateway restano valide.", "invalidName": "Usare un nome di massimo 80 caratteri senza caratteri di controllo."},
		"vi": {"ui": "Controller UI", "node": "Node", "uiName": "Tên thiết bị Controller UI", "nodeName": "Tên thiết bị Node", "permissions": "Quyền truy cập", "permissionsHint": "Chức năng thiết bị được phép", "diagUI": "Chẩn đoán Controller UI", "diagNode": "Chẩn đoán Node", "forget": "Quên cả hai kết nối", "forgetHint": "Xóa hai danh tính cục bộ và cấu hình Gateway chung? Phê duyệt Gateway không bị thu hồi.", "invalidName": "Dùng tên tối đa 80 ký tự, không có ký tự điều khiển."},
	}
	lang := say.Chosen()
	if messages[lang] == nil {
		lang = "en"
	}
	return messages[lang][key]
}
func openclawConnectionPage(g *openclaw.Connections) *sharedshell.Page {
	var mu sync.Mutex
	draft := g.Editable()
	dirty := false
	notice := ""
	setNotice := func(err error) {
		mu.Lock()
		notice = ""
		if err != nil {
			if err.Error() == "invalid_name" {
				notice = connectionText("invalidName")
			} else {
				notice = clawReason(err.Error())
				if notice == "" {
					notice = say.T("openclaw.error.settings")
				}
			}
		}
		mu.Unlock()
		shell.Get().Redraw()
	}
	page := &sharedshell.Page{Title: say.T("openclaw.connection")}
	page.Build = func() ([]menu.Row, []func(int)) {
		ui, node := g.UI.Status(), g.Node.Status()
		mu.Lock()
		if !dirty {
			draft = g.Editable()
		}
		d, n := draft, notice
		mu.Unlock()
		edit := func(apply func(*openclaw.ConnectionSettings, string)) func(string) {
			return func(v string) { mu.Lock(); apply(&draft, v); dirty = true; mu.Unlock() }
		}
		rows := []menu.Row{
			{Label: connectionText("ui"), Value: clawState(ui.State), Hint: clawReason(ui.Reason), Kind: menu.Plain},
			{Label: connectionText("node"), Value: clawState(node.State), Hint: clawReason(node.Reason), Kind: menu.Plain},
			{Label: say.T("openclaw.endpoint.label"), Value: d.URL, Kind: menu.Field, Save: edit(func(s *openclaw.ConnectionSettings, v string) { s.URL = v })},
			{Label: say.T("openclaw.token.label"), Value: d.Token, Kind: menu.Password, Save: edit(func(s *openclaw.ConnectionSettings, v string) { s.Token = v })},
			{Label: connectionText("uiName"), Value: d.UIName, Kind: menu.Field, Save: edit(func(s *openclaw.ConnectionSettings, v string) { s.UIName = v })},
			{Label: connectionText("nodeName"), Value: d.NodeName, Kind: menu.Field, Save: edit(func(s *openclaw.ConnectionSettings, v string) { s.NodeName = v })},
			{Label: connectionText("permissions"), Hint: connectionText("permissionsHint"), Kind: menu.Chevron, Open: func() menu.View { return nodePermissionsPageFor(g.Node) }},
			{Label: connectionText("diagUI"), Kind: menu.Chevron, Open: func() menu.View { return clawDiagnostics(g.UI, connectionText("diagUI")) }},
			{Label: connectionText("diagNode"), Kind: menu.Chevron, Open: func() menu.View { return clawDiagnostics(g.Node, connectionText("diagNode")) }},
		}
		if n != "" {
			rows = append(rows, menu.Row{Label: n, Kind: menu.Plain})
		}
		return rows, make([]func(int), len(rows))
	}
	page.Actions = []sharedshell.Action{
		{Label: say.T("openclaw.action.connect"), MinHeight: 64, Role: sharedshell.Plain, Do: func() { setNotice(g.Connect()) }},
		{Label: say.T("openclaw.action.disconnect"), MinHeight: 64, Role: sharedshell.Plain, Do: func() { setNotice(g.Disconnect()) }},
		{Label: say.T("openclaw.action.pair_again"), MinHeight: 64, Role: sharedshell.Plain, Do: func() { setNotice(g.PairAgain()) }},
		{Label: say.T("openclaw.action.forget"), MinHeight: 64, Role: sharedshell.Danger, Do: func() {
			shell.Get().Push(&sharedshell.Page{
				Title: connectionText("forget"),
				Build: func() ([]menu.Row, []func(int)) {
					return []menu.Row{{Label: connectionText("forgetHint"), Kind: menu.Plain}}, []func(int){nil}
				},
				Actions: []sharedshell.Action{
					{Label: say.T("openclaw.action.cancel"), MinHeight: 64, Do: func() { shell.Get().Pop() }},
					{Label: say.T("openclaw.action.delete"), MinHeight: 64, Role: sharedshell.Danger, Do: func() {
						if err := g.Forget(); err != nil {
							setNotice(err)
							return
						}
						mu.Lock()
						draft = g.Editable()
						dirty = false
						notice = ""
						mu.Unlock()
						shell.Get().Pop()
					}},
				},
			})
		}},
		{Label: say.T("openclaw.action.save"), MinHeight: 64, Role: sharedshell.Primary, Do: func() {
			mu.Lock()
			next := draft
			mu.Unlock()
			err := g.Save(next)
			setNotice(err)
			if err == nil {
				mu.Lock()
				draft = g.Editable()
				dirty = false
				notice = say.T("openclaw.notice.saved")
				mu.Unlock()
			}
		}},
	}
	return page
}
