//go:build !board_biscuit

package control

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/ygelfand/echolocal/internal/config"
	"github.com/ygelfand/echolocal/internal/feature/alarm"
	"github.com/ygelfand/echolocal/internal/feature/clawtalk"
	"github.com/ygelfand/echolocal/internal/feature/idle"
	"github.com/ygelfand/echolocal/internal/feature/localvideo"
	"github.com/ygelfand/echolocal/internal/feature/screen"
	timers "github.com/ygelfand/echolocal/internal/feature/timer"
	"github.com/ygelfand/echolocal/internal/feature/voice"
	"github.com/ygelfand/echolocal/internal/feature/volume"
	"github.com/ygelfand/echolocal/internal/hardware/display"
	"github.com/ygelfand/echolocal/internal/hardware/speaker"
	connection "github.com/ygelfand/echolocal/internal/openclaw"
	esphome "github.com/ygelfand/go-esphome-device"
	"github.com/ygelfand/go-esphome-device/api"
	libalarm "github.com/ygelfand/libcountertop/pkg/assistant/alarm"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

func nodePercent(v *float64) bool {
	return v != nil && !math.IsNaN(*v) && !math.IsInf(*v, 0) && *v >= 0 && *v <= 100
}
func nodeStream(s string) (config.Stream, bool) {
	switch s {
	case "", "main":
		return config.StreamMain, true
	case "media":
		return config.StreamMedia, true
	case "voice":
		return config.StreamVoice, true
	case "alerts":
		return config.StreamAlerts, true
	case "feedback":
		return config.StreamFeedback, true
	}
	return "", false
}
func registerNodeConvenience(a *connection.NodeAccess) {
	add := func(name, group string, run func(context.Context, json.RawMessage) (any, error)) {
		_ = nodeRegister(a, connection.NodeOperation{Command: name, Group: group, Run: run})
	}
	add("audio.status", "audio", func(ctx context.Context, raw json.RawMessage) (any, error) {
		if nodeEmpty(raw) != nil {
			return nil, errors.New("invalid_params")
		}
		out := map[string]any{}
		for _, s := range config.Streams() {
			out[string(s)] = map[string]any{"volume": volume.Get().Level(s), "muted": speaker.Get().Muted(s)}
		}
		return map[string]any{"channels": out, "video": localvideo.Get().Snapshot()}, nil
	})
	add("audio.volume.set", "audio", func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p struct {
			Channel string   `json:"channel"`
			Volume  *float64 `json:"volume"`
		}
		if decodeNode(raw, &p) != nil || !nodePercent(p.Volume) {
			return nil, errors.New("invalid_params")
		}
		s, ok := nodeStream(p.Channel)
		if !ok {
			return nil, errors.New("invalid_params")
		}
		volume.Get().Set(s, int(math.Round(*p.Volume)))
		return map[string]any{"channel": s, "volume": volume.Get().Level(s)}, nil
	})
	add("audio.mute.set", "audio", func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p struct {
			Channel string `json:"channel"`
			Muted   *bool  `json:"muted"`
		}
		if decodeNode(raw, &p) != nil || p.Muted == nil {
			return nil, errors.New("invalid_params")
		}
		s, ok := nodeStream(p.Channel)
		if !ok {
			return nil, errors.New("invalid_params")
		}
		speaker.Get().Mute(s, *p.Muted)
		return map[string]any{"channel": s, "muted": speaker.Get().Muted(s)}, nil
	})
	add("display.status", "display", func(ctx context.Context, raw json.RawMessage) (any, error) {
		if nodeEmpty(raw) != nil {
			return nil, errors.New("invalid_params")
		}
		cfg := config.Get().Screen
		w, h := display.Get().Size()
		return map[string]any{"brightness": cfg.Backlight, "automatic": cfg.Mode == schema.ModeAuto, "width": w, "height": h}, nil
	})
	add("display.brightness.set", "display", func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p struct {
			Brightness *float64 `json:"brightness"`
		}
		if decodeNode(raw, &p) != nil || !nodePercent(p.Brightness) {
			return nil, errors.New("invalid_params")
		}
		// An explicit brightness command selects manual mode. It cannot silently
		// become a mere bias for automatic brightness.
		if screen.Get().Set("auto", "off") != nil || screen.Get().Set("backlight", strconv.Itoa(int(math.Round(*p.Brightness)))) != nil {
			return nil, errors.New("operation_failed")
		}
		return map[string]any{"brightness": config.Get().Screen.Backlight, "automatic": false}, nil
	})
	var powerMu sync.Mutex
	previous := 70
	previousAuto := false
	poweredOff := false
	add("display.power.set", "display", func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p struct{ On *bool }
		if decodeNode(raw, &p) != nil || p.On == nil {
			return nil, errors.New("invalid_params")
		}
		powerMu.Lock()
		defer powerMu.Unlock()
		if *p.On {
			idle.Get().Wake()
			if poweredOff || config.Get().Screen.Backlight == 0 {
				if screen.Get().Set("backlight", strconv.Itoa(previous)) != nil {
					return nil, errors.New("operation_failed")
				}
				if previousAuto && screen.Get().Set("auto", "on") != nil {
					return nil, errors.New("operation_failed")
				}
				poweredOff = false
			}
		} else if !poweredOff {
			cfg := config.Get().Screen
			previous, previousAuto = cfg.Backlight, cfg.Mode == schema.ModeAuto
			if screen.Get().Set("auto", "off") != nil || screen.Get().Set("backlight", "0") != nil {
				return nil, errors.New("operation_failed")
			}
			poweredOff = true
		}
		return map[string]bool{"on": *p.On}, nil
	})
	add("alarm.list", "alarms", func(ctx context.Context, raw json.RawMessage) (any, error) {
		if nodeEmpty(raw) != nil {
			return nil, errors.New("invalid_params")
		}
		return map[string]any{"alarms": alarm.Get().List()}, nil
	})
	for _, mode := range []string{"create", "update"} {
		add("alarm."+mode, "alarms", func(ctx context.Context, raw json.RawMessage) (any, error) {
			var p nodeAlarmParams
			if decodeNode(raw, &p) != nil {
				return nil, errors.New("invalid_params")
			}
			var base libalarm.Alarm
			if mode == "update" {
				found := false
				for _, v := range alarm.Get().List() {
					if v.ID == p.ID {
						base = v
						found = true
						break
					}
				}
				if !found {
					return nil, errors.New("not_found")
				}
			} else if p.ID != "" {
				return nil, errors.New("invalid_params")
			} else {
				base.On = true
			}
			v, err := p.apply(base, mode == "create")
			if err != nil {
				return nil, err
			}
			saved, err := alarm.Get().Set(v)
			if err != nil {
				return nil, errors.New("invalid_params")
			}
			return map[string]any{"alarm": saved}, nil
		})
	}
	add("alarm.delete", "alarms", func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p struct {
			ID string `json:"id"`
		}
		if decodeNode(raw, &p) != nil || p.ID == "" || len(p.ID) > 128 {
			return nil, errors.New("invalid_params")
		}
		if alarm.Get().Delete(p.ID) != nil {
			return nil, errors.New("not_found")
		}
		return map[string]bool{"done": true}, nil
	})
	registerNodeTimers(a)
	registerNodeTalk(a)
}

