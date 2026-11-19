//go:build !board_biscuit

package control

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/ygelfand/echolocal/internal/component"
	"github.com/ygelfand/echolocal/internal/feature/message"
	"github.com/ygelfand/echolocal/internal/feature/mute"
	"github.com/ygelfand/echolocal/internal/layout"
	connection "github.com/ygelfand/echolocal/internal/openclaw"
	"time"
)

func registerNodeDevice(a *connection.NodeAccess) {
	add := func(name, group string, run func(context.Context, json.RawMessage) (any, error)) {
		_ = nodeRegister(a, connection.NodeOperation{Command: name, Group: group, Run: run})
	}
	add("device.status", "state", func(ctx context.Context, raw json.RawMessage) (any, error) {
		if nodeEmpty(raw) != nil {
			return nil, errors.New("invalid_params")
		}
		out := map[string]any{}
		if b, err := os.ReadFile("/proc/uptime"); err == nil {
			fields := strings.Fields(string(b))
			if len(fields) > 0 {
				if v, e := strconv.ParseFloat(fields[0], 64); e == nil {
					out["uptimeSeconds"] = v
				}
			}
		}
		var st syscall.Statfs_t
		if syscall.Statfs(layout.StateDir, &st) == nil {
			total := uint64(st.Blocks) * uint64(st.Bsize)
			free := uint64(st.Bavail) * uint64(st.Bsize)
			out["storage"] = map[string]uint64{"totalBytes": total, "freeBytes": free, "usedBytes": total - free}
		}
		return out, nil
	})
	add("device.health", "state", func(ctx context.Context, raw json.RawMessage) (any, error) {
		if nodeEmpty(raw) != nil {
			return nil, errors.New("invalid_params")
		}
		out := map[string]any{}
		if b, err := os.ReadFile("/proc/meminfo"); err == nil {
			vals := map[string]uint64{}
			for _, line := range strings.Split(string(b), "\n") {
				f := strings.Fields(line)
				if len(f) >= 2 && (f[0] == "MemTotal:" || f[0] == "MemAvailable:") {
					n, e := strconv.ParseUint(f[1], 10, 64)
					if e == nil {
						vals[f[0]] = n * 1024
					}
				}
			}
			total, avail := vals["MemTotal:"], vals["MemAvailable:"]
			if total > 0 && avail <= total {
				out["memory"] = map[string]any{"totalRamBytes": total, "availableRamBytes": avail, "usedRamBytes": total - avail}
			}
		}
		return out, nil
	})
	add("device.permissions", "state", func(ctx context.Context, raw json.RawMessage) (any, error) {
		if nodeEmpty(raw) != nil {
			return nil, errors.New("invalid_params")
		}
		permission := func(on bool) any {
			status := "denied"
			if on {
				status = "granted"
			}
			return map[string]any{"status": status, "promptable": false}
		}
		muted, err := mute.Get().Muted()
		grants := map[string]bool{}
		for _, g := range connection.PermissionGroups {
			grants[g] = a.Granted(g)
		}
		return map[string]any{"permissions": map[string]any{
			"camera":          permission(component.Board().CameraWidth > 0 && a.Granted("camera")),
			"microphone":      permission(err == nil && !muted && a.Granted("talk")),
			"screenRecording": permission(a.Granted("screen")),
		}, "echolocal": map[string]any{"grants": grants, "source": "local node policy and hardware availability"}}, nil
	})
	add("system.notify", "display", func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p struct {
			Title    string `json:"title"`
			Body     string `json:"body"`
			Sound    string `json:"sound"`
			Priority string `json:"priority"`
			Delivery string `json:"delivery"`
		}
		if decodeNode(raw, &p) != nil || len(p.Title) > 256 || p.Body == "" || len(p.Body) > 4096 {
			return nil, errors.New("invalid_params")
		}
		if (p.Delivery != "" && p.Delivery != "overlay" && p.Delivery != "auto") || p.Sound != "" {
			return nil, errors.New("unsupported_option")
		}
		// A quiet local overlay: it cannot implicitly use audio or microphone grants.
		message.Get().Quiet(p.Title, p.Body, 15*time.Second)
		return map[string]bool{"delivered": true}, nil
	})
}
