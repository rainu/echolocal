//go:build !board_biscuit

package control

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ygelfand/echolocal/internal/component"
	connection "github.com/ygelfand/echolocal/internal/openclaw"
	"github.com/ygelfand/libcountertop/pkg/runtime/webapi"
	setting "github.com/ygelfand/libcountertop/pkg/settings"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

type nodeDAVMemory struct{ value schema.Frontend }

func (m *nodeDAVMemory) Read() schema.Frontend                 { return m.value }
func (m *nodeDAVMemory) Update(f func(*schema.Frontend)) error { f(&m.value); return nil }

func TestNodeWebDAVUsesSharedAccessControlAndGrant(t *testing.T) {
	if !catalogIsolated(t) {
		return
	}
	component.Settings = setting.Registry{}
	m := &nodeDAVMemory{value: schema.Frontend{Password: "owner-test"}}
	api := webapi.New(webapi.Options{Settings: m, Registry: &component.Settings, Pages: map[string]http.Handler{
		"webdav": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(207) }),
	}})
	component.Settings.Add(api.Controls())
	a := connection.NewNodeAccess(filepath.Join(t.TempDir(), "permissions"))
	registerNodeSettings(a)
	invoke := func(cmd, raw string) (any, error) {
		return a.Execute(context.Background(), "echolocal.settings."+cmd, json.RawMessage(raw))
	}
	davStatus := func() int {
		req := httptest.NewRequest("PROPFIND", webapi.Prefix+"/webdav/", nil)
		req.TLS = &tls.ConnectionState{}
		req.SetBasicAuth("owner", "owner-test")
		w := httptest.NewRecorder()
		api.Handler().ServeHTTP(w, req)
		return w.Code
	}
	for _, cmd := range []string{"get", "discover"} {
		raw := `{"domain":"access","name":""}`
		if cmd == "discover" {
			raw = `{"domain":"access"}`
		}
		out, err := invoke(cmd, raw)
		data, _ := json.Marshal(out)
		if err != nil || !strings.Contains(string(data), "webdav") || strings.Contains(string(data), "password") {
			t.Fatal("discovery", cmd, err, string(data))
		}
	}
	if davStatus() != 404 {
		t.Fatal("default must remain disabled")
	}
	for _, v := range []string{"on", "off"} {
		if _, err := invoke("set", `{"domain":"access","name":"webdav","value":"`+v+`"}`); err != nil {
			t.Fatal(err)
		}
		out, err := invoke("get", `{"domain":"access","name":"webdav"}`)
		if err != nil || out.(map[string]any)["value"] != v || api.WebDAVEnabled() != (v == "on") {
			t.Fatal("shared value", out, err)
		}
		want := 404
		if v == "on" {
			want = 207
		}
		if davStatus() != want {
			t.Fatal("shared DAV admission", v)
		}
	}
	if _, err := invoke("set", `{"domain":"access","name":"webdav","value":"true"}`); err == nil {
		t.Fatal("invalid toggle accepted")
	}
	if _, err := invoke("import", `{"values":{"access":{"webdav":"on"}}}`); err != nil || !api.WebDAVEnabled() {
		t.Fatal("import", err)
	}
	out, err := invoke("export", "{}")
	data, _ := json.Marshal(out)
	if err != nil || !strings.Contains(string(data), `"webdav":"on"`) || strings.Contains(string(data), "password") || strings.Contains(string(data), "owner-test") {
		t.Fatal("export", err, string(data))
	}
	if err := api.SetWebDAV(false); err != nil {
		t.Fatal(err)
	}
	out, err = invoke("get", `{"domain":"access","name":"webdav"}`)
	if err != nil || out.(map[string]any)["value"] != "off" {
		t.Fatal("reverse API parity", err)
	}
	for _, cmd := range []string{"get", "set"} {
		raw := `{"domain":"access","name":"password"}`
		if cmd == "set" {
			raw = `{"domain":"access","name":"password","value":"replacement"}`
		}
		if _, err := invoke(cmd, raw); err == nil || !strings.HasPrefix(err.Error(), "setting_excluded:") {
			t.Fatal("password boundary", cmd, err)
		}
	}
	if m.value.Password != "owner-test" {
		t.Fatal("password modified")
	}
	if err := a.SetGrants(map[string]bool{"settings": false}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ cmd, raw string }{
		{"get", `{"domain":"access","name":"webdav"}`},
		{"set", `{"domain":"access","name":"webdav","value":"on"}`},
		{"discover", `{"domain":"access"}`},
		{"export", "{}"},
		{"import", `{"values":{"access":{"webdav":"on"}}}`},
	} {
		if _, err := invoke(tc.cmd, tc.raw); err == nil || err.Error() != "permission_denied" {
			t.Fatal("grant boundary", tc.cmd, err)
		}
	}
	if api.WebDAVEnabled() {
		t.Fatal("revoked write took effect")
	}
}
