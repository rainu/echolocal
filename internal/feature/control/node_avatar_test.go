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
	a.SetGrants(map[string]bool{"settings": false})
	for _, cmd := range []string{"get", "set", "discover"} {
		if _, e = call(cmd, `{"domain":"avatar","name":"path","value":"/custom.vrm"}`); e == nil || e.Error() != "permission_denied" {
			t.Fatal(cmd, e)
		}
	}
}
