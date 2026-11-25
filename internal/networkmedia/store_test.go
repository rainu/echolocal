package networkmedia

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func fixture(t *testing.T) *Store { t.Helper(); return New(filepath.Join(t.TempDir(), "smb.json")) }
func configured(t *testing.T) *Store {
	t.Helper()
	s := fixture(t)
	pw := "synthetic-password"
	if e := s.Save(Update{Settings: Settings{Enabled: true, Host: "server.example", Port: 445, Share: "videos", Username: "reader"}, Password: &pw}); e != nil {
		t.Fatal(e)
	}
	return s
}
func TestPasswordWriteOnlyPersistenceAndClear(t *testing.T) {
	s := configured(t)
	b, _ := json.Marshal(s.View())
	if strings.Contains(string(b), "synthetic-password") {
		t.Fatal("secret read")
	}
	mode, _ := os.Stat(s.path)
	if mode.Mode().Perm() != 0600 {
		t.Fatal("permissions")
	}
	s = New(s.path)
	if !s.View().PasswordSet || !s.View().Enabled {
		t.Fatal("restart")
	}
	calls := 0
	s.Changed = func() { calls++ }
	u := Update{Settings: s.View().Settings}
	u.Share = "other"
	if e := s.Save(u); e != nil {
		t.Fatal(e)
	}
	c, _ := s.snapshot()
	if c.Password != "synthetic-password" || calls != 1 {
		t.Fatal("omission did not retain secret")
	}
	u.Enabled = false
	u.ClearPassword = true
	if e := s.Save(u); e != nil {
		t.Fatal(e)
	}
	if New(s.path).View().PasswordSet {
		t.Fatal("clear not durable")
	}
}
func TestValidationAndAtomicFailures(t *testing.T) {
	s := configured(t)
	before, _ := s.snapshot()
	for _, edit := range []func(*Update){
		func(u *Update) { u.Host = "user:pass@host" }, func(u *Update) { u.Port = 0 }, func(u *Update) { u.Share = "../videos" }, func(u *Update) { u.ClearPassword = true }, func(u *Update) { x := ""; u.Password = &x }, func(u *Update) { u.Domain = "bad\nvalue" },
	} {
		u := Update{Settings: before.Settings}
		edit(&u)
		if s.Save(u) == nil {
			t.Fatal("invalid saved")
		}
		after, _ := s.snapshot()
		if !reflect.DeepEqual(before, after) {
			t.Fatal("invalid mutated")
		}
	}
}
func TestHTTPDoesNotEchoRequestsOrSecrets(t *testing.T) {
	s := configured(t)
	for _, body := range []string{`{"enabled":true,"host":"bad-secret-value@host"}`, `{"password":"do-not-echo","extra":1}`, `{} {}`} {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest("PUT", "/", strings.NewReader(body)))
		if w.Code != 400 || strings.Contains(w.Body.String(), "do-not-echo") || strings.Contains(w.Body.String(), "bad-secret-value") {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), "synthetic-password") {
		t.Fatal("unsafe GET")
	}
}
func TestByteRanges(t *testing.T) {
	for _, tc := range []struct {
		raw      string
		from, to int64
	}{{"", 0, 99}, {"bytes=0-", 0, 99}, {"bytes=5-9", 5, 9}, {"bytes=95-200", 95, 99}} {
		a, b, e := byteRange(tc.raw, 100)
		if e != nil || a != tc.from || b != tc.to {
			t.Fatal(tc, e)
		}
	}
	for _, raw := range []string{"bytes=100-", "bytes=-5", "bytes=9-5", "bytes=0-1,4-5", "bad", "bytes=x-", "bytes=0-99999999999999999999"} {
		if _, _, e := byteRange(raw, 100); e == nil {
			t.Fatal(raw)
		}
	}
}
func TestSettingsCoverageInventory(t *testing.T) {
	b, e := os.ReadFile("../../docs/openclaw-owner-coverage.json")
	if e != nil {
		t.Fatal(e)
	}
	var m struct {
		Stores map[string]map[string]struct{ Device, Web, API string }
	}
	if e = json.Unmarshal(b, &m); e != nil {
		t.Fatal(e)
	}
	typ := reflect.TypeFor[Settings]()
	for i := 0; i < typ.NumField(); i++ {
		key := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
		row := m.Stores["SMBSettings"][key]
		if row.Device == "" || row.Web == "" || row.API == "" {
			t.Error("missing owner coverage", key)
		}
	}
	for _, lang := range []string{"en", "de", "it", "vi"} {
		for key := range messages["en"] {
			if messages[lang][key] == "" {
				t.Error("missing label", lang, key)
			}
		}
	}
}
