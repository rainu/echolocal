//go:build !board_biscuit

package control

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"

	"github.com/ygelfand/echolocal/internal/component"
	"github.com/ygelfand/echolocal/internal/feature/firmware"
	connection "github.com/ygelfand/echolocal/internal/openclaw"
	esphome "github.com/ygelfand/go-esphome-device"
)

func registerNodeMaintenance(a *connection.NodeAccess) {
	for _, e := range component.Default().Entities() {
		b, ok := e.(*esphome.Button)
		if !ok || b.OnPress == nil || b.ObjectID != "purge_cache" {
			continue
		}
		_ = a.Register(connection.NodeOperation{Command: "maintenance.purgeCache", Group: "maintenance", Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var p struct {
				Confirm bool `json:"confirm"`
			}
			if decodeNode(raw, &p) != nil || !p.Confirm {
				return nil, errors.New("confirmation_required")
			}
			b.OnPress()
			return map[string]bool{"scheduled": true}, nil
		}})
	}
	_ = a.Register(connection.NodeOperation{Command: "maintenance.updateCheck", Group: "maintenance", Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
		if e := nodeEmpty(raw); e != nil {
			return nil, e
		}
		firmware.Get().Check(ctx)
		return map[string]any{"checked": true}, nil
	}})
	_ = a.Register(connection.NodeOperation{Command: "maintenance.updateInstall", Group: "maintenance", Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p struct {
			Confirm bool   `json:"confirm"`
			Version string `json:"version"`
		}
		if decodeNode(raw, &p) != nil || !p.Confirm {
			return nil, errors.New("confirmation_required")
		}
		if !regexp.MustCompile(`^v?[0-9][a-zA-Z0-9._-]{0,79}$`).MatchString(p.Version) {
			return nil, errors.New("invalid_params")
		}
		if e := firmware.Get().InstallBuild(ctx, p.Version); e != nil {
			return nil, errors.New("update_failed")
		}
		return map[string]bool{"done": true}, nil
	}})
	_ = a.Register(connection.NodeOperation{Command: "device.sensors", Group: "state", Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
		if e := nodeEmpty(raw); e != nil {
			return nil, e
		}
		out := map[string]any{}
		for _, entity := range component.Default().Entities() {
			switch e := entity.(type) {
			case *esphome.Sensor:
				if nodeSensor(e.ObjectID) {
					out[e.ObjectID] = e.Get()
				}
			case *esphome.BinarySensor:
				if e.ObjectID == "mic_mute" || e.ObjectID == "headphones" {
					out[e.ObjectID] = e.Get()
				}
			}
		}
		return out, nil
	}})
}
func nodeSensor(id string) bool {
	switch id {
	case "cpu_temperature", "radio_temperature", "lux", "cpu_usage", "load_average", "memory_available", "free_space", "uptime", "cpu_cores", "cpu_cores_online":
		return true
	}
	return false
}
