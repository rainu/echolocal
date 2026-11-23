//go:build !board_biscuit

package control

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/ygelfand/echolocal/internal/component"
	connection "github.com/ygelfand/echolocal/internal/openclaw"
	esphome "github.com/ygelfand/go-esphome-device"
	"strings"
)

func registerNodeEntities(a *connection.NodeAccess) {
	for _, entity := range component.Default().Entities() {
		switch e := entity.(type) {
		case *esphome.Button:
			g := ""
			switch e.ObjectID {
			case "poster_next", "poster_previous", "dashboard_reload":
				g = "display"
			case "camera_reset":
				g = "camera"
			}
			if g == "" || e.OnPress == nil {
				continue
			}
			_ = nodeRegister(a, connection.NodeOperation{Command: "button." + e.ObjectID, Group: g, Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
				if err := nodeEmpty(raw); err != nil {
					return nil, err
				}
				e.OnPress()
				return map[string]bool{"done": true}, nil
			}})
		case *esphome.Select:
			if (e.ObjectID != "noise_layer_1" && e.ObjectID != "noise_layer_2") || e.OnCommand == nil {
				continue
			}
			_ = nodeRegister(a, connection.NodeOperation{Command: "audio.background." + e.ObjectID, Group: "audio",
				Description: "Read background-noise layer value/options; set value to one of the current entity options. Omitted value reads only.",
				Parameters:  nodeObject(nodeSchema{"value": nodeEnum(e.Options...)}),
				Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
					var p struct {
						Value *string `json:"value"`
					}
					if decodeNode(raw, &p) != nil {
						return nil, errors.New("invalid_params")
					}
					if p.Value != nil {
						valid := false
						for _, v := range e.Options {
							if v == *p.Value {
								valid = true
							}
						}
						if !valid {
							return nil, errors.New("invalid_params")
						}
						e.OnCommand(*p.Value)
					}
					return map[string]any{"value": e.Get(), "options": e.Options}, nil
				}})
		case *esphome.Number:
			if !strings.HasPrefix(e.ObjectID, "volume_") || e.OnCommand == nil {
				continue
			}
			_ = nodeRegister(a, connection.NodeOperation{Command: "audio.volume." + e.ObjectID, Group: "audio",
				Description: "Read this speaker volume entity and its min/max; optional value sets it within the entity's published range. No microphone mute.",
				Parameters:  nodeObject(nodeSchema{"value": nodeNumber("number", e.Min, e.Max)}),
				Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
					var p struct {
						Value *float32 `json:"value"`
					}
					if decodeNode(raw, &p) != nil {
						return nil, errors.New("invalid_params")
					}
					if p.Value != nil {
						if float64(*p.Value) < e.Min || float64(*p.Value) > e.Max {
							return nil, errors.New("invalid_params")
						}
						e.OnCommand(*p.Value)
					}
					return map[string]any{"value": e.Get(), "min": e.Min, "max": e.Max}, nil
				}})
		}
	}
}
