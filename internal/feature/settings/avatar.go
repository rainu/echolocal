//go:build !board_biscuit

package settings

import (
	"github.com/ygelfand/libcountertop/pkg/display/menu"
	sharedshell "github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/say"
	setting "github.com/ygelfand/libcountertop/pkg/settings"

	"github.com/ygelfand/echolocal/internal/avatar"
	"github.com/ygelfand/echolocal/internal/config"
	"github.com/ygelfand/echolocal/internal/feature/avatarlab"
	featureshell "github.com/ygelfand/echolocal/internal/feature/shell"
	"github.com/ygelfand/echolocal/internal/filepicker"
	"github.com/ygelfand/echolocal/internal/layout"
	pickerui "github.com/ygelfand/echolocal/internal/ui/filepicker"
)

func AvatarModelPage() *sharedshell.Page { return avatarModelPageFor(avatarlab.Controls()) }

func avatarModelPageFor(c *setting.Controls[config.Avatar]) *sharedshell.Page {
	note := ""
	return &sharedshell.Page{
		Title: avatar.Text(say.Chosen(), "model"),
		Build: func() ([]menu.Row, []func(int)) {
			text := func(k string) string { return avatar.Text(say.Chosen(), k) }
			save := func(v string) error {
				note = ""
				err := c.ChangeRaw("path", v)
				if err != nil {
					note = text("invalid_path")
				}
				return err
			}
			rows := []menu.Row{
				avatarFileRow(
					pickerWords("Choose VRM file", "VRM-Datei auswählen"),
					c.Read().Path,
					[]string{".vrm"},
					save,
				),
				{Label: text("default"), Kind: menu.Plain},
				{
					Label: pickerWords(
						"Upload your VRM using Files or WebDAV, then choose it here.",
						"VRM über Dateien oder WebDAV hochladen und hier auswählen.",
					),
					Kind: menu.Plain,
				},
			}
			acts := []func(int){nil, func(int) { _ = save("") }, nil}
			if note != "" {
				rows = append(rows, menu.Row{Label: note, Kind: menu.Plain})
				acts = append(acts, nil)
			}
			return rows, acts
		},
	}
}

func AvatarPresentationPage() *sharedshell.Page {
	return avatarPresentationPageFor(avatarlab.Controls())
}

func avatarPresentationPageFor(c *setting.Controls[config.Avatar]) *sharedshell.Page {
	note := ""
	return &sharedshell.Page{
		Title: avatar.Text(say.Chosen(), "presentation"),
		Build: func() ([]menu.Row, []func(int)) {
			rows := []menu.Row{}
			acts := []func(int){}
			for _, s := range c.Table.In("presentation") {
				s := s
				if s.Kind == setting.Toggle {
					a := c.Read()
					on, _ := setting.Boolean(s.Read(&a))
					rows = append(
						rows,
						menu.Row{
							Label: s.Title(),
							Hint:  s.OptionalText("help"),
							Kind:  menu.Toggle,
							On:    on,
						},
					)
					acts = append(acts, func(int) {
						// Read at activation time; the page may have been changed by API/Node.
						a := c.Read()
						current, _ := setting.Boolean(s.Read(&a))
						note = ""
						if c.ChangeRaw(s.Name, setting.OnOff(!current)) != nil {
							note = avatar.Text(say.Chosen(), "save_failed")
						}
					})
					continue
				}
				acts = append(acts, nil)
				rows = append(
					rows,
					menu.Row{
						Label: s.Title(),
						Hint:  s.OptionalText("help"),
						Value: func() string { a := c.Read(); return s.Read(&a) }(),
						Kind:  menu.Field,
						Save: func(v string) {
							note = ""
							if c.ChangeRaw(s.Name, v) != nil {
								note = avatar.Text(say.Chosen(), "presentation_invalid")
							}
						},
					},
				)
			}
			rows = append(
				rows,
				menu.Row{Label: avatar.Text(say.Chosen(), "reset_presentation"), Kind: menu.Plain},
			)
			acts = append(acts, nil)
			acts[len(acts)-1] = func(int) {
				note = ""
				defaults := config.Avatar{}
				for _, row := range c.Table.In("presentation") {
					if c.ChangeRaw(row.Name, row.Read(&defaults)) != nil {
						note = avatar.Text(say.Chosen(), "save_failed")
						break
					}
				}
			}
			if note != "" {
				rows = append(rows, menu.Row{Label: note, Kind: menu.Plain})
				acts = append(acts, nil)
			}
			return rows, acts
		},
	}
}

