//go:build !board_biscuit

package settings

import (
	"fmt"
	"strings"
	"sync"

	"github.com/ygelfand/echolocal/internal/feature/clawtalk"
	featureclaw "github.com/ygelfand/echolocal/internal/feature/openclaw"
	"github.com/ygelfand/echolocal/internal/feature/shell"
	"github.com/ygelfand/echolocal/internal/openclaw"
	"github.com/ygelfand/libcountertop/pkg/display/menu"
	sharedshell "github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/say"
)

func claw() *openclaw.Client { return featureclaw.Get().Client }

func openclawPage() *sharedshell.Page {
	return &sharedshell.Page{Title: say.T("openclaw.title"), Build: func() ([]menu.Row, []func(int)) {
		rows := []menu.Row{{Label: say.T("openclaw.connection"), Kind: menu.Chevron, Open: func() menu.View { return openclawClientPage(claw()) }}}
		rows = append(rows, menu.Row{Label: "OpenClaw Node", Kind: menu.Chevron, Open: func() menu.View { p := openclawClientPage(featureclaw.Get().Node); p.Title = "OpenClaw Node"; return p }})
		rows = append(rows, menu.Row{Label: "Node-Berechtigungen", Kind: menu.Chevron, Open: func() menu.View { return nodePermissionsPage() }})
		for slot := 0; slot < 2; slot++ {
			rows = append(rows, menu.Row{Label: say.F("assistants.slot", map[string]any{"N": slot + 1}), Kind: menu.Chevron, Open: func() menu.View { return talkOptionsPage(slot) }})
		}
		rows = append(rows, menu.Row{Label: say.T("clawtalk.history"), Hint: say.T("clawtalk.history_local"), Kind: menu.Chevron, Open: func() menu.View { return clawtalk.Get().HistoryPage() }})
		return rows, make([]func(int), len(rows))
	}}
}

