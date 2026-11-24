package avatarlab

import (
	"crypto/tls"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/ygelfand/echolocal/internal/avatar"
	"github.com/ygelfand/echolocal/internal/config"
	"github.com/ygelfand/libcountertop/pkg/runtime/webapi"
	setting "github.com/ygelfand/libcountertop/pkg/settings"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

type zoomOwnerSettings struct{ value schema.Frontend }

func (m *zoomOwnerSettings) Read() schema.Frontend                 { return m.value }
func (m *zoomOwnerSettings) Update(f func(*schema.Frontend)) error { f(&m.value); return nil }

func TestOwnerWebZoomContractPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "avatar.json")
	config.Use(path)
	registry := &setting.Registry{}
	c := &setting.Controls[config.Avatar]{Table: NewTable(), Read: config.AvatarSection.Read, Save: func(row setting.Setting[config.Avatar], v string) error {
		return setting.Store(config.AvatarSection, row, v)
	}}
	registry.Add(c)
	api := webapi.New(webapi.Options{Registry: registry, Settings: &zoomOwnerSettings{schema.Frontend{Password: "synthetic-owner"}}})
	request := func(method, name, body string, auth bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/v1/settings/avatar/"+name, strings.NewReader(body))
		req.TLS = &tls.ConnectionState{}
		if auth {
			req.SetBasicAuth("admin", "synthetic-owner")
		}
		w := httptest.NewRecorder()
		api.Handler().ServeHTTP(w, req)
		return w
	}
	if got := request(http.MethodPut, "zoom", `{"value":"999"}`, false).Code; got != 401 {
		t.Fatal("unauthenticated", got)
	}
	for _, lang := range []string{"en", "de", "it", "vi"} {
		if !strings.Contains(avatar.Text(lang, "zoom_help"), "999") {
			t.Fatal("range help", lang)
		}
	}
	for _, zoom := range []int{300, 500, 999} {
		w := request(http.MethodPut, "zoom", `{"value":"`+strconv.Itoa(zoom)+`"}`, true)
		if w.Code != 200 {
			t.Fatal("write", zoom, w.Code, w.Body.String())
		}
		config.Use(path)
		w = request(http.MethodGet, "zoom", "", true)
		var field setting.Field
		if err := json.Unmarshal(w.Body.Bytes(), &field); err != nil || w.Code != 200 || field.Min != 50 || field.Max != 999 || field.Value != strconv.Itoa(zoom) {
			t.Fatal("restored web metadata", field, w.Code, err)
		}
	}
	before := config.AvatarSection.Read()
	for _, bad := range []string{"49", "1000", "999.1", "NaN"} {
		if w := request(http.MethodPut, "zoom", `{"value":"`+bad+`"}`, true); w.Code != 422 || !reflect.DeepEqual(config.AvatarSection.Read(), before) {
			t.Fatal("invalid web write", bad, w.Code)
		}
	}
	for _, axis := range []string{"x", "y"} {
		for _, v := range []string{"-500", "500"} {
			if w := request(http.MethodPut, axis, `{"value":"`+v+`"}`, true); w.Code != 200 {
				t.Fatal(axis, v, w.Code)
			}
		}
		if w := request(http.MethodPut, axis, `{"value":"501"}`, true); w.Code != 422 {
			t.Fatal(axis, w.Code)
		}
	}
}

func TestOwnerBackgroundWebContract(t *testing.T) {
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
		req := httptest.NewRequest(method, "/api/v1/settings/avatar/background", strings.NewReader(string(data)))
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
		if json.Unmarshal(w.Body.Bytes(), &field) != nil || field.Value != v || field.Help != "" {
			t.Fatal("web metadata/read", w.Body.String())
		}
	}
	if request(http.MethodPut, p, true).Code != 200 {
		t.Fatal("write")
	}
	if request(http.MethodPut, "/missing.png", true).Code == 200 || config.AvatarSection.Read().Background != p {
		t.Fatal("invalid changed selection")
	}
}

func TestOwnerWebMouthIntensityContractPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "avatar.json")
	config.Use(path)
	registry := &setting.Registry{}
	c := &setting.Controls[config.Avatar]{Table: NewTable(), Read: config.AvatarSection.Read, Save: func(row setting.Setting[config.Avatar], v string) error {
		return setting.Store(config.AvatarSection, row, v)
	}}
	registry.Add(c)
	api := webapi.New(webapi.Options{Registry: registry, Settings: &zoomOwnerSettings{schema.Frontend{Password: "synthetic-owner"}}})
	request := func(method, name, body string, auth bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/v1/settings/avatar/"+name, strings.NewReader(body))
		req.TLS = &tls.ConnectionState{}
		if auth {
			req.SetBasicAuth("admin", "synthetic-owner")
		}
		w := httptest.NewRecorder()
		api.Handler().ServeHTTP(w, req)
		return w
	}
	if got := request(http.MethodPut, "mouth_intensity", `{"value":"100"}`, false).Code; got != 401 {
		t.Fatal("unauthenticated", got)
	}
	for _, lang := range []string{"en", "de", "it", "vi"} {
		if !strings.Contains(avatar.Text(lang, "mouth_intensity_help"), "100") {
			t.Fatal("range help", lang)
		}
	}
	for _, zoom := range []int{0, 50, 100} {
		w := request(http.MethodPut, "mouth_intensity", `{"value":"`+strconv.Itoa(zoom)+`"}`, true)
		if w.Code != 200 {
			t.Fatal("write", zoom, w.Code, w.Body.String())
		}
		config.Use(path)
		w = request(http.MethodGet, "mouth_intensity", "", true)
		var field setting.Field
		if err := json.Unmarshal(w.Body.Bytes(), &field); err != nil || w.Code != 200 || field.Min != 0 || field.Max != 100 || field.Value != strconv.Itoa(zoom) {
			t.Fatal("restored web metadata", field, w.Code, err)
		}
	}
	before := config.AvatarSection.Read()
	for _, bad := range []string{"-1", "101", "50.5", "NaN"} {
		if w := request(http.MethodPut, "mouth_intensity", `{"value":"`+bad+`"}`, true); w.Code != 422 || !reflect.DeepEqual(config.AvatarSection.Read(), before) {
			t.Fatal("invalid web write", bad, w.Code)
		}
	}
	for _, axis := range []string{"x", "y"} {
		for _, v := range []string{"-500", "500"} {
			if w := request(http.MethodPut, axis, `{"value":"`+v+`"}`, true); w.Code != 200 {
				t.Fatal(axis, v, w.Code)
			}
		}
		if w := request(http.MethodPut, axis, `{"value":"501"}`, true); w.Code != 422 {
			t.Fatal(axis, w.Code)
		}
	}
}

func TestFullscreenOwnerWebPersistenceAndValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	config.Use(path)
	registry := &setting.Registry{}
	c := &setting.Controls[config.Avatar]{Table: NewTable(), Read: config.AvatarSection.Read, Save: func(row setting.Setting[config.Avatar], v string) error {
		return setting.Store(config.AvatarSection, row, v)
	}}
	registry.Add(c)
	api := webapi.New(webapi.Options{Registry: registry, Settings: &zoomOwnerSettings{schema.Frontend{Password: "synthetic-owner"}}})
	request := func(name, value string, auth bool) *httptest.ResponseRecorder {
		b, _ := json.Marshal(map[string]string{"value": value})
		req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/avatar/"+name, strings.NewReader(string(b)))
		req.TLS = &tls.ConnectionState{}
		if auth {
			req.SetBasicAuth("admin", "synthetic-owner")
		}
		w := httptest.NewRecorder()
		api.Handler().ServeHTTP(w, req)
		return w
	}
	if request("talk_mode", "fullscreen", false).Code != 401 {
		t.Fatal("owner guard")
	}
	for name, value := range map[string]string{"talk_mode": "fullscreen", "fullscreen_zoom": "225", "fullscreen_yaw": "-30", "fullscreen_pitch": "20", "fullscreen_x": "-50", "fullscreen_y": "75"} {
		if w := request(name, value, true); w.Code != 200 {
			t.Fatal(name, w.Code, w.Body.String())
		}
	}
	config.Use(path)
	a := config.AvatarSection.Read()
	if a.Mode() != "fullscreen" || a.FullscreenZoom != 225 || a.Zoom != 0 || a.Background != "" || a.FullscreenX != -50 {
		t.Fatal("persisted independent settings", a)
	}
	for name, value := range map[string]string{"talk_mode": "bad", "fullscreen_zoom": "1000", "fullscreen_pitch": "86", "fullscreen_yaw": "181", "fullscreen_x": "501", "fullscreen_y": "NaN", "fullscreen_background": "/not-present.png"} {
		if request(name, value, true).Code != 422 || config.AvatarSection.Read() != a {
			t.Fatal("invalid mutated settings", name)
		}
	}
}
