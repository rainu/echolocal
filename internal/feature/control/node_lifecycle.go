//go:build !board_biscuit

package control

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/ygelfand/echolocal/internal/feature/frontend"
	screens "github.com/ygelfand/echolocal/internal/feature/shell"
	connection "github.com/ygelfand/echolocal/internal/openclaw"
	"github.com/ygelfand/echolocal/internal/update"
	sharedshell "github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/input/textentry"
)

func nodeInputAllowed() bool {
	for _, v := range screens.Get().Stack() {
		if p, ok := v.View.(*sharedshell.Page); ok {
			title := strings.ToLower(p.Title)
			if strings.Contains(title, "openclaw") || strings.Contains(title, "node-") {
				return false
			}
		}
	}
	return true
}
func registerNodeLifecycle(a *connection.NodeAccess) {
	_ = nodeRegister(a, connection.NodeOperation{Command: "input.text", Group: "input", Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p textentry.Request
		if decodeNode(raw, &p) != nil || len(p.Text) > textentry.TextMost || !nodeInputAllowed() {
			return nil, errors.New("invalid_params")
		}
		switch p.Action {
		case "", "replace", "save", "cancel":
		default:
			return nil, errors.New("invalid_params")
		}
		c := frontend.Get().Input()
		view, e := c.Call(ctx, textentry.Request{})
		if e != nil || view.Password {
			return nil, errors.New("input_unavailable")
		}
		if p.Action != "" {
			if p.Target == "" || p.Target != view.Target || p.Revision != view.Revision {
				return nil, errors.New("input_conflict")
			}
			if !a.Granted("input") || !nodeInputAllowed() {
				return nil, errors.New("permission_denied")
			}
			view, e = c.Call(ctx, p)
			if e != nil {
				return nil, errors.New("input_conflict")
			}
		}
		// Existing text can contain personal information or credentials: only
		// lifecycle metadata is returned. Password fields are not Node-editable.
		return map[string]any{"target": view.Target, "revision": view.Revision}, nil
	}})
	_ = nodeRegister(a, connection.NodeOperation{Command: "maintenance.restart", Group: "maintenance", Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p struct {
			Confirm bool `json:"confirm"`
		}
		if decodeNode(raw, &p) != nil || !p.Confirm {
			return nil, errors.New("confirmation_required")
		}
		update.Restart("Node maintenance restart")
		return map[string]bool{"scheduled": true}, nil
	}})
	_ = nodeRegister(a, connection.NodeOperation{Command: "diagnostics.status", Group: "diagnostics", Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
		if e := nodeEmpty(raw); e != nil {
			return nil, e
		}
		c := featureNodeStatus()
		return c, nil
	}})
}
func featureNodeStatus() map[string]any {
	return map[string]any{"source": "bounded local diagnostics", "inputAvailable": nodeInputAllowed()}
}