func AvatarForegroundPage(full bool) *sharedshell.Page {
	return avatarForegroundPageFor(avatarlab.Controls(), full)
}
func avatarForegroundPageFor(c *setting.Controls[config.Avatar], full bool) *sharedshell.Page {
	name := "foreground"
	if full {
		name = "fullscreen_foreground"
	}
	note := ""
	return &sharedshell.Page{Title: avatar.Text(say.Chosen(), name), Build: func() ([]menu.Row, []func(int)) {
		text := func(k string) string { return avatar.Text(say.Chosen(), k) }
		row := c.Table.MustRow(name)
		a := c.Read()
		save := func(v string) error {
			note = ""
			err := c.ChangeRaw(name, v)
			if err != nil {
				note = err.Error()
			}
			return err
		}
		rows := []menu.Row{avatarFileRow(text(name), row.Read(&a), []string{".png", ".jpg", ".jpeg"}, save),
			{Label: text("foreground_reset"), Kind: menu.Plain},
			{Label: text("foreground_help"), Kind: menu.Plain}}
		acts := []func(int){nil, func(int) { _ = save("") }, nil}
		if note != "" {
			rows = append(rows, menu.Row{Label: note, Kind: menu.Plain})
			acts = append(acts, nil)
		}
		return rows, acts
	}}
}
func AvatarBackgroundPage() *sharedshell.Page { return avatarBackgroundPageFor(avatarlab.Controls()) }
func avatarBackgroundPageFor(c *setting.Controls[config.Avatar]) *sharedshell.Page {
	note := ""
	return &sharedshell.Page{
		Title: avatar.Text(say.Chosen(), "background"),
		Build: func() ([]menu.Row, []func(int)) {
			text := func(k string) string { return avatar.Text(say.Chosen(), k) }
			save := func(v string) error {
				note = ""
				err := c.ChangeRaw("background", v)
				if err != nil {
					note = err.Error()
				}
				return err
			}
			rows := []menu.Row{
				avatarFileRow(
					text("background"),
					c.Read().Background,
					[]string{".png", ".jpg", ".jpeg"},
					save,
				),
				{Label: text("background_reset"), Kind: menu.Plain},
			}
			acts := []func(int){nil, func(int) { _ = save("") }}
			if note != "" {
				rows = append(rows, menu.Row{Label: note, Kind: menu.Plain})
				acts = append(acts, nil)
			}
			return rows, acts
		},
	}
}

// Fullscreen uses the same validated registry as the owner web UI and API.
func AvatarFullscreenPage() *sharedshell.Page { return avatarFullscreenPageFor(avatarlab.Controls()) }
func avatarFullscreenPageFor(c *setting.Controls[config.Avatar]) *sharedshell.Page {
	note := ""
	return &sharedshell.Page{
		Title: avatar.Text(say.Chosen(), "fullscreen_mode"),
		Build: func() ([]menu.Row, []func(int)) {
			rows := []menu.Row{}
			acts := []func(int){}
			save := func(name, value string) error {
				note = ""
				err := c.ChangeRaw(name, value)
				if err != nil {
					note = avatar.Text(say.Chosen(), "save_failed")
				}
				return err
			}
			for _, row := range c.Table.In("fullscreen") {
				row := row
				a := c.Read()
				if row.Name == "talk_mode" {
					for _, mode := range []string{"split_screen", "fullscreen"} {
						mode := mode
						key := mode
						if mode == "fullscreen" {
							key = "fullscreen_mode"
						}
						rows = append(
							rows,
							menu.Row{
								Label: avatar.Text(
									say.Chosen(),
									"talk_mode",
								) + ": " + avatar.Text(
									say.Chosen(),
									key,
								),
								Hint: row.OptionalText("help"),
								Kind: menu.Toggle,
								On:   a.Mode() == mode,
							},
						)
						acts = append(acts, func(int) { _ = save("talk_mode", mode) })
					}
					continue
				}
				if row.Name == "fullscreen_background" || row.Name == "fullscreen_foreground" {
					rows = append(
						rows,
						avatarFileRow(
							row.Title(),
							row.Read(&a),
							[]string{".png", ".jpg", ".jpeg"},
							func(v string) error { return save(row.Name, v) },
						),
					)
					acts = append(acts, nil)
					rows = append(
						rows,
						menu.Row{
							Label: func() string {
								if row.Name == "fullscreen_foreground" {
									return avatar.Text(say.Chosen(), "foreground_reset")
								}
								return avatar.Text(say.Chosen(), "background_reset")
							}(),
							Kind: menu.Plain,
						},
					)
					acts = append(acts, func(int) { _ = save(row.Name, "") })
					continue
				}
				rows = append(
					rows,
					menu.Row{
						Label: row.Title(),
						Hint:  row.OptionalText("help"),
						Value: row.Read(&a),
						Kind:  menu.Field,
						Save:  func(v string) { _ = save(row.Name, v) },
					},
				)
				acts = append(acts, nil)
			}
			rows = append(
				rows,
				menu.Row{Label: avatar.Text(say.Chosen(), "reset_fullscreen"), Kind: menu.Plain},
			)
			acts = append(acts, func(int) {
				note = ""
				defaults := config.Avatar{}
				for _, row := range c.Table.In("fullscreen") {
					if row.Name != "talk_mode" {
						if c.ChangeRaw(row.Name, row.Read(&defaults)) != nil {
							note = avatar.Text(say.Chosen(), "save_failed")
							break
						}
					}
				}
			})
			if note != "" {
				rows = append(rows, menu.Row{Label: note, Kind: menu.Plain})
				acts = append(acts, nil)
			}
			return rows, acts
		},
	}
}

// Only user asset roots are exposed; the parent application state holds secrets.
func avatarFileRow(label, current string, extensions []string, save func(string) error) menu.Row {
	return pickerui.Row(
		label,
		current,
		pickerui.Config{
			Title:  label,
			Shell:  featureshell.Get(),
			Choose: save,
			Browser: filepicker.Options{
				Extensions: extensions,
				Locations: []filepicker.Location{
					{
						Label: pickerWords("User files", "Benutzerdateien"),
						Path:  layout.StateDir + "/user-files",
					},
					{Label: pickerWords("Local files", "Lokale Dateien"), Path: "/data/local"},
					{
						Label: pickerWords("Shared storage", "Gemeinsamer Speicher"),
						Path:  "/data/media/0",
					},
				},
			},
		},
	)
}
func pickerWords(en, de string) string {
	if say.Chosen() == "de" {
		return de
	}
	return en
}
