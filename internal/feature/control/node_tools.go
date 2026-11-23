//go:build !board_biscuit

package control

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/ygelfand/echolocal/internal/component"
	featureclaw "github.com/ygelfand/echolocal/internal/feature/openclaw"
	"github.com/ygelfand/echolocal/internal/feature/vision"
	"github.com/ygelfand/echolocal/internal/hardware/display"
	"github.com/ygelfand/echolocal/internal/layout"
	connection "github.com/ygelfand/echolocal/internal/openclaw"
	harness "github.com/ygelfand/libcountertop/pkg/runtime/control"
)

func init() { featureclaw.NodeSetup = registerNodeTools }
func decodeNode(raw json.RawMessage, v any) error {
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	if len(bytes.TrimSpace(raw)) == 0 || bytes.TrimSpace(raw)[0] != '{' {
		return errors.New("invalid_params")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		return errors.New("invalid_params")
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("invalid_params")
	}
	return nil
}
func nodeEmpty(raw json.RawMessage) error { return decodeNode(raw, &struct{}{}) }
func registerNodeTools(a *connection.NodeAccess) {
	nodeInstallCleanup(a)
	registerNodeRegistry(a)
	registerNodeLifecycle(a)
	registerNodeMaintenance(a)
	registerNodeDisplay(a)
	registerNodeEntities(a)
	add := func(name, group string, run func(context.Context, json.RawMessage) (any, error)) {
		_ = nodeRegister(a, connection.NodeOperation{Command: name, Group: group, Run: run})
	}
	registerNodeScreen(a)
	registerNodeDesktop(a)
	registerNodeConvenience(a)
	registerNodeSettings(a)
	registerNodeDevice(a)
	if component.Board().CameraWidth > 0 {
		add("camera.snap", "camera", func(ctx context.Context, raw json.RawMessage) (any, error) {
			if e := nodeEmpty(raw); e != nil {
				return nil, e
			}
			b, e := vision.Get().Still()
			if e != nil {
				return nil, errors.New("camera_unavailable")
			}
			return map[string]any{"format": "jpeg", "base64": base64.StdEncoding.EncodeToString(b)}, nil
		})
	}
	add("device.info", "state", func(ctx context.Context, raw json.RawMessage) (any, error) {
		if e := nodeEmpty(raw); e != nil {
			return nil, e
		}
		return map[string]any{"version": layout.Version, "commit": layout.GitCommit, "model": component.Board().Model, "board": component.Board().Codename, "deviceName": component.Board().Model, "modelIdentifier": component.Board().Codename, "systemName": "Android", "appVersion": layout.Version, "appBuild": layout.GitCommit}, nil
	})
	for name, path := range map[string][]string{
		"input.tap": {"input", "tap"}, "input.swipe": {"input", "swipe"}, "input.hold": {"input", "hold"},
		"input.button":    {"input", "button"},
		"display.message": {"display", "message"}, "display.clear": {"display", "clear"},
		"display.open":  {"display", "open"},
		"alarm.control": {"device", "alarm"}, "timer.control": {"display", "timer"},
		"intercom.control": {"call"},
	} {
		group := "display"
		switch strings.Split(name, ".")[0] {
		case "input":
			group = "input"
		case "alarm", "timer":
			group = "alarms"
		case "intercom":
			group = "intercom"
		}
		add(name, group, func(ctx context.Context, raw json.RawMessage) (any, error) {
			var p struct {
				Args []string `json:"args"`
			}
			if decodeNode(raw, &p) != nil || len(p.Args) > 32 {
				return nil, errors.New("invalid_params")
			}
			for _, v := range p.Args {
				if len(v) > 4096 || strings.ContainsRune(v, 0) {
					return nil, errors.New("invalid_params")
				}
			}
			if strings.HasPrefix(name, "input.") {
				if !nodeInputAllowed() {
					return nil, errors.New("permission_denied")
				}
				n := 2
				if name == "input.swipe" {
					n = 4
				}
				if name == "input.button" {
					n = 0
				}
				if len(p.Args) < n {
					return nil, errors.New("invalid_params")
				}
				w, h := display.Get().Size()
				for i := 0; i < n; i++ {
					v, e := strconv.Atoi(p.Args[i])
					limit := w
					if i%2 == 1 {
						limit = h
					}
					if e != nil || v < 0 || v >= limit {
						return nil, errors.New("invalid_params")
					}
				}
				if name == "input.swipe" && len(p.Args) > 4 {
					v, e := strconv.Atoi(p.Args[4])
					if e != nil || v < 1 || v > 200 {
						return nil, errors.New("invalid_params")
					}
				}
				if name == "input.hold" && len(p.Args) > 2 {
					v, e := strconv.Atoi(p.Args[2])
					if e != nil || v < 0 || v > 5000 {
						return nil, errors.New("invalid_params")
					}
				}
			}
			// No arbitrary control path, shell, flag, filename or administrative proxy.
			if name == "display.open" {
				if len(p.Args) != 1 || !nodePage(p.Args[0]) {
					return nil, errors.New("invalid_params")
				}
			}
			if name == "display.message" {
				if len(p.Args) < 3 {
					return nil, errors.New("invalid_params")
				}
				v, e := strconv.Atoi(p.Args[1])
				if e != nil || v < 1 || v > 300 {
					return nil, errors.New("invalid_params")
				}
			}
			before := nodeCallID()
			out, e := harness.Execute(ctx, Get().tree, append(append([]string{}, path...), p.Args...))
			if e != nil {
				return nil, errors.New("operation_failed")
			}
			if name == "intercom.control" {
				nodeTrackCall(before)
			}
			// Call control may print peer addressing; never return raw local reports.
			_ = out
			return map[string]bool{"done": true}, nil
		})
	}
}
func nodePage(s string) bool {
	switch s {
	case "settings", "camera":
		return true
	}
	return false
}
