//go:build !board_biscuit

package settings

import (
	featureclaw "github.com/ygelfand/echolocal/internal/feature/openclaw"
	"github.com/ygelfand/echolocal/internal/feature/shell"
	"github.com/ygelfand/echolocal/internal/openclaw"
	"github.com/ygelfand/libcountertop/pkg/display/menu"
	sharedshell "github.com/ygelfand/libcountertop/pkg/display/shell"
)

func nodePermissionsPage() *sharedshell.Page {
	return nodePermissionsPageFor(featureclaw.Get().Node)
}
func nodePermissionsPageFor(c *openclaw.Client) *sharedshell.Page {
	hints := map[string]string{
		"desktop": "Nur Ansicht · standardmäßig aus · keine Fernbedienung",
		"talk":    "Konfigurierte Sprachassistenten starten/stoppen", "screen": "Bildschirmaufnahmen lesen", "input": "Tippen, Wischen und Texteingabe",
		"display": "Nachrichten und Bildschirmseiten steuern", "settings": "Nicht geheime Geräteeinstellungen lesen/ändern",
		"audio": "Medien und Lautstärke steuern", "alarms": "Wecker und Timer steuern",
		"intercom": "Anrufe und Durchsagen", "camera": "Kamerabilder und Kameraeinstellungen",
		"state": "Gerätezustand lesen", "diagnostics": "Begrenzte, bereinigte Diagnose",
		"maintenance": "EchoLocal-Wartung ausführen", "tones": "Benutzerdefinierte Töne verwalten",
	}
	labels := map[string]string{"desktop": "Live-Bildschirmfreigabe", "talk": "Sprachmodus", "screen": "Bildschirm", "input": "Fernbedienung", "display": "Anzeige und Nachrichten", "settings": "Einstellungen", "audio": "Audio und Medien", "alarms": "Wecker und Timer", "intercom": "Gegensprechen", "camera": "Kamera", "state": "Gerätezustand", "diagnostics": "Diagnose", "maintenance": "Wartung", "tones": "Eigene Töne"}
	var notice string
	apply := func(g map[string]bool) {
		if c.SetNodeGrants(g) != nil {
			notice = "Speichern fehlgeschlagen"
		} else {
			notice = ""
		}
		shell.Get().Redraw()
	}
	all := func(on bool) {
		g := map[string]bool{}
		for _, k := range openclaw.PermissionGroups {
			g[k] = on
		}
		apply(g)
	}
	return &sharedshell.Page{Title: connectionText("permissions"), Build: func() ([]menu.Row, []func(int)) {
		rows := []menu.Row{{Label: connectionText("permissionsHint") + " · Stream: " + c.DesktopStatus().State, Kind: menu.Plain}}
		acts := []func(int){nil}
		for _, g := range openclaw.PermissionGroups {
			rows = append(rows, menu.Row{Label: labels[g], Hint: hints[g], Kind: menu.Toggle, On: c.NodeAccess().Granted(g)})
			acts = append(acts, func(int) { apply(map[string]bool{g: !c.NodeAccess().Granted(g)}) })
		}
		if notice != "" {
			rows = append(rows, menu.Row{Label: notice, Kind: menu.Plain})
			acts = append(acts, nil)
		}
		return rows, acts
	}, Actions: []sharedshell.Action{
		{Label: "Stream stoppen", Role: sharedshell.Plain, Do: func() { c.StopDesktop(); shell.Get().Redraw() }},
		{Label: "Alle erlauben", Role: sharedshell.Plain, Do: func() { all(true) }},
		{Label: "Alle sperren", Role: sharedshell.Danger, Do: func() { all(false) }},
	}}
}