type nodeAlarmParams struct {
	ID            string  `json:"id"`
	Name          *string `json:"name"`
	Time          *string `json:"time"`
	Days          *string `json:"days"`
	Enabled       *bool   `json:"enabled"`
	Sound         *string `json:"sound"`
	SnoozeMinutes *int    `json:"snoozeMinutes"`
}

func (p nodeAlarmParams) apply(v libalarm.Alarm, create bool) (libalarm.Alarm, error) {
	bad := errors.New("invalid_params")
	if create {
		v.Volume = libalarm.VolumeStatic
		v.On = true
	}
	if len(p.ID) > 128 || (create && p.Time == nil) {
		return v, bad
	}
	if p.Name != nil {
		if len(*p.Name) > 256 {
			return v, bad
		}
		v.Name = *p.Name
	}
	if p.Time != nil {
		h, m, err := libalarm.ParseClock(*p.Time)
		if err != nil {
			return v, bad
		}
		v.Hour, v.Minute = h, m
	}
	if p.Days != nil {
		d, err := libalarm.ParseWeekdays(*p.Days)
		if err != nil {
			return v, bad
		}
		v.Days = d
	}
	if p.Enabled != nil {
		v.On = *p.Enabled
	}
	if p.Sound != nil {
		if len(*p.Sound) > 128 {
			return v, bad
		}
		v.Sound = schema.Chime(*p.Sound)
	}
	if p.SnoozeMinutes != nil {
		if *p.SnoozeMinutes < 0 || *p.SnoozeMinutes > 60 {
			return v, bad
		}
		v.Snooze = time.Duration(*p.SnoozeMinutes) * time.Minute
	}
	if v.Check() != nil {
		return v, bad
	}
	return v, nil
}

type nodeTimer struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Seconds  int    `json:"seconds"`
	EndsAtMS int64  `json:"endsAtMs"`
}

