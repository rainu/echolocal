//go:build !board_biscuit

// Package filepicker presents the shared touch-menu file/directory picker.
package filepicker

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ygelfand/libcountertop/pkg/display/menu"
	"github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/say"

	"github.com/ygelfand/echolocal/internal/filepicker"
)

type Config struct {
	Title   string
	Browser filepicker.Options
	Choose  func(string) error
	Shell   *shell.Shell
}

func text(en, de string) string {
	if say.Chosen() == "de" {
		return de
	}
	return en
}
func New(cfg Config) *shell.Page {
	b := filepicker.New(cfg.Browser)
	p := &shell.Page{Title: cfg.Title}
	loaded := false
	note := ""
	refresh := func() { cfg.Shell.Redraw() }
	navigate := func(name string) {
		_ = b.Navigate(name)
		note = ""
		p.Reveal = text("Location", "Ort")
		refresh()
	}
	p.OnOpen = func() {
		if !loaded {
			b.Start()
			loaded = true
		}
	}
	p.Actions = []shell.Action{
		{Label: text("Select", "Auswählen"), MinHeight: 64, Role: shell.Primary, Do: func() {
			name, err := b.Confirm()
			if err == nil && cfg.Choose != nil {
				err = cfg.Choose(name)
			}
			if err != nil {
				note = text(
					"Selection unavailable or invalid.",
					"Auswahl nicht verfügbar oder ungültig.",
				)
				p.Reveal = text("Location", "Ort")
				refresh()
				return
			}
			cfg.Shell.Remove(p)
		}},
		{
			Label:     text("Cancel", "Abbrechen"),
			MinHeight: 64,
			Role:      shell.Plain,
			Do:        func() { cfg.Shell.Remove(p) },
		},
	}
	p.Build = func() ([]menu.Row, []func(int)) {
		// Settings search may construct/build this page without opening it.
		if !loaded {
			return nil, nil
		}
		var rows []menu.Row
		var actions []func(int)
		add := func(r menu.Row, action func(int)) { rows = append(rows, r); actions = append(actions, action) }
		location := b.Current
		if location == "" {
			location = text("User file locations", "Benutzerdatei-Orte")
		}
		add(menu.Row{Label: text("Location", "Ort"), Hint: location, Dim: true}, nil)
		if b.Current != "" {
			add(
				menu.Row{
					Label: text("Parent folder", "Übergeordneter Ordner"),
					Glyph: "mdi:arrow-up",
					Kind:  menu.Chevron,
				},
				func(int) {
					b.Parent()
					note = ""
					p.Reveal = text("Location", "Ort")
					refresh()
				},
			)
		}
		if b.Err != nil {
			msg := text(
				"Folder or previous selection unavailable.",
				"Ordner oder bisherige Auswahl nicht verfügbar.",
			)
			if errors.Is(b.Err, filepicker.ErrLimit) {
				msg = text("Folder has too many entries.", "Ordner enthält zu viele Einträge.")
			}
			add(menu.Row{Label: msg, Dim: true}, nil)
		}
		if note != "" {
			add(menu.Row{Label: note, Dim: true}, nil)
		}
		if b.Current == "" {
			for _, loc := range b.Locations() {
				loc := loc
				add(
					menu.Row{
						Label: loc.Label,
						Hint:  loc.Path,
						Glyph: "mdi:folder",
						Kind:  menu.Chevron,
					},
					func(int) { navigate(loc.Path) },
				)
			}
			return rows, actions
		}
		candidate := b.Candidate()
		if candidate != "" {
			add(menu.Row{Label: text("Selection", "Auswahl"), Hint: candidate, Dim: true}, nil)
		}
		end := min(b.Offset+filepicker.PageSize, len(b.Entries))
		if len(b.Entries) == 0 {
			add(
				menu.Row{
					Label: text("No matching entries.", "Keine passenden Einträge."),
					Dim:   true,
				},
				nil,
			)
		}
		if b.Offset > 0 {
			add(
				menu.Row{Label: text("Previous entries", "Vorherige Einträge"), Kind: menu.Chevron},
				func(int) {
					b.Offset -= filepicker.PageSize
					p.Reveal = text("Location", "Ort")
					refresh()
				},
			)
		}
		for _, e := range b.Entries[b.Offset:end] {
			e := e
			row := menu.Row{Label: e.Name, Chosen: b.Selected == e.Path, Glyph: "mdi:file"}
			if e.Directory {
				row.Kind = menu.Chevron
				row.Glyph = "mdi:folder"
			}
			add(row, func(int) {
				if e.Directory {
					navigate(e.Path)
				} else {
					b.Select(e.Path)
					note = ""
					refresh()
				}
			})
		}
		if end < len(b.Entries) {
			add(
				menu.Row{Label: text("Next entries", "Weitere Einträge"), Kind: menu.Chevron},
				func(int) {
					b.Offset += filepicker.PageSize
					p.Reveal = text("Location", "Ort")
					refresh()
				},
			)
		}
		// Passive status follows the entries, never masquerading as a file or action.
		detail := strings.Join(cfg.Browser.Extensions, " / ")
		if detail == "" {
			detail = text("All permitted types", "Alle zulässigen Typen")
		}
		if cfg.Browser.Directory {
			detail = text("Folders", "Ordner")
		}
		if len(b.Entries) > filepicker.PageSize {
			detail += fmt.Sprintf(" · %d–%d / %d", b.Offset+1, end, len(b.Entries))
		}
		add(menu.Row{Label: text("File types", "Dateitypen"), Hint: detail, Dim: true}, nil)
		return rows, actions
	}
	return p
}

// Row deliberately keeps the long current path in a wrapping hint, not the
// fixed-width trailing value column. The caller owns validation/persistence.
func Row(label, current string, cfg Config) menu.Row {
	return menu.Row{Label: label, Hint: current, Kind: menu.Chevron,
		Save: func(v string) {
			if cfg.Choose != nil {
				_ = cfg.Choose(v)
			}
		},
		Open: func() menu.View {
			cfg.Browser.Initial = current
			return New(cfg)
		}}
}
