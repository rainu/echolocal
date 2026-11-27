//go:build !board_biscuit

package settings

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/ygelfand/echolocal/internal/config"
	"github.com/ygelfand/echolocal/internal/feature/microphone"
	test "github.com/ygelfand/echolocal/internal/feature/mictest"
	"github.com/ygelfand/echolocal/internal/feature/shell"
	core "github.com/ygelfand/echolocal/internal/mictest"
	"github.com/ygelfand/libcountertop/pkg/display/menu"
	sharedshell "github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/say"
	setting "github.com/ygelfand/libcountertop/pkg/settings"
)

func micText(en, de string) string {
	if say.Chosen() == "de" {
		return de
	}
	return en
}
func microphoneTitle() string { return micText("Microphone", "Mikrofon") }
func microphoneRows(sh *sharedshell.Shell, active func() bool) ([]menu.Row, []func(int)) {
	var rows []menu.Row
	var taps []func(int)
	at := config.Get().Microphone
	for _, s := range microphone.Table(microphone.Mixings()).Rows() {
		row := menu.Row{Label: s.Title(), Dim: active()}
		var tap func(int)
		save := func(v string) {
			if active() {
				return
			}
			_ = microphone.Change(s.Name, v)
			sh.Redraw()
		}
		switch s.Kind {
		case setting.Toggle:
			row.Kind = menu.Toggle
			row.On = s.On(&at)
			on := row.On
			tap = func(int) { save(setting.OnOff(!on)) }
		case setting.Number:
			n, _ := strconv.Atoi(s.Read(&at))
			row.Kind = menu.Slider
			row.Value = fmt.Sprintf("%d %s", n, s.Unit)
			row.Level = (n - s.Min) * 100 / (s.Max - s.Min)
			row.Snap = func(v int) int { n := (v*(s.Max-s.Min) + 50) / 100; return n * 100 / (s.Max - s.Min) }
			tap = func(v int) { save(strconv.Itoa(s.Min + (v*(s.Max-s.Min)+50)/100)) }
		case setting.Choice:
			choices := s.Choices()
			row.Kind = menu.Chevron
			row.Value = s.Read(&at)
			tap = func(int) {
				if active() {
					return
				}
				sh.Push(&sharedshell.Page{Title: s.Title(), Build: func() ([]menu.Row, []func(int)) {
					var rs []menu.Row
					var ts []func(int)
					for _, c := range choices {
						rs = append(rs, menu.Row{Label: s.Label(c), Chosen: c.Value == s.Read(&at)})
						ts = append(ts, func(int) { save(c.Value); sh.Pop() })
					}
					return rs, ts
				}})
			}
		}
		rows = append(rows, row)
		taps = append(taps, tap)
	}
	return rows, taps
}

type microphoneTestControl interface {
	State(string) core.State
	Command(string, string) error
}

func microphonePage() *sharedshell.Page { return microphonePageWith(test.Get(), shell.Get()) }
func microphonePageWith(m microphoneTestControl, sh *sharedshell.Shell) *sharedshell.Page {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		panic(err)
	}
	owner := hex.EncodeToString(id[:])
	var stop func()
	var commandError string
	command := func(action string) {
		if err := m.Command(owner, action); err != nil {
			commandError = err.Error()
		} else {
			commandError = ""
		}
		sh.Redraw()
	}
	active := func() bool { s := m.State(owner); return s.Phase == "recording" || s.Phase == "playing" }
	canRecord := func() bool { s := m.State(owner); return !active() && (s.Owned || s.Seconds == 0) }
	canPlay := func() bool { s := m.State(owner); return s.Owned && s.Phase == "ready" }
	canStop := func() bool { s := m.State(owner); return s.Owned && active() }
	p := &sharedshell.Page{Title: microphoneTitle(), Build: func() ([]menu.Row, []func(int)) {
		rows, taps := microphoneRows(sh, active)
		s := m.State(owner)
		phase := map[string]string{
			"idle":      micText("Microphone test — ready to record", "Mikrofontest — bereit zur Aufnahme"),
			"recording": micText("● RECORDING — speak now (max 8 s)", "● AUFNAHME — jetzt sprechen (max. 8 s)"),
			"ready":     micText("Microphone test — ready to play", "Mikrofontest — bereit zur Wiedergabe"),
			"playing":   micText("Playing on this Echo", "Wiedergabe auf diesem Echo"),
			"error":     micText("Microphone test stopped", "Mikrofontest gestoppt"),
		}[s.Phase]
		add := func(row menu.Row) { rows = append(rows, row); taps = append(taps, nil) }
		add(menu.Row{Label: phase, Value: fmt.Sprintf("%.1f s", s.Seconds)})
		if s.Error != "" {
			add(menu.Row{Label: micText("Error: ", "Fehler: ") + s.Error})
		}
		if commandError != "" {
			add(menu.Row{Label: micText("Command: ", "Befehl: ") + commandError})
		}
		add(menu.Row{Label: micText("Processed signal · RAM only · replaced on Start; cleared on exit or after 2 minutes.", "Verarbeitetes Signal · nur im RAM · Starten ersetzt die Aufnahme; Löschen beim Verlassen oder nach 2 Minuten.")})
		if s.Processing != "" {
			add(menu.Row{Label: s.Processing})
		}
		return rows, taps
	}, Actions: []sharedshell.Action{
		{Label: micText("Start recording", "Starten"), Glyph: "mdi:microphone", MinHeight: 64, Enabled: canRecord, Do: func() { command("record") }},
		{Label: micText("Play", "Abspielen"), Glyph: "mdi:play", MinHeight: 64, Role: sharedshell.Plain, Enabled: canPlay, Do: func() { command("play") }},
		{Label: micText("Stop", "Stoppen"), Glyph: "mdi:stop", MinHeight: 64, Role: sharedshell.Plain, Enabled: canStop, Do: func() { command("stop") }},
	}}
	p.OnOpen = func() {
		if stop != nil {
			stop()
		}
		done := make(chan struct{})
		unlisten := test.Changed.Listen(func(struct{}) { sh.Redraw() })
		var unwatch func()
		stop = sync.OnceFunc(func() {
			close(done)
			unlisten()
			if unwatch != nil {
				unwatch()
			}
			_ = m.Command(owner, "discard")
		})
		unwatch = sh.Changed.Listen(func(c sharedshell.Change) {
			if c.From == p && c.To != p {
				stop()
			}
		})
		go func() {
			tick := time.NewTicker(time.Second)
			defer tick.Stop()
			for {
				select {
				case <-done:
					return
				case <-tick.C:
					_ = m.Command(owner, "ping")
					sh.Redraw()
				}
			}
		}()
	}
	return p
}