func registerNodeTimers(a *connection.NodeAccess) {
	var mu sync.Mutex
	held := map[string]nodeTimer{}
	add := func(name string, run func(context.Context, json.RawMessage) (any, error)) {
		_ = nodeRegister(a, connection.NodeOperation{Command: name, Group: "alarms", Run: run})
	}
	prune := func() {
		for id, t := range held {
			if t.EndsAtMS <= time.Now().UnixMilli() {
				delete(held, id)
			}
		}
	}
	add("timer.list", func(ctx context.Context, raw json.RawMessage) (any, error) {
		if nodeEmpty(raw) != nil {
			return nil, errors.New("invalid_params")
		}
		mu.Lock()
		defer mu.Unlock()
		prune()
		out := []nodeTimer{}
		for _, v := range held {
			out = append(out, v)
		}
		sort.Slice(out, func(i, j int) bool { return out[i].EndsAtMS < out[j].EndsAtMS })
		return map[string]any{"timers": out, "scope": "node-created"}, nil
	})
	add("timer.create", func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p struct {
			Seconds int    `json:"seconds"`
			Name    string `json:"name"`
		}
		if decodeNode(raw, &p) != nil || p.Seconds < 1 || p.Seconds > 86400 || len(p.Name) > 256 {
			return nil, errors.New("invalid_params")
		}
		mu.Lock()
		defer mu.Unlock()
		prune()
		if len(held) >= 64 {
			return nil, errors.New("busy")
		}
		var id [12]byte
		if _, err := rand.Read(id[:]); err != nil {
			return nil, errors.New("operation_failed")
		}
		t := nodeTimer{ID: "echolocal-node-" + hex.EncodeToString(id[:]), Name: p.Name, Seconds: p.Seconds, EndsAtMS: time.Now().Add(time.Duration(p.Seconds) * time.Second).UnixMilli()}
		timers.Get().Event(esphome.TimerEvent{Type: api.VoiceAssistantTimerEvent_VOICE_ASSISTANT_TIMER_STARTED, TimerID: t.ID, Name: t.Name, TotalSeconds: uint32(t.Seconds), SecondsLeft: uint32(t.Seconds), IsActive: true})
		held[t.ID] = t
		return t, nil
	})
	add("timer.cancel", func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p struct {
			ID string `json:"id"`
		}
		if decodeNode(raw, &p) != nil || p.ID == "" || len(p.ID) > 128 {
			return nil, errors.New("invalid_params")
		}
		mu.Lock()
		defer mu.Unlock()
		prune()
		if _, ok := held[p.ID]; !ok {
			return nil, errors.New("not_found")
		}
		timers.Get().Event(esphome.TimerEvent{Type: api.VoiceAssistantTimerEvent_VOICE_ASSISTANT_TIMER_CANCELLED, TimerID: p.ID})
		delete(held, p.ID)
		return map[string]bool{"done": true}, nil
	})
}
func registerNodeTalk(a *connection.NodeAccess) {
	var mu sync.Mutex
	var owned uint64
	a.OnCleanup(func() {
		mu.Lock()
		defer mu.Unlock()
		clawtalk.Get().CancelTransfer()
		if owned != 0 {
			clawtalk.Get().StopOwned(owned)
			owned = 0
		}
	})
	add := func(name string, run func(context.Context, json.RawMessage) (any, error)) {
		_ = nodeRegister(a, connection.NodeOperation{Command: name, Group: "talk", Run: run})
	}
	add("talk.status", func(ctx context.Context, raw json.RawMessage) (any, error) {
		if nodeEmpty(raw) != nil {
			return nil, errors.New("invalid_params")
		}
		s := clawtalk.Get().Status()
		return map[string]any{"active": s.Active, "state": s.State, "transfer": s.Transfer, "transferReason": s.TransferReason}, nil
	})
	add("talk.start", func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p struct {
			Slot int `json:"slot"`
		}
		if decodeNode(raw, &p) != nil || p.Slot < 1 || p.Slot > 2 {
			return nil, errors.New("invalid_params")
		}
		mu.Lock()
		defer mu.Unlock()
		// Cleanup owns this mutex too: if revocation races this check, it stops
		// the just-created generation before returning.
		if ctx.Err() != nil {
			return nil, errors.New("cancelled")
		}
		v := voice.Get()
		gen, err := clawtalk.Get().StartOwned(p.Slot-1, v.Turn().Busy() || v.Turn().Pending())
		if err != nil {
			return nil, errors.New("talk_unavailable")
		}
		owned = gen
		return map[string]bool{"started": true}, nil
	})
	add("talk.transfer", func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p struct {
			SessionID string `json:"sessionId"`
		}
		if decodeNode(raw, &p) != nil {
			return nil, errors.New("invalid_params")
		}
		return clawtalk.Get().PrepareTransfer(ctx, p.SessionID)
	})
	add("talk.stop", func(ctx context.Context, raw json.RawMessage) (any, error) {
		if nodeEmpty(raw) != nil {
			return nil, errors.New("invalid_params")
		}
		mu.Lock()
		defer mu.Unlock()
		stopped := false
		if owned != 0 {
			stopped = clawtalk.Get().StopOwned(owned)
			owned = 0
		}
		return map[string]bool{"stopped": stopped}, nil
	})

}
