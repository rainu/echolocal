package avatarlab

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"github.com/ygelfand/echolocal/internal/config"
	"github.com/ygelfand/libcountertop/pkg/runtime/webapi"
	setting "github.com/ygelfand/libcountertop/pkg/settings"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// On-device integration: the native helper must be installed. Host runs retain
// the separate descriptor/probe contract tests in internal/avatar.
func TestVideoOwnerSettingsOnDevice(t *testing.T) {
	fixture := os.Getenv("ECHOLOCAL_VIDEO_FIXTURE")
	if fixture == "" {
		t.Skip("requires explicit synthetic video fixture and installed native decoder")
	}
	state := filepath.Join(t.TempDir(), "settings.json")
	config.Use(state)
	registry := &setting.Registry{}
	registry.Add(&setting.Controls[config.Avatar]{Table: NewTable(), Read: config.AvatarSection.Read, Save: func(row setting.Setting[config.Avatar], v string) error {
		return setting.Store(config.AvatarSection, row, v)
	}})
	api := webapi.New(webapi.Options{Registry: registry, Settings: &zoomOwnerSettings{schema.Frontend{Password: "synthetic-owner"}}})
	request := func(method, name, value string, auth bool) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{"value": value})
		req := httptest.NewRequest(method, "/api/v1/settings/avatar/"+name, bytes.NewReader(body))
		req.TLS = &tls.ConnectionState{}
		if auth {
			req.SetBasicAuth("admin", "synthetic-owner")
		}
		out := httptest.NewRecorder()
		api.Handler().ServeHTTP(out, req)
		return out
	}
	for _, name := range []string{"background", "fullscreen_background"} {
		if request(http.MethodPut, name, fixture, false).Code != 401 {
			t.Fatal("owner guard")
		}
		if out := request(http.MethodPut, name, fixture, true); out.Code != 200 {
			t.Fatal(name, out.Code, out.Body.String())
		}
		config.Use(state)
		out := request(http.MethodGet, name, "", true)
		var field setting.Field
		if json.Unmarshal(out.Body.Bytes(), &field) != nil || field.Value != fixture || !strings.Contains(field.Help, "MP4") {
			t.Fatal("persistence/metadata", name)
		}
		bad := filepath.Join(t.TempDir(), "bad.mp4")
		os.WriteFile(bad, []byte("not a video"), 0600)
		if request(http.MethodPut, name, bad, true).Code == 200 {
			t.Fatal("bad video accepted")
		}
		out = request(http.MethodGet, name, "", true)
		json.Unmarshal(out.Body.Bytes(), &field)
		if field.Value != fixture {
			t.Fatal("invalid write changed selection")
		}
		if request(http.MethodPut, name, "", true).Code != 200 {
			t.Fatal("clear")
		}
	}
	for _, name := range []string{"foreground", "fullscreen_foreground"} {
		if request(http.MethodPut, name, fixture, true).Code == 200 {
			t.Fatal("foreground video accepted")
		}
	}
}