func openclawClientPage(c *openclaw.Client) *sharedshell.Page {
	var draftMu sync.Mutex
	endpoint, token := c.EditableConnection()
	var notice string
	dirty := false
	setNotice := func(e error) {
		draftMu.Lock()
		defer draftMu.Unlock()
		notice = ""
		if e != nil {
			notice = e.Error()
		}
	}
	page := &sharedshell.Page{Title: say.T("openclaw.title")}
	page.Build = func() ([]menu.Row, []func(int)) {
		s := c.Status()
		storedURL, storedToken := c.EditableConnection()
		draftMu.Lock()
		if !dirty {
			endpoint, token = storedURL, storedToken
		}
		u, t, note := endpoint, token, notice
		draftMu.Unlock()
		if note == "saved" {
			note = say.T("openclaw.notice.saved")
		} else {
			note = clawReason(note)
		}
		rows := []menu.Row{
			{Label: say.T("openclaw.connection"), Value: clawState(s.State), Hint: clawReason(s.Reason), Kind: menu.Plain},
			{Label: say.T("openclaw.pairing.title"), Hint: say.T("openclaw.pairing.hint"), Kind: menu.Plain},
			{Label: say.T("openclaw.endpoint.label"), Hint: say.T("openclaw.endpoint.hint"), Value: u, Kind: menu.Field, Save: func(v string) { draftMu.Lock(); endpoint = v; dirty = true; draftMu.Unlock() }},
			{Label: say.T("openclaw.token.label"), Hint: say.T("openclaw.token.hint"), Value: t, Kind: menu.Password, Save: func(v string) { draftMu.Lock(); token = v; dirty = true; draftMu.Unlock() }},
			{Label: say.T("openclaw.diagnostics.label"), Hint: say.T("openclaw.diagnostics.hint"), Kind: menu.Chevron, Open: func() menu.View { return clawDiagnostics(c) }},
		}
		if note != "" {
			rows = append(rows, menu.Row{Label: note, Kind: menu.Plain})
		}
		return rows, make([]func(int), len(rows))
	}
	page.Actions = []sharedshell.Action{
		{Label: say.T("openclaw.action.connect"), MinHeight: 64, Role: sharedshell.Plain, Do: func() { setNotice(c.Connect()) }},
		{Label: say.T("openclaw.action.disconnect"), MinHeight: 64, Role: sharedshell.Plain, Do: func() { setNotice(c.Disconnect()) }},
		{Label: say.T("openclaw.action.pair_again"), MinHeight: 64, Role: sharedshell.Plain, Do: func() { setNotice(c.PairAgain()) }},
		{Label: say.T("openclaw.action.forget"), MinHeight: 64, Role: sharedshell.Danger, Do: func() {
			shell.Get().Push(clawForgetPage(c, func() { draftMu.Lock(); endpoint, token, notice = "", "", ""; dirty = false; draftMu.Unlock() }))
		}},
		{Label: say.T("openclaw.action.save"), MinHeight: 64, Role: sharedshell.Primary, Do: func() {
			draftMu.Lock()
			u, t := endpoint, token
			draftMu.Unlock()
			err := c.UpdateConnection(u, t)
			setNotice(err)
			if err == nil {
				draftMu.Lock()
				dirty = false
				notice = "saved"
				draftMu.Unlock()
			}
		}},
	}
	return page
}
func clawForgetPage(c *openclaw.Client, forgotten func()) *sharedshell.Page {
	return &sharedshell.Page{
		Title: say.T("openclaw.forget.title"),
		Build: func() ([]menu.Row, []func(int)) {
			return []menu.Row{{Label: say.T("openclaw.forget.label"), Hint: say.T("openclaw.forget.hint"), Kind: menu.Plain}}, []func(int){nil}
		},
		Actions: []sharedshell.Action{
			{Label: say.T("openclaw.action.cancel"), MinHeight: 64, Role: sharedshell.Plain, Do: func() { shell.Get().Pop() }},
			{Label: say.T("openclaw.action.delete"), MinHeight: 64, Role: sharedshell.Danger, Do: func() {
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
		Title: say.T("openclaw.diagnostics.title"),
		Build: func() ([]menu.Row, []func(int)) {
			var rows []menu.Row
			var acts []func(int)
			events := c.Events()
			for i := len(events) - 1; i >= 0; i-- {
				e := events[i]
				rows = append(rows, menu.Row{Label: fmt.Sprintf("%s %s %s", e.At.Format("15:04:05"), clawDirection(e.Direction), clawEvent(e.Type)), Hint: clawResult(e.Result), Kind: menu.Plain})
				acts = append(acts, nil)
			}
			return rows, acts
		},
		Actions: []sharedshell.Action{{Label: say.T("openclaw.diagnostics.clear"), MinHeight: 64, Role: sharedshell.Danger, Do: c.ClearEvents}},
	}
}

func clawState(state string) string {
	switch state {
	case "connected":
		return say.T("openclaw.state.connected")
	case "connecting":
		return say.T("openclaw.state.connecting")
	case "reconnecting":
		return say.T("openclaw.state.reconnecting")
	case "pairing":
		return say.T("openclaw.state.pairing")
	case "error":
		return say.T("openclaw.state.error")
	default:
		return say.T("openclaw.state.disconnected")
	}
}
func clawReason(reason string) string {
	switch reason {
	case "invalid_endpoint":
		return say.T("openclaw.error.invalid_endpoint")
	case "credentials_missing":
		return say.T("openclaw.error.credentials_missing")
	case "approval_required":
		return say.T("openclaw.error.approval_required")
	case "authentication":
		return say.T("openclaw.error.authentication")
	case "tls":
		return say.T("openclaw.error.tls")
	case "dns":
		return say.T("openclaw.error.dns")
	case "timeout":
		return say.T("openclaw.error.timeout")
	case "network":
		return say.T("openclaw.error.network")
	case "unavailable":
		return say.T("openclaw.error.unavailable")
	case "protocol":
		return say.T("openclaw.error.protocol")
	case "identity":
		return say.T("openclaw.error.identity")
	case "storage":
		return say.T("openclaw.error.storage")
	case "settings":
		return say.T("openclaw.error.settings")
	default:
		return ""
	}
}

func clawDirection(direction string) string {
	switch direction {
	case "in", "out", "local":
		return say.T("openclaw.diagnostics.direction." + direction)
	}
	return say.T("openclaw.diagnostics.direction.unknown")
}
func clawEvent(kind string) string {
	switch kind {
	case "connect", "settings_saved", "disconnect", "pair_again", "forget", "stop", "backoff", "upgrade", "transport", "challenge", "hello", "credentials", "close", "shutdown", "application_event":
		return say.T("openclaw.event." + kind)
	}
	return say.T("openclaw.event.application_event")
}
func clawResult(result string) string {
	if result == "received" {
		return say.T("openclaw.diagnostics.received")
	}
	state, reason, _ := strings.Cut(result, ":")
	switch state {
	case "connected", "connecting", "reconnecting", "pairing", "error", "disconnected":
		text := clawState(state)
		if detail := clawReason(reason); detail != "" {
			return text + " · " + detail
		}
		return text
	}
	return say.T("openclaw.diagnostics.unknown")
}
