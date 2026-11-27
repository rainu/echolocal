package avatarlab

import (
	"crypto/tls"
	"encoding/json"
	"github.com/ygelfand/echolocal/internal/config"
	"github.com/ygelfand/libcountertop/pkg/runtime/webapi"
	setting "github.com/ygelfand/libcountertop/pkg/settings"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOwnerForegroundWebContract(t *testing.T) {
	root := t.TempDir()
	state := filepath.Join(root, "state.json")
	config.Use(state)
	p := filepath.Join(root, "background.png")
	f, _ := os.Create(p)
	png.Encode(f, image.NewGray(image.Rect(0, 0, 8, 4)))
	f.Close()
	registry := &setting.Registry{}
	c := &setting.Controls[config.Avatar]{Table: NewTable(), Read: config.AvatarSection.Read, Save: func(row setting.Setting[config.Avatar], v string) error {
		return setting.Store(config.AvatarSection, row, v)
	}}
	registry.Add(c)
	api := webapi.New(webapi.Options{Registry: registry, Settings: &zoomOwnerSettings{schema.Frontend{Password: "synthetic-owner"}}})
	request := func(method, value string, auth bool) *httptest.ResponseRecorder {
		data, _ := json.Marshal(map[string]string{"value": value})
		req := httptest.NewRequest(method, "/api/v1/settings/avatar/foreground", strings.NewReader(string(data)))
		req.TLS = &tls.ConnectionState{}
		if auth {
			req.SetBasicAuth("admin", "synthetic-owner")
		}
		out := httptest.NewRecorder()
		api.Handler().ServeHTTP(out, req)
		return out
	}
	if request(http.MethodPut, p, false).Code != 401 {
		t.Fatal("owner guard")
	}
	for _, v := range []string{p, ""} {
		if w := request(http.MethodPut, v, true); w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		config.Use(state)
		w := request(http.MethodGet, "", true)
		var field setting.Field
		if json.Unmarshal(w.Body.Bytes(), &field) != nil || field.Value != v || field.Help == "" {
			t.Fatal("web metadata/read", w.Body.String())
		}
	}
	if request(http.MethodPut, p, true).Code != 200 {
		t.Fatal("write")
	}
	if request(http.MethodPut, "/missing.png", true).Code == 200 || config.AvatarSection.Read().Foreground != p {
		t.Fatal("invalid changed selection")
	}
}

func TestOwnerFullscreenForegroundWebContract(t *testing.T) {
	root := t.TempDir()
	state := filepath.Join(root, "state.json")
	config.Use(state)
	p := filepath.Join(root, "background.png")
	f, _ := os.Create(p)
	png.Encode(f, image.NewGray(image.Rect(0, 0, 8, 4)))
	f.Close()
	registry := &setting.Registry{}
	c := &setting.Controls[config.Avatar]{Table: NewTable(), Read: config.AvatarSection.Read, Save: func(row setting.Setting[config.Avatar], v string) error {
		return setting.Store(config.AvatarSection, row, v)
	}}
	registry.Add(c)
	api := webapi.New(webapi.Options{Registry: registry, Settings: &zoomOwnerSettings{schema.Frontend{Password: "synthetic-owner"}}})
	request := func(method, value string, auth bool) *httptest.ResponseRecorder {
		data, _ := json.Marshal(map[string]string{"value": value})
		req := httptest.NewRequest(method, "/api/v1/settings/avatar/fullscreen_foreground", strings.NewReader(string(data)))
		req.TLS = &tls.ConnectionState{}
		if auth {
			req.SetBasicAuth("admin", "synthetic-owner")
		}
		out := httptest.NewRecorder()
		api.Handler().ServeHTTP(out, req)
		return out
	}
	if request(http.MethodPut, p, false).Code != 401 {
		t.Fatal("owner guard")
	}
	for _, v := range []string{p, ""} {
		if w := request(http.MethodPut, v, true); w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		config.Use(state)
		w := request(http.MethodGet, "", true)
		var field setting.Field
		if json.Unmarshal(w.Body.Bytes(), &field) != nil || field.Value != v || field.Help == "" {
			t.Fatal("web metadata/read", w.Body.String())
		}
	}
	if request(http.MethodPut, p, true).Code != 200 {
		t.Fatal("write")
	}
	if request(http.MethodPut, "/missing.png", true).Code == 200 || config.AvatarSection.Read().FullscreenForeground != p {
		t.Fatal("invalid changed selection")
	}
}
