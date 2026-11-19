//go:build !board_biscuit

package control

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/url"

	"github.com/ygelfand/echolocal/internal/component"
	"github.com/ygelfand/echolocal/internal/feature/tones"
	connection "github.com/ygelfand/echolocal/internal/openclaw"
	esphome "github.com/ygelfand/go-esphome-device"
)

func registerNodeRegistry(a *connection.NodeAccess) {
	for _, action := range component.Default().Actions() {
		group := nodeActionGroup(action.Name)
		if group == "" {
			continue
		}
		_ = nodeRegister(a, connection.NodeOperation{Command: "action." + action.Name, Group: group, Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var p struct {
				Args map[string]any `json:"args"`
			}
			if decodeNode(raw, &p) != nil {
				return nil, errors.New("invalid_params")
			}
			for k, v := range p.Args {
				known := false
				for _, arg := range action.Args {
					if arg.Name != k {
						continue
					}
					known = true
					switch arg.Type {
					case esphome.ArgString:
						s, ok := v.(string)
						if !ok || len(s) > 48000 {
							return nil, errors.New("invalid_params")
						}
					case esphome.ArgBool:
						if _, ok := v.(bool); !ok {
							return nil, errors.New("invalid_params")
						}
					case esphome.ArgInt:
						n, ok := v.(float64)
						if !ok || n != math.Trunc(n) || n < -2147483648 || n > 2147483647 {
							return nil, errors.New("invalid_params")
						}
					case esphome.ArgFloat:
						n, ok := v.(float64)
						if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
							return nil, errors.New("invalid_params")
						}
					}
				}
				if !known {
					return nil, errors.New("invalid_params")
				}
			}
			// Node uploads are finite, complete operations: no partial transfer can
			// survive a permission revocation or a replacement authenticated socket.
			if action.Name == "tone_write" && (p.Args["page"] != float64(0) || p.Args["pages"] != float64(1)) {
				return nil, errors.New("invalid_params")
			}
			c, e := esphome.NewCall(p.Args)
			if e != nil {
				return nil, errors.New("invalid_params")
			}
			if ctx.Err() != nil || !a.Granted(group) {
				return nil, errors.New("cancelled")
			}
			before := nodeCallID()
			out, e := action.Run(c)
			if group == "intercom" {
				nodeTrackCall(before)
			}
			if e != nil {
				return nil, errors.New("operation_failed")
			}
			// Only vetted registry actions may return their structured answers.
			return out, nil
		}})
	}
	_ = nodeRegister(a, connection.NodeOperation{Command: "tones.list", Group: "tones", Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
		if e := nodeEmpty(raw); e != nil {
			return nil, e
		}
		return tones.Get().List(), nil
	}})
	for _, entity := range component.Default().Entities() {
		switch e := entity.(type) {
		case *esphome.MediaPlayer:
			if e.OnCommand == nil {
				continue
			}
			_ = nodeRegister(a, connection.NodeOperation{Command: "audio.control." + e.ObjectID, Group: "audio", Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var p struct {
					Command      string   `json:"command"`
					Volume       *float32 `json:"volume"`
					URL          string   `json:"url"`
					Announcement bool     `json:"announcement"`
				}
				if decodeNode(raw, &p) != nil {
					return nil, errors.New("invalid_params")
				}
				cmd := esphome.MediaCommand{}
				if p.Command != "" {
					commands := map[string]esphome.MediaPlayerCommand{"play": esphome.MediaPlayerPlay, "pause": esphome.MediaPlayerPause, "stop": esphome.MediaPlayerStop, "mute": esphome.MediaPlayerMute, "unmute": esphome.MediaPlayerUnmute, "volume_up": esphome.MediaPlayerVolumeUp, "volume_down": esphome.MediaPlayerVolumeDown, "toggle": esphome.MediaPlayerToggle}
					v, ok := commands[p.Command]
					if !ok {
						return nil, errors.New("invalid_params")
					}
					cmd.HasCommand = true
					cmd.Command = v
				}
				if p.Volume != nil {
					if *p.Volume < 0 || *p.Volume > 1 {
						return nil, errors.New("invalid_params")
					}
					cmd.HasVolume = true
					cmd.Volume = *p.Volume
				}
				if p.URL != "" {
					u, err := url.Parse(p.URL)
					if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || len(p.URL) > 2048 {
						return nil, errors.New("invalid_params")
					}
					cmd.HasMediaURL = true
					cmd.MediaURL = p.URL
					cmd.HasAnnouncement = true
					cmd.Announcement = p.Announcement
				}
				if ctx.Err() != nil || !a.Granted("audio") {
					return nil, errors.New("cancelled")
				}
				e.OnCommand(cmd)
				return map[string]bool{"done": true}, nil
			}})
		case *esphome.Button:
			if e.ObjectID != "test_playback" || e.OnPress == nil {
				continue
			}
			_ = nodeRegister(a, connection.NodeOperation{Command: "audio.test", Group: "audio", Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
				if err := nodeEmpty(raw); err != nil {
					return nil, err
				}
				e.OnPress()
				return map[string]bool{"done": true}, nil
			}})
		}
	}
}
func nodeActionGroup(name string) string {
	switch name {
	case "youtube_tv":
		return "audio"
	case "call", "call_answer", "call_hangup":
		return "intercom"
	case "alarm_list", "alarm_set", "alarm_enable", "alarm_delete", "alarm_stop", "alarm_snooze", "alarm":
		return "alarms"
	case "dashboard_show_page", "dashboard_show_alert", "dashboard_show_alert_choice", "dashboard_dismiss_alert", "dashboard_screen_message", "screen_show_page", "screen_show_alert", "screen_show_alert_choice", "screen_dismiss_alert", "screen_screen_message", "show_message", "clear_message":
		return "display"
	case "tone_write", "tone_rename", "tone_remove":
		return "tones"
	}
	// Future registry additions are not silently executable.

	return ""
}
