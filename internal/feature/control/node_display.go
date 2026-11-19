//go:build !board_biscuit

package control

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/ygelfand/echolocal/internal/feature/call"
	"github.com/ygelfand/echolocal/internal/feature/dashboard"
	connection "github.com/ygelfand/echolocal/internal/openclaw"
)

func registerNodeDisplay(a *connection.NodeAccess) {
	_ = a.Register(connection.NodeOperation{Command: "display.pages", Group: "display", Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
		if e := nodeEmpty(raw); e != nil {
			return nil, e
		}
		return dashboard.Tabs().List(), nil
	}})
	_ = a.Register(connection.NodeOperation{Command: "display.page", Group: "display", Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p struct {
			Key string `json:"key"`
		}
		if decodeNode(raw, &p) != nil || len(p.Key) > 256 {
			return nil, errors.New("invalid_params")
		}
		if p.Key == "" {
			dashboard.Tabs().Clock()
			return map[string]bool{"done": true}, nil
		}
		for _, tab := range dashboard.Tabs().List() {
			if tab.Key == p.Key {
				dashboard.Tabs().Show(p.Key)
				return map[string]bool{"done": true}, nil
			}
		}
		return nil, errors.New("unsupported_page")
	}})
	_ = a.Register(connection.NodeOperation{Command: "intercom.state", Group: "intercom", Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
		if e := nodeEmpty(raw); e != nil {
			return nil, e
		}
		c, on := call.Get().Now()
		return map[string]any{"active": on, "state": c.State, "video": c.Video, "muted": c.Muted, "cameraOff": c.CameraOff}, nil
	}})
}
