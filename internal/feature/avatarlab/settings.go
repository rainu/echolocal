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
	"math"
	"strconv"
	"sync"
)

func NewTable() *setting.Table[config.Avatar] {
	rows := []setting.Setting[config.Avatar]{{Name: "path", Kind: setting.Text, Group: "model", Icon: "mdi:account", Read: func(c *config.Avatar) string { return c.Path }, Write: func(c *config.Avatar, v string) error {
		if err := avatar.ValidatePath(v); err != nil {
			return errors.New(avatar.Text(say.Chosen(), "invalid_path"))
		}
		c.Path = v
		return nil
	}}}
	rows = append(rows, setting.Setting[config.Avatar]{Name: "background", Kind: setting.Text, Group: "background", Icon: "mdi:image",
		Read: func(a *config.Avatar) string { return a.Background },
		Write: func(a *config.Avatar, v string) error {
			if e := avatar.ValidateBackground(v); e != nil {
				return errors.New(avatar.Text(say.Chosen(), e.Error()))
			}
			a.Background = v
			return nil
		}})
	for _, f := range []struct {
		name     string
		min, max int
		unit     string
		ptr      func(*config.Avatar) *int
	}{
		{"zoom", config.AvatarZoomMin, config.AvatarZoomMax, "%", func(a *config.Avatar) *int { return &a.Zoom }},
		{"yaw", -180, 180, "°", func(a *config.Avatar) *int { return &a.Yaw }},
		{"pitch", -85, 85, "°", func(a *config.Avatar) *int { return &a.Pitch }},
		{"x", -config.AvatarPositionLimit, config.AvatarPositionLimit, "%", func(a *config.Avatar) *int { return &a.X }},
		{"y", -config.AvatarPositionLimit, config.AvatarPositionLimit, "%", func(a *config.Avatar) *int { return &a.Y }},
	} {
		rows = append(rows, setting.Setting[config.Avatar]{Name: f.name, Kind: setting.Number, Group: "presentation", Icon: "mdi:account", Min: f.min, Max: f.max, Unit: f.unit, Read: func(a *config.Avatar) string { v := a.Presentation(); return strconv.Itoa(*f.ptr(&v)) }, Write: func(a *config.Avatar, v string) error {
			n, e := strconv.Atoi(v)
			if e != nil || n < f.min || n > f.max {
				return errors.New(avatar.Text(say.Chosen(), "presentation_invalid"))
			}
			*f.ptr(a) = n
			return nil
		}})
	}

	for _, f := range []struct {
		name     string
		disabled func(*config.Avatar) *bool
	}{
		{"idle", func(a *config.Avatar) *bool { return &a.IdleDisabled }},
		{"blink", func(a *config.Avatar) *bool { return &a.BlinkDisabled }},
		{"animation", func(a *config.Avatar) *bool { return &a.AnimationDisabled }},
	} {
		rows = append(rows, setting.Setting[config.Avatar]{Name: f.name, Kind: setting.Toggle, Group: "presentation", Icon: "mdi:account",
			Read: func(a *config.Avatar) string { return setting.OnOff(!*f.disabled(a)) },
			Write: func(a *config.Avatar, v string) error {
				on, ok := setting.Boolean(v)
				if !ok {
					return errors.New(avatar.Text(say.Chosen(), "toggle_invalid"))
				}
				*f.disabled(a) = !on
				return nil
			},
		})
	}
	return setting.NewTable("avatar", []setting.Group{"model", "background", "presentation"}, rows, setting.Messages{
		Text: func(k string) string {
			if key, ok := presentationMessage(k); ok {
				return avatar.Text(say.Chosen(), key)
			}
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
			if _, ok := presentationMessage(k); ok {
				return false
			}
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

func presentationMessage(k string) (string, bool) {
	if k == "avatar.group.background" || k == "avatar.setting.background" || k == "avatar.entity.background" {
		return "background", true
	}
	if k == "avatar.reset.background" {
		return "background_reset", true
	}
	if k == "avatar.example.background" {
		return "background_example", true
	}
	if k == "avatar.group.presentation" {
		return "presentation", true
	}
	for _, name := range []string{"zoom", "yaw", "pitch", "x", "y", "idle", "blink", "animation"} {
		if k == "avatar.setting."+name || k == "avatar.entity."+name {
			return name, true
		}
		if k == "avatar.help."+name {
			return name + "_help", true
		}
	}
	return "", false
}

// Options always reads the same presentation used by the owner settings API.
func Options(a config.Avatar) avatar.Options {
	a = a.Presentation()
	return avatar.Options{Idle: !a.IdleDisabled, Blink: !a.BlinkDisabled, Animation: !a.AnimationDisabled, Zoom: float64(a.Zoom) / 100, Yaw: float64(a.Yaw), Pitch: float64(a.Pitch), PanX: float64(a.X) / 100, PanY: float64(a.Y) / 100}
}
func SavePresentation(o avatar.Options) error {
	return config.AvatarSection.Update(func(a *config.Avatar) { applyPresentation(a, o) })
}

func applyPresentation(a *config.Avatar, o avatar.Options) {
	a.IdleDisabled = !o.Idle
	a.BlinkDisabled = !o.Blink
	a.AnimationDisabled = !o.Animation
	a.Zoom = int(math.Round(avatar.EffectiveZoom(o.Zoom) * 100))
	a.Yaw = int(math.Round(o.Yaw))
	a.Pitch = int(math.Round(o.Pitch))
	a.X = int(math.Round(o.PanX * 100))
	a.Y = int(math.Round(o.PanY * 100))
	*a = a.Presentation()
}
