//go:build !board_biscuit

package settings

import (
	"fmt"
	"sync"

	"github.com/ygelfand/echolocal/internal/feature/shell"
	"github.com/ygelfand/echolocal/internal/layout"
	"github.com/ygelfand/echolocal/internal/openclaw"
	"github.com/ygelfand/libcountertop/pkg/display/menu"
	sharedshell "github.com/ygelfand/libcountertop/pkg/display/shell"
)

var claw = sync.OnceValue(func() *openclaw.Client {
	c := openclaw.New(layout.StateDir+"/openclaw.json", layout.Version)
	c.Start()
	return c
})

func openclawPage() *sharedshell.Page {
	c := claw()
	var draftMu sync.Mutex
	endpoint, token := c.EditableConnection()
	var notice string
	dirty := false
	setNotice := func(e error) {
		draftMu.Lock()
		defer draftMu.Unlock()
		notice = ""
		if e != nil {
			notice = clawReason(e.Error())
		}
	}
	page := &sharedshell.Page{Title: "OpenClaw"}
	page.Build = func() ([]menu.Row, []func(int)) {
		s := c.Status()
		storedURL, storedToken := c.EditableConnection()
		draftMu.Lock()
		if !dirty {
			endpoint, token = storedURL, storedToken
		}
		u, t, note := endpoint, token, notice
		draftMu.Unlock()
		return []menu.Row{
			{Label: "Verbindung", Value: clawState(s.State), Hint: clawReason(s.Reason), Kind: menu.Plain},
			{Label: "Geräte-Pairing", Hint: "Adresse und Token speichern, verbinden und das Gerät im Gateway freigeben. Danach erneut verbinden.", Kind: menu.Plain},
			{Label: "Gateway-Adresse", Hint: "Gespeicherte WSS-Adresse; hier direkt editierbar.", Value: u, Kind: menu.Field, Save: func(v string) { draftMu.Lock(); endpoint = v; dirty = true; draftMu.Unlock() }},
			{Label: "Gateway-Token", Hint: "Gespeicherter Zugangstoken bzw. Gerätetoken; verdeckt und editierbar.", Value: t, Kind: menu.Password, Save: func(v string) { draftMu.Lock(); token = v; dirty = true; draftMu.Unlock() }},
			{Label: "Änderungen", Hint: note, Kind: menu.Plain},
			{Label: "Erneut pairen", Hint: "Behält die Identität. Ein widerrufener Token benötigt gegebenenfalls einen neuen Gateway-Token.", Kind: menu.Plain},
			{Label: "Vergessen", Hint: "Löscht nur lokale Zugangsdaten, nicht die Freigabe am Gateway.", Kind: menu.Plain},
			{Label: "Diagnose", Hint: "Begrenzte technische Ereignisse ohne Inhalte oder Zugangsdaten.", Kind: menu.Chevron, Open: func() menu.View { return clawDiagnostics(c) }},
		}, []func(int){nil, nil, nil, nil, nil, nil, nil, nil}
	}
	page.Actions = []sharedshell.Action{
		{Label: "Verbinden / prüfen", Role: sharedshell.Plain, Do: func() { setNotice(c.Connect()) }},
		{Label: "Trennen", Role: sharedshell.Plain, Do: func() { setNotice(c.Disconnect()) }},
		{Label: "Neu pairen", Role: sharedshell.Plain, Do: func() { setNotice(c.PairAgain()) }},
		{Label: "Vergessen", Role: sharedshell.Danger, Do: func() {
			shell.Get().Push(clawForgetPage(c, func() { draftMu.Lock(); endpoint, token, notice = "", "", ""; dirty = false; draftMu.Unlock() }))
		}},
		{Label: "Speichern", Role: sharedshell.Primary, Do: func() {
			draftMu.Lock()
			u, t := endpoint, token
			draftMu.Unlock()
			err := c.UpdateConnection(u, t)
			setNotice(err)
			if err == nil {
				draftMu.Lock()
				dirty = false
				notice = "Gespeichert."
				draftMu.Unlock()
			}
		}},
	}
	return page
}
func clawForgetPage(c *openclaw.Client, forgotten func()) *sharedshell.Page {
	return &sharedshell.Page{
		Title: "OpenClaw lokal vergessen?",
		Build: func() ([]menu.Row, []func(int)) {
			return []menu.Row{{Label: "Nur lokal löschen", Hint: "Die Gateway-Freigabe bleibt bestehen und muss dort separat widerrufen werden.", Kind: menu.Plain}}, []func(int){nil}
		},
		Actions: []sharedshell.Action{
			{Label: "Abbrechen", Role: sharedshell.Plain, Do: func() { shell.Get().Pop() }},
			{Label: "Zugangsdaten löschen", Role: sharedshell.Danger, Do: func() {
				if c.Forget() == nil {
					forgotten()
					shell.Get().Pop()
				}
			}},
		},
	}
}
func clawDiagnostics(c *openclaw.Client) *sharedshell.Page {
	return &sharedshell.Page{
		Title: "OpenClaw Diagnose",
		Build: func() ([]menu.Row, []func(int)) {
			var rows []menu.Row
			var acts []func(int)
			events := c.Events()
			for i := len(events) - 1; i >= 0; i-- {
				e := events[i]
				rows = append(rows, menu.Row{Label: fmt.Sprintf("%s %s %s", e.At.Format("15:04:05"), e.Direction, e.Type), Hint: e.Result, Kind: menu.Plain})
				acts = append(acts, nil)
			}
			return rows, acts
		},
		Actions: []sharedshell.Action{{Label: "Diagnose löschen", Role: sharedshell.Danger, Do: c.ClearEvents}},
	}
}

func clawState(state string) string {
	switch state {
	case "connected":
		return "Verbunden"
	case "connecting":
		return "Verbindung wird aufgebaut"
	case "reconnecting":
		return "Wiederverbinden"
	case "pairing":
		return "Gateway-Freigabe erforderlich"
	case "error":
		return "Verbindung fehlgeschlagen"
	default:
		return "Getrennt"
	}
}
func clawReason(reason string) string {
	switch reason {
	case "invalid_endpoint":
		return "Bitte eine gültige wss://-Adresse ohne Zugangsdaten, Query oder Fragment eingeben."
	case "credentials_missing":
		return "Bitte zuerst den Gateway-Token verdeckt eingeben und speichern."
	case "approval_required":
		return "Das Gerät im Gateway freigeben, danach hier erneut prüfen."
	case "authentication":
		return "Zugangsdaten wurden abgewiesen. Token und Gerätefreigabe prüfen."
	case "tls":
		return "TLS-Zertifikat oder Servername konnte nicht bestätigt werden."
	case "dns":
		return "Der Gateway-Name konnte nicht aufgelöst werden."
	case "timeout":
		return "Die Verbindung hat das Zeitlimit überschritten."
	case "network":
		return "Netzwerkverbindung unterbrochen oder Gateway nicht erreichbar."
	case "unavailable":
		return "Gateway vorübergehend nicht verfügbar."
	case "protocol":
		return "Das Gateway-Protokoll ist nicht kompatibel."
	case "identity":
		return "Die lokale Geräteidentität ist ungültig."
	case "storage":
		return "Lokale Zugangsdaten konnten nicht gelesen oder gespeichert werden."
	case "settings":
		return "Bitte Verbindungseinstellungen und Zugangsdaten prüfen."
	default:
		return ""
	}
}
