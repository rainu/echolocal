//go:build !board_biscuit

package settings

import (
	"context"
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
	testing := false
	var cancelTest context.CancelFunc
	labels := networkmedia.Labels()
	page := &sharedshell.Page{Title: labels["title"]}
	page.Build = func() ([]menu.Row, []func(int)) {
		mu.Lock()
		defer mu.Unlock()
		if !dirty {
			latest := source.View().Settings
			if draft != latest {
				notice = ""
			}
			draft = latest
		}
		rows := []menu.Row{{Label: labels["help"], Kind: menu.Plain}, {Label: labels["enabled"], Kind: menu.Toggle, On: draft.Enabled}}
		edit := func(k, value string, set func(string)) {
			rows = append(rows, menu.Row{Label: labels[k], Value: value, Kind: menu.Field, Save: func(v string) { mu.Lock(); set(v); dirty = true; notice = ""; mu.Unlock() }})
		}
		edit("host", draft.Host, func(v string) { draft.Host = v })
		edit("port", strconv.Itoa(draft.Port), func(v string) { draft.Port, _ = strconv.Atoi(v) })
		edit("share", draft.Share, func(v string) { draft.Share = v })
		edit("username", draft.Username, func(v string) { draft.Username = v })
		edit("domain", draft.Domain, func(v string) { draft.Domain = v })
		rows = append(rows, menu.Row{Label: labels["password"], Value: password, Kind: menu.Password, Save: func(v string) { mu.Lock(); password = v; dirty = true; notice = ""; mu.Unlock() }})
		key := "password_unset"
		if source.View().PasswordSet {
			key = "password_set"
		}
		rows = append(rows, menu.Row{Label: labels[key], Kind: menu.Plain})
		if notice != "" {
			rows = append(rows, menu.Row{Label: notice, Kind: menu.Plain})
		}
		taps := make([]func(int), len(rows))
		taps[1] = func(int) { mu.Lock(); draft.Enabled = !draft.Enabled; dirty = true; notice = ""; mu.Unlock() }
		return rows, taps
	}
	save := func(clear bool) {
		mu.Lock()
		if testing {
			mu.Unlock()
			return
		}
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
	page.Busy = func() (sharedshell.Busy, bool) {
		mu.Lock()
		defer mu.Unlock()
		return sharedshell.Busy{Text: labels["testing"], Outcome: sharedshell.Working, Buttons: []sharedshell.BusyButton{{ID: "cancel", Label: labels["test_cancel"], Do: func() {
			mu.Lock()
			cancel := cancelTest
			mu.Unlock()
			if cancel != nil {
				cancel()
			}
		}}}}, testing
	}
	test := func() {
		mu.Lock()
		if testing {
			mu.Unlock()
			return
		}
		u := networkmedia.Update{Settings: draft}
		if password != "" {
			v := password
			u.Password = &v
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancelTest = cancel
		testing = true
		notice = ""
		mu.Unlock()
		redraw()
		go func() {
			result := source.Test(ctx, u)
			cancel()
			mu.Lock()
			testing = false
			cancelTest = nil
			notice = result.Message
			mu.Unlock()
			redraw()
		}()
	}
	page.Actions = []sharedshell.Action{
		{Label: labels["save"], MinHeight: 64, Role: sharedshell.Primary, Do: func() { save(false) }},
		{Label: labels["test"], MinHeight: 64, Role: sharedshell.Plain, Do: test},
		{Label: labels["clear"], MinHeight: 64, Role: sharedshell.Danger, Do: func() { save(true) }},
	}
	return page
}
