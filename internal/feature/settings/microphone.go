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
func microphoneTitle() string { return micText("Microphone / test", "Mikrofon / Test") }
func microphonePage() *sharedshell.Page {
	return &sharedshell.Page{Title: microphoneTitle(), Build: func() ([]menu.Row, []func(int)) {
		rows := []menu.Row{{Label: micText("Record and listen (processed)", "Aufnehmen und anhören (verarbeitet)"), Kind: menu.Chevron, Open: func() sharedshell.View { return microphoneTestPage() }}}
		taps := []func(int){nil}
		at := config.Get().Microphone
		for _, s := range microphone.Table(microphone.Mixings()).Rows() {
			row := menu.Row{Label: s.Title(), Dim: test.Active()}
			var tap func(int)
			save := func(v string) {
				if test.Active() {
					return
				}
				_ = microphone.Change(s.Name, v)
				shell.Get().Redraw()
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
					if test.Active() {
						return
					}
					shell.Get().Push(&sharedshell.Page{Title: s.Title(), Build: func() ([]menu.Row, []func(int)) {
						var rs []menu.Row
						var ts []func(int)
						for _, c := range choices {
							rs = append(rs, menu.Row{Label: s.Label(c), Chosen: c.Value == s.Read(&at)})
							ts = append(ts, func(int) { save(c.Value); shell.Get().Pop() })
						}
						return rs, ts
					}})
				}
			}
			rows = append(rows, row)
			taps = append(taps, tap)
		}
		return rows, taps
	}}
}

func microphoneTestPage() *sharedshell.Page {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		panic(err)
	}
	owner := hex.EncodeToString(id[:])
	var stop func()
	var p *sharedshell.Page
	var commandError string
	command := func(action string) {
		if err := test.Get().Command(owner, action); err != nil {
			commandError = err.Error()
		} else {
			commandError = ""
		}
		shell.Get().Redraw()
	}
	p = &sharedshell.Page{Title: micText("Microphone test", "Mikrofontest"), Build: func() ([]menu.Row, []func(int)) {
		s := test.Get().State(owner)
		phase := map[string]string{"idle": micText("Ready to record", "Bereit zur Aufnahme"), "recording": micText("● RECORDING — speak now (max 8 s)", "● AUFNAHME — jetzt sprechen (max. 8 s)"), "ready": micText("Ready to play", "Bereit zur Wiedergabe"), "playing": micText("Playing on this Echo", "Wiedergabe auf diesem Echo"), "error": micText("Test stopped", "Test gestoppt")}[s.Phase]
		rows := []menu.Row{{Label: phase, Value: fmt.Sprintf("%.1f s", s.Seconds)}}
		if s.Error != "" {
			rows = append(rows, menu.Row{Label: micText("Error: ", "Fehler: ") + s.Error})
		}
		if commandError != "" {
			rows = append(rows, menu.Row{Label: micText("Command: ", "Befehl: ") + commandError})
		}
		taps := make([]func(int), len(rows))
		active := s.Phase == "recording" || s.Phase == "playing"
		add := func(label, action string, enabled bool) {
			rows = append(rows, menu.Row{Label: label, Dim: !enabled})
			taps = append(taps, func(int) {
				if enabled {
					command(action)
				}
			})
		}
		add(micText("Record / replace take (8 s)", "Aufnehmen / Aufnahme ersetzen (8 s)"), "record", !active)
		add(micText("Stop", "Stopp"), "stop", s.Owned && active)
		add(micText("Play on Echo", "Auf Echo anhören"), "play", s.Owned && s.Phase == "ready")
		add(micText("Delete take", "Aufnahme löschen"), "discard", s.Owned)
		rows = append(rows, menu.Row{Label: micText("Processed recognition signal. RAM only; deleted on exit or after 2 minutes. Wake words pause during capture/playback.", "Verarbeitetes Erkennungssignal. Nur im RAM; Löschen beim Verlassen oder nach 2 Minuten. Aktivierungswörter pausieren bei Aufnahme/Wiedergabe.")})
		taps = append(taps, nil)
		if s.Processing != "" {
			rows = append(rows, menu.Row{Label: s.Processing})
			taps = append(taps, nil)
		}
		return rows, taps
	}}
	p.OnOpen = func() {
		if stop != nil {
			stop()
		}
		done := make(chan struct{})
		unlisten := test.Changed.Listen(func(struct{}) { shell.Get().Redraw() })
		var unwatch func()
		stop = sync.OnceFunc(func() {
			close(done)
			unlisten()
			if unwatch != nil {
				unwatch()
			}
			_ = test.Get().Command(owner, "discard")
		})
		unwatch = shell.Get().Changed.Listen(func(c sharedshell.Change) {
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
					_ = test.Get().Command(owner, "ping")
					shell.Get().Redraw()
				}
			}
		}()
	}
	return p
}
