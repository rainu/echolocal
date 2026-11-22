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
