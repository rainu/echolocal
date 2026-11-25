//go:build !board_biscuit

package settings

import (
	"strconv"
	"sync"

	"github.com/ygelfand/echolocal/internal/feature/localvideo"
	"github.com/ygelfand/echolocal/internal/feature/shell"
	"github.com/ygelfand/echolocal/internal/networkmedia"
	"github.com/ygelfand/libcountertop/pkg/display/menu"
	sharedshell "github.com/ygelfand/libcountertop/pkg/display/shell"
)

func smbPage() *sharedshell.Page { return smbPageFor(localvideo.Source(), shell.Get().Redraw) }
func smbPageFor(source *networkmedia.Store, redraw func()) *sharedshell.Page {
	var mu sync.Mutex
	draft := source.View().Settings
	password := ""
	dirty := false
	notice := ""
	labels := networkmedia.Labels()
	page := &sharedshell.Page{Title: labels["title"]}
	page.Build = func() ([]menu.Row, []func(int)) {
		mu.Lock()
		defer mu.Unlock()
		if !dirty {
			draft = source.View().Settings
		}
		rows := []menu.Row{{Label: labels["help"], Kind: menu.Plain}, {Label: labels["enabled"], Kind: menu.Toggle, On: draft.Enabled}}
		edit := func(k, value string, set func(string)) {
			rows = append(rows, menu.Row{Label: labels[k], Value: value, Kind: menu.Field, Save: func(v string) { mu.Lock(); set(v); dirty = true; mu.Unlock() }})
		}
		edit("host", draft.Host, func(v string) { draft.Host = v })
		edit("port", strconv.Itoa(draft.Port), func(v string) { draft.Port, _ = strconv.Atoi(v) })
		edit("share", draft.Share, func(v string) { draft.Share = v })
		edit("username", draft.Username, func(v string) { draft.Username = v })
		edit("domain", draft.Domain, func(v string) { draft.Domain = v })
		rows = append(rows, menu.Row{Label: labels["password"], Value: password, Kind: menu.Password, Save: func(v string) { mu.Lock(); password = v; dirty = true; mu.Unlock() }})
		key := "password_unset"
		if source.View().PasswordSet {
			key = "password_set"
		}
		rows = append(rows, menu.Row{Label: labels[key], Kind: menu.Plain})
		if notice != "" {
			rows = append(rows, menu.Row{Label: notice, Kind: menu.Plain})
		}
		taps := make([]func(int), len(rows))
		taps[1] = func(int) { mu.Lock(); draft.Enabled = !draft.Enabled; dirty = true; mu.Unlock() }
		return rows, taps
	}
	save := func(clear bool) {
		mu.Lock()
		u := networkmedia.Update{Settings: draft, ClearPassword: clear}
		if clear {
			u.Enabled = false
		} else if password != "" {
			v := password
			u.Password = &v
		}
		mu.Unlock()
		e := source.Save(u)
		mu.Lock()
		notice = labels["saved"]
		if e != nil {
			notice = labels["error"] + ": " + e.Error()
		} else {
			draft = source.View().Settings
			password = ""
			dirty = false
		}
		mu.Unlock()
		redraw()
	}
	page.Actions = []sharedshell.Action{
		{Label: labels["save"], MinHeight: 64, Role: sharedshell.Primary, Do: func() { save(false) }},
		{Label: labels["clear"], MinHeight: 64, Role: sharedshell.Danger, Do: func() { save(true) }},
	}
	return page
}
