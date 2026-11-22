//go:build !board_biscuit

package settings

import (
	"github.com/ygelfand/echolocal/internal/avatar"
	"github.com/ygelfand/echolocal/internal/config"
	"github.com/ygelfand/echolocal/internal/feature/avatarlab"
	"github.com/ygelfand/libcountertop/pkg/display/menu"
	sharedshell "github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/say"
	setting "github.com/ygelfand/libcountertop/pkg/settings"
)

func AvatarModelPage() *sharedshell.Page { return avatarModelPageFor(avatarlab.Controls()) }

func avatarModelPageFor(c *setting.Controls[config.Avatar]) *sharedshell.Page {
	note := ""
	return &sharedshell.Page{Title: avatar.Text(say.Chosen(), "model"), Build: func() ([]menu.Row, []func(int)) {
		text := func(k string) string { return avatar.Text(say.Chosen(), k) }
		save := func(v string) {
			note = ""
			if err := c.ChangeRaw("path", v); err != nil {
				note = text("invalid_path")
			}
		}
		rows := []menu.Row{
			{Label: text("path"), Value: c.Read().Path, Kind: menu.Field, Save: save},
			{Label: text("default"), Kind: menu.Plain},
			{Label: text("help"), Kind: menu.Plain},
		}
		acts := []func(int){nil, func(int) { save("") }, nil}
		if note != "" {
			rows = append(rows, menu.Row{Label: note, Kind: menu.Plain})
			acts = append(acts, nil)
		}
		return rows, acts
	}}
}

func AvatarPresentationPage() *sharedshell.Page {
	return avatarPresentationPageFor(avatarlab.Controls())
}

func avatarPresentationPageFor(c *setting.Controls[config.Avatar]) *sharedshell.Page {
	note := ""
	return &sharedshell.Page{Title: avatar.Text(say.Chosen(), "presentation"), Build: func() ([]menu.Row, []func(int)) {
		rows := []menu.Row{}
		acts := []func(int){}
		for _, s := range c.Table.In("presentation") {
			s := s
			if s.Kind == setting.Toggle {
				a := c.Read()
				on, _ := setting.Boolean(s.Read(&a))
				rows = append(rows, menu.Row{Label: s.Title(), Hint: s.OptionalText("help"), Kind: menu.Toggle, On: on})
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
			rows = append(rows, menu.Row{Label: s.Title(), Hint: s.OptionalText("help"), Value: func() string { a := c.Read(); return s.Read(&a) }(), Kind: menu.Field, Save: func(v string) {
				note = ""
				if c.ChangeRaw(s.Name, v) != nil {
					note = avatar.Text(say.Chosen(), "presentation_invalid")
				}
			}})
		}
		rows = append(rows, menu.Row{Label: avatar.Text(say.Chosen(), "reset_presentation"), Kind: menu.Plain})
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
	}}
}

func AvatarBackgroundPage() *sharedshell.Page {
	c := avatarlab.Controls()
	note := ""
	return &sharedshell.Page{Title: avatar.Text(say.Chosen(), "background"), Build: func() ([]menu.Row, []func(int)) {
		text := func(k string) string { return avatar.Text(say.Chosen(), k) }
		save := func(v string) {
			note = ""
			if e := c.ChangeRaw("background", v); e != nil {
				note = e.Error()
			}
		}
		rows := []menu.Row{
			{Label: text("background"), Value: c.Read().Background, Kind: menu.Field, Save: save},
			{Label: text("background_reset"), Kind: menu.Plain},
			{Label: text("background_help"), Kind: menu.Plain},
		}
		acts := []func(int){nil, func(int) { save("") }, nil}
		if note != "" {
			rows = append(rows, menu.Row{Label: note, Kind: menu.Plain})
			acts = append(acts, nil)
		}
		return rows, acts
	}}
}
