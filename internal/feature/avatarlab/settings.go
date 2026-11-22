package avatarlab

import (
	"errors"
	"github.com/ygelfand/echolocal/internal/avatar"
	"github.com/ygelfand/echolocal/internal/board"
	"github.com/ygelfand/echolocal/internal/component"
	"github.com/ygelfand/echolocal/internal/config"
	sharedcomponent "github.com/ygelfand/libcountertop/pkg/runtime/component"
	"github.com/ygelfand/libcountertop/pkg/say"
	setting "github.com/ygelfand/libcountertop/pkg/settings"
	"sync"
)

func NewTable() *setting.Table[config.Avatar] {
	return setting.NewTable("avatar", []setting.Group{"model"}, []setting.Setting[config.Avatar]{{Name: "path", Kind: setting.Text, Group: "model", Icon: "mdi:account", Read: func(c *config.Avatar) string { return c.Path }, Write: func(c *config.Avatar, v string) error {
		if err := avatar.ValidatePath(v); err != nil {
			return errors.New(avatar.Text(say.Chosen(), "invalid_path"))
		}
		c.Path = v
		return nil
	}}}, setting.Messages{
		Text: func(k string) string {
			switch k {
			case "avatar.group.model":
				return avatar.Text(say.Chosen(), "title")
			case "avatar.setting.path", "avatar.entity.path":
				return avatar.Text(say.Chosen(), "path")
			case "avatar.help.path":
				return avatar.Text(say.Chosen(), "help")
			case "avatar.reset.path":
				return avatar.Text(say.Chosen(), "default")
			case "avatar.example.path":
				return "/data/local/tmp/avatar.vrm"
			}
			return k
		},
		Missing: func(k string) bool {
			return k != "avatar.help.path" && k != "avatar.reset.path" && k != "avatar.group.model" && k != "avatar.setting.path" && k != "avatar.entity.path" && k != "avatar.example.path"
		},
	})
}

var controls = sync.OnceValue(func() *setting.Controls[config.Avatar] {
	c := &setting.Controls[config.Avatar]{Table: NewTable(), Read: config.AvatarSection.Read, Save: func(s setting.Setting[config.Avatar], v string) error {
		return setting.Store(config.AvatarSection, s, v)
	}}
	component.Settings.Add(c)
	return c
})

func Controls() *setting.Controls[config.Avatar] { return controls() }

type service struct{}

func (*service) Name() string { return "avatar-settings" }
func init() {
	component.Register(sharedcomponent.Device, func() *service {
		if component.Board().Codename == "athena" {
			Controls()
		}
		return &service{}
	}, component.Needs(board.Panel))
}
