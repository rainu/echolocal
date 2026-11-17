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
	var endpoint, token, notice string
	setNotice := func(e error) {
		draftMu.Lock()
		defer draftMu.Unlock()
		notice = ""
		if e != nil {
			notice = e.Error()
		}
	}
	return &sharedshell.Page{Title: "OpenClaw", Build: func() ([]menu.Row, []func(int)) {
		s := c.Status()
		draftMu.Lock()
		note := notice
		draftMu.Unlock()
		rows := []menu.Row{
			{Label: "Verbindung", Value: s.State, Hint: s.Reason, Kind: menu.Plain},
			{Label: "Geräte-Pairing", Hint: "1. WSS-Adresse und Token eingeben. 2. Verbinden. 3. Gerät im Gateway freigeben, dann erneut verbinden.", Kind: menu.Plain},
			{Label: "Gateway-Adresse", Hint: "Nur wss://; wird privat gespeichert. Gespeicherte Adresse wird nicht angezeigt.", Kind: menu.Password, Save: func(v string) { draftMu.Lock(); endpoint = v; draftMu.Unlock() }},
			{Label: "Gateway-Token", Hint: "Verdeckte Eingabe; nach dem Pairing wird der Gerätetoken verwendet.", Kind: menu.Password, Save: func(v string) { draftMu.Lock(); token = v; draftMu.Unlock() }},
			{Label: "Eingaben speichern", Hint: note, Kind: menu.Chevron},
			{Label: "Verbinden / erneut prüfen", Hint: "Bei ausstehender Freigabe: erst im Gateway genehmigen, danach hier erneut prüfen.", Kind: menu.Chevron},
			{Label: "Trennen", Hint: "Deaktiviert auch das automatische Wiederverbinden nach Neustart.", Kind: menu.Chevron},
			{Label: "Erneut pairen", Hint: "Identität bleibt erhalten; ein widerrufener Gerätetoken benötigt einen neuen Gateway-Token.", Kind: menu.Chevron},
			{Label: "Verbindung vergessen", Hint: "Löscht nur lokale Zugangsdaten. Eine Gateway-Freigabe muss dort separat widerrufen werden.", Kind: menu.Chevron, Open: func() menu.View { return clawForgetPage(c) }},
			{Label: "Diagnose", Hint: "64 technische Ereignisse im Arbeitsspeicher, ohne Inhalte oder Zugangsdaten.", Kind: menu.Chevron, Open: func() menu.View { return clawDiagnostics(c) }},
		}
		return rows, []func(int){nil, nil, nil, nil, func(int) {
			draftMu.Lock()
			u, t := endpoint, token
			token = ""
			draftMu.Unlock()
			setNotice(c.Configure(u, t))
		}, func(int) { setNotice(c.Connect()) }, func(int) { setNotice(c.Disconnect()) }, func(int) { setNotice(c.PairAgain()) }, nil, nil}
	}}
}
func clawForgetPage(c *openclaw.Client) *sharedshell.Page {
	return &sharedshell.Page{Title: "OpenClaw lokal vergessen?", Build: func() ([]menu.Row, []func(int)) {
		return []menu.Row{{Label: "Abbrechen", Kind: menu.Chevron}, {Label: "Lokale Zugangsdaten endgültig löschen", Hint: "Die Gateway-Freigabe bleibt bestehen.", Kind: menu.Chevron}}, []func(int){func(int) { shell.Get().Pop() }, func(int) {
			if c.Forget() == nil {
				shell.Get().Pop()
			}
		}}
	}}
}
func clawDiagnostics(c *openclaw.Client) *sharedshell.Page {
	return &sharedshell.Page{Title: "OpenClaw Diagnose", Build: func() ([]menu.Row, []func(int)) {
		rows := []menu.Row{{Label: "Diagnose löschen", Kind: menu.Chevron}}
		acts := []func(int){func(int) { c.ClearEvents() }}
		events := c.Events()
		for i := len(events) - 1; i >= 0; i-- {
			e := events[i]
			rows = append(rows, menu.Row{Label: fmt.Sprintf("%s %s %s", e.At.Format("15:04:05"), e.Direction, e.Type), Hint: e.Result, Kind: menu.Plain})
			acts = append(acts, nil)
		}
		return rows, acts
	}}
}
