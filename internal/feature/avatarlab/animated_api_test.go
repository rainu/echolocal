package avatarlab

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"github.com/gen2brain/webp"
	"github.com/ygelfand/echolocal/internal/config"
	"github.com/ygelfand/libcountertop/pkg/runtime/webapi"
	setting "github.com/ygelfand/libcountertop/pkg/settings"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
	"image"
	"image/color"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnimatedOwnerSettingsRoundTrip(t *testing.T) {
	root := t.TempDir()
	state := filepath.Join(root, "state.json")
	config.Use(state)
	first, second := image.NewNRGBA(image.Rect(0, 0, 4, 2)), image.NewNRGBA(image.Rect(0, 0, 4, 2))
	first.SetNRGBA(0, 0, color.NRGBA{255, 0, 0, 128})
	second.SetNRGBA(0, 0, color.NRGBA{0, 255, 0, 128})
	p := filepath.Join(root, "scene.webp")
	var b bytes.Buffer
	if e := webp.EncodeAll(&b, &webp.WEBP{Image: []image.Image{first, second}, Delay: []int{100, 200}}, webp.Options{Lossless: true}); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(p, b.Bytes(), 0600)
	bad := filepath.Join(root, "bad.webp")
	os.WriteFile(bad, b.Bytes()[:20], 0600)
	registry := &setting.Registry{}
	registry.Add(&setting.Controls[config.Avatar]{Table: NewTable(), Read: config.AvatarSection.Read, Save: func(row setting.Setting[config.Avatar], v string) error {
		return setting.Store(config.AvatarSection, row, v)
	}})
	api := webapi.New(webapi.Options{Registry: registry, Settings: &zoomOwnerSettings{schema.Frontend{Password: "synthetic-owner"}}})
	for _, name := range []string{"background", "foreground", "fullscreen_background", "fullscreen_foreground"} {
		request := func(method, value string, auth bool) *httptest.ResponseRecorder {
			data, _ := json.Marshal(map[string]string{"value": value})
			req := httptest.NewRequest(method, "/api/v1/settings/avatar/"+name, bytes.NewReader(data))
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
		if out := request(http.MethodPut, p, true); out.Code != 200 {
			t.Fatal(name, out.Code, out.Body.String())
		}
		config.Use(state)
		out := request(http.MethodGet, "", true)
		var field setting.Field
		if json.Unmarshal(out.Body.Bytes(), &field) != nil || field.Value != p || !strings.Contains(field.Help, "WebP") {
			t.Fatal("persist/metadata", name, out.Body.String())
		}
		if request(http.MethodPut, bad, true).Code == 200 {
			t.Fatal("damaged accepted")
		}
		out = request(http.MethodGet, "", true)
		json.Unmarshal(out.Body.Bytes(), &field)
		if field.Value != p {
			t.Fatal("bad asset changed selection")
		}
		if out = request(http.MethodPut, "", true); out.Code != 200 {
			t.Fatal("clear", name, out.Code)
		}
	}
}
