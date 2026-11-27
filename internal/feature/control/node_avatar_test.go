//go:build !board_biscuit

package control

import (
	"context"
	"encoding/json"
	"github.com/ygelfand/echolocal/internal/component"
	"github.com/ygelfand/echolocal/internal/config"
	"github.com/ygelfand/echolocal/internal/feature/avatarlab"
	connection "github.com/ygelfand/echolocal/internal/openclaw"
	setting "github.com/ygelfand/libcountertop/pkg/settings"
	"path/filepath"
	"strings"
	"testing"
)

func TestNodeAvatarSharedRegistryAndPermission(t *testing.T) {
	if !catalogIsolated(t) {
		return
	}
	component.Settings = setting.Registry{}
	value := config.Avatar{}
	c := &setting.Controls[config.Avatar]{Table: avatarlab.NewTable(), Read: func() config.Avatar { return value }, Save: func(s setting.Setting[config.Avatar], v string) error { return s.Write(&value, v) }}
	component.Settings.Add(c)
	a := connection.NewNodeAccess(filepath.Join(t.TempDir(), "permissions"))
	registerNodeSettings(a)
	call := func(cmd, raw string) (any, error) {
		return a.Execute(context.Background(), "echolocal.settings."+cmd, json.RawMessage(raw))
	}
	out, e := call("discover", `{"domain":"avatar"}`)
	b, _ := json.Marshal(out)
	if e != nil || !strings.Contains(string(b), "path") {
		t.Fatal(out, e)
	}
	for _, v := range []string{"/data/local/tmp/custom.vrm", ""} {
		raw, _ := json.Marshal(map[string]string{"domain": "avatar", "name": "path", "value": v})
		if _, e = call("set", string(raw)); e != nil {
			t.Fatal(e)
		}
		out, e = call("get", `{"domain":"avatar","name":"path"}`)
		if e != nil || out.(map[string]any)["value"] != v || value.Path != v {
			t.Fatal(out, e)
		}
	}
	if _, e = call("set", `{"domain":"avatar","name":"path","value":"../bad.vrm"}`); e == nil {
		t.Fatal("invalid path accepted")
	}
	for _, name := range []string{"zoom", "yaw", "pitch", "x", "y", "fullscreen_zoom", "fullscreen_yaw", "fullscreen_pitch", "fullscreen_x", "fullscreen_y"} {
		value := "25"
		if name == "zoom" || name == "fullscreen_zoom" {
			value = "150"
		}
		raw, _ := json.Marshal(map[string]string{"domain": "avatar", "name": name, "value": value})
		if _, e = call("set", string(raw)); e != nil {
			t.Fatal(name, e)
		}
		raw, _ = json.Marshal(map[string]string{"domain": "avatar", "name": name})
		out, e = call("get", string(raw))
		if e != nil || out.(map[string]any)["value"] != value {
			t.Fatal(name, out, e)
		}
	}

	for _, z := range []string{"300", "500", "999"} {
		raw, _ := json.Marshal(map[string]string{"domain": "avatar", "name": "zoom", "value": z})
		if _, e = call("set", string(raw)); e != nil {
			t.Fatal(z, e)
		}
		out, e = call("get", `{"domain":"avatar","name":"zoom"}`)
		if e != nil || out.(map[string]any)["value"] != z {
			t.Fatal(z, out, e)
		}
	}
	before := value
	for _, z := range []string{"49", "1000", "999.1", "NaN"} {
		raw, _ := json.Marshal(map[string]string{"domain": "avatar", "name": "zoom", "value": z})
		if _, e = call("set", string(raw)); e == nil || value != before {
			t.Fatal("invalid Node zoom", z, e)
		}
	}

	for _, value := range []string{"0", "50", "100"} {
		raw, _ := json.Marshal(map[string]string{"domain": "avatar", "name": "mouth_intensity", "value": value})
		if _, e = call("set", string(raw)); e != nil {
			t.Fatal(e)
		}
		out, e = call("get", `{"domain":"avatar","name":"mouth_intensity"}`)
		if e != nil {
			t.Fatal(e)
		}
		b, _ := json.Marshal(out)
		if !strings.Contains(string(b), `"value":"`+value+`"`) {
			t.Fatal(string(b))
		}
	}
	for _, bad := range []string{"-1", "101", "0.5", "NaN"} {
		raw, _ := json.Marshal(map[string]string{"domain": "avatar", "name": "mouth_intensity", "value": bad})
		if _, e = call("set", string(raw)); e == nil {
			t.Fatal("invalid mouth intensity", bad)
		}
	}
	rawBackground, _ := json.Marshal(map[string]string{"domain": "avatar", "name": "background", "value": ""})
	if _, e = call("set", string(rawBackground)); e != nil {
		t.Fatal(e)
	}
	rawBackground, _ = json.Marshal(map[string]string{"domain": "avatar", "name": "background", "value": "/missing.png"})
	if _, e = call("set", string(rawBackground)); e == nil {
		t.Fatal("missing background accepted")
	}
	for _, name := range []string{"foreground", "fullscreen_foreground"} {
		raw, _ := json.Marshal(map[string]string{"domain": "avatar", "name": name, "value": ""})
		if _, e = call("set", string(raw)); e != nil {
			t.Fatal(name, e)
		}
		out, e = call("get", `{"domain":"avatar","name":"`+name+`"}`)
		if e != nil {
			t.Fatal(name, e)
		}
		data, _ := json.Marshal(out)
		if !strings.Contains(string(data), `"value":""`) {
			t.Fatal(name, string(data))
		}
		raw, _ = json.Marshal(map[string]string{"domain": "avatar", "name": name, "value": "/missing.png"})
		if _, e = call("set", string(raw)); e == nil {
			t.Fatal("invalid foreground", name)
		}
	}
	a.SetGrants(map[string]bool{"settings": false})
	for _, cmd := range []string{"get", "set", "discover"} {
		if _, e = call(cmd, `{"domain":"avatar","name":"path","value":"/custom.vrm"}`); e == nil || e.Error() != "permission_denied" {
			t.Fatal(cmd, e)
		}
	}
}
