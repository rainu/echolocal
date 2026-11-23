package avatarlab

import (
	"errors"
	"github.com/ygelfand/echolocal/internal/avatar"
	"github.com/ygelfand/echolocal/internal/config"
	"github.com/ygelfand/libcountertop/pkg/say"
	setting "github.com/ygelfand/libcountertop/pkg/settings"
	"strconv"
	"strings"
)

func fullscreenSettings() []setting.Setting[config.Avatar] {
	rows := []setting.Setting[config.Avatar]{
		{Name: "talk_mode", Kind: setting.Choice, Options: []setting.Option{{Value: "split_screen", Key: "split_screen"}, {Value: "fullscreen", Key: "fullscreen"}}, Group: "fullscreen", Read: func(a *config.Avatar) string { return a.Mode() }, Write: func(a *config.Avatar, v string) error {
			if v != "split_screen" && v != "fullscreen" {
				return errors.New(avatar.Text(say.Chosen(), "talk_mode_help"))
			}
			a.TalkMode = v
			return nil
		}},
		{Name: "fullscreen_background", Kind: setting.Text, Group: "fullscreen", Read: func(a *config.Avatar) string { return a.FullscreenBackground }, Write: func(a *config.Avatar, v string) error {
			if e := avatar.ValidateBackground(v); e != nil {
				return errors.New(avatar.Text(say.Chosen(), e.Error()))
			}
			a.FullscreenBackground = v
			return nil
		}},
	}
	for _, f := range []struct {
		name     string
		min, max int
		unit     string
		ptr      func(*config.Avatar) *int
	}{
		{"zoom", 50, 999, "%", func(a *config.Avatar) *int { return &a.FullscreenZoom }},
		{"yaw", -180, 180, "°", func(a *config.Avatar) *int { return &a.FullscreenYaw }},
		{"pitch", -85, 85, "°", func(a *config.Avatar) *int { return &a.FullscreenPitch }},
		{"x", -500, 500, "%", func(a *config.Avatar) *int { return &a.FullscreenX }},
		{"y", -500, 500, "%", func(a *config.Avatar) *int { return &a.FullscreenY }},
	} {
		rows = append(rows, setting.Setting[config.Avatar]{Name: "fullscreen_" + f.name, Kind: setting.Number, Group: "fullscreen", Min: f.min, Max: f.max, Unit: f.unit,
			Read: func(a *config.Avatar) string {
				v := a.Composition(true)
				switch f.name {
				case "zoom":
					return strconv.Itoa(v.Zoom)
				case "yaw":
					return strconv.Itoa(v.Yaw)
				case "pitch":
					return strconv.Itoa(v.Pitch)
				case "x":
					return strconv.Itoa(v.X)
				default:
					return strconv.Itoa(v.Y)
				}
			},
			Write: func(a *config.Avatar, v string) error {
				n, e := strconv.Atoi(v)
				if e != nil || n < f.min || n > f.max {
					return errors.New(avatar.Text(say.Chosen(), "presentation_invalid"))
				}
				*f.ptr(a) = n
				return nil
			}})
	}
	return rows
}
func fullscreenMessage(k string) (string, bool) {
	if k == "avatar.option.talk_mode.split_screen" {
		return "split_screen", true
	}
	if k == "avatar.option.talk_mode.fullscreen" {
		return "fullscreen_mode", true
	}
	if k == "avatar.group.fullscreen" {
		return "fullscreen_mode", true
	}
	for _, prefix := range []string{"avatar.setting.", "avatar.entity.", "avatar.help."} {
		if strings.HasPrefix(k, prefix) {
			name := strings.TrimPrefix(k, prefix)
			if name == "talk_mode" {
				if prefix == "avatar.help." {
					return "talk_mode_help", true
				}
				return "talk_mode", true
			}
			if strings.HasPrefix(name, "fullscreen_") {
				name = strings.TrimPrefix(name, "fullscreen_")
				if prefix == "avatar.help." {
					return name + "_help", true
				}
				if name == "background" {
					return "fullscreen_background", true
				}
				return name, true
			}
		}
	}
	return "", false
}
