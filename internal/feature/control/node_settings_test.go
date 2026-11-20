//go:build !board_biscuit

package control

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/ygelfand/echolocal/internal/board"
	"github.com/ygelfand/echolocal/internal/component"
	connection "github.com/ygelfand/echolocal/internal/openclaw"
	setting "github.com/ygelfand/libcountertop/pkg/settings"
	"path/filepath"
	"strings"
	"testing"
)

func TestNodeSettingsDiscoveryAndParserBoundaries(t *testing.T) {
	if !catalogIsolated(t) {
		return
	}
	component.Settings = setting.Registry{}
	type config struct {
		On    bool
		Level int
		Mode  string
		Text  string
	}
	at := config{Level: 25, Mode: "wire"}
	saves, reads, secretReads := 0, 0, 0
	makeBound := func(domain string, rows []setting.Setting[config]) *setting.Controls[config] {
		return &setting.Controls[config]{Table: setting.NewTable(domain, []setting.Group{"test"}, rows, setting.Messages{}), Read: func() config { reads++; return at }, Save: func(s setting.Setting[config], v string) error { saves++; return s.Write(&at, v) }}
	}
	toggle := func(name string) setting.Setting[config] {
		return setting.Setting[config]{Name: name, Kind: setting.Toggle, Group: "test", NoEntity: true, Read: func(c *config) string { return setting.OnOff(c.On) }, Write: func(c *config, v string) error {
			on, ok := setting.Boolean(v)
			if !ok {
				return errors.New("synthetic sensitive input " + v)
			}
			c.On = on
			return nil
		}}
	}
	cast := makeBound("cast", []setting.Setting[config]{toggle("receiver"), {Name: "private", Kind: setting.Credential, Read: func(*config) string { secretReads++; return "synthetic-secret" }}})
	// These tables deliberately share the media domain.
	component.Settings.Add(cast, makeBound("media", []setting.Setting[config]{{Name: "duck", Kind: setting.Number, Group: "test", NoEntity: true, Min: 0, Max: 100, Read: func(c *config) string { return "25" }, Write: func(c *config, v string) error { c.Level = 42; return nil }}}), makeBound("media", []setting.Setting[config]{toggle("sendspin")}))
	component.Settings.Add(makeBound("screen", []setting.Setting[config]{{Name: "style", Kind: setting.Choice, Group: "test", NoEntity: true, Options: []setting.Option{{Value: "wire", Label: "Localized label"}}, Read: func(c *config) string { return c.Mode }, Write: func(c *config, v string) error { c.Mode = v; return nil }}}))
	component.Settings.Add(makeBound("clock", []setting.Setting[config]{{Name: "date_pattern", Kind: setting.Text, Group: "test", NoEntity: true, Read: func(c *config) string { return c.Text }, Write: func(c *config, v string) error { c.Text = v; return nil }}}))
	// The real Apps table uses app_order, not apps_order. Authorization must
	// follow bound Domain/Name, never infer it from this different entity ID.
	component.Settings.Add(makeBound("apps", []setting.Setting[config]{{Name: "order", ID: "app_order", Kind: setting.Text, Group: "test", NoEntity: true, Read: func(c *config) string { return c.Text }, Write: func(c *config, v string) error { c.Text = v; return nil }}}))
	a := connection.NewNodeAccess(filepath.Join(t.TempDir(), "permissions"))
	registerNodeSettings(a)
	invoke := func(command, raw string) (any, error) {
		return a.Execute(context.Background(), "echolocal.settings."+command, json.RawMessage(raw))
	}
	out, err := invoke("discover", "{}")
	if err != nil {
		t.Fatal(err)
	}
	fallback, fallbackErr := invoke("get", `{"domain":"","name":""}`)
	left, _ := json.Marshal(out)
	right, _ := json.Marshal(fallback)
	if fallbackErr != nil || string(left) != string(right) {
		t.Fatal("existing get discovery fallback differs")
	}
	data, _ := json.Marshal(out)
	if reads != 0 || saves != 0 || secretReads != 0 || strings.Contains(string(data), "private") || strings.Contains(string(data), "synthetic-secret") || strings.Contains(string(data), "wire") || strings.Contains(string(data), "\"choices\":") {
		t.Fatal("root index collected values or bulky choice metadata")
	}
	if len(data) > 4096 {
		t.Fatal("fixture index unexpectedly unbounded")
	}
	for _, want := range []string{"receiver", "sendspin", "requiredGrants", "domains"} {
		if !strings.Contains(string(data), want) {
			t.Fatal("missing compact discovery identifier", want)
		}
	}
	castDetails, err := invoke("get", `{"domain":"cast","name":""}`)
	if err != nil {
		t.Fatal(err)
	}
	data, _ = json.Marshal(castDetails)
	if !strings.Contains(string(data), "on | off") || !strings.Contains(string(data), "requiredGrants") {
		t.Fatal("domain details omitted value formats or grants")
	}
	screenDetails, err := invoke("discover", `{"domain":"screen"}`)
	if err != nil {
		t.Fatal(err)
	}
	data, _ = json.Marshal(screenDetails)
	if !strings.Contains(string(data), "wire") {
		t.Fatal("domain details omitted canonical choices")
	}
	for _, tc := range []struct{ raw, want string }{
		{`{"domain":"playback","name":"cast_receiver"}`, "unsupported_domain:"},
		{`{"domain":"cast","name":"missing"}`, "unknown_setting:"},
		{`{"domain":"screen","name":"lock_pin"}`, "setting_excluded:"},
		{`{"domain":"clock","name":"face"}`, "setting_unavailable:"},
	} {
		_, err := invoke("get", tc.raw)
		if err == nil || !strings.HasPrefix(err.Error(), tc.want) {
			t.Fatal(tc.want, err)
		}
		if (tc.want == "unsupported_domain:" || tc.want == "unknown_setting:") && !strings.Contains(err.Error(), "settings.get with domain=\"\" and name=\"\"") {
			t.Fatal("recovery hint omitted existing get route")
		}

	}
	for _, raw := range []string{`{"domain":"cast","name":"receiver","value":"synthetic-secret"}`, `{"domain":"media","name":"duck","value":"101"}`, `{"domain":"screen","name":"style","value":"Localized label"}`} {
		_, err := invoke("set", raw)
		if err == nil || !strings.HasPrefix(err.Error(), "invalid_value:") || strings.Contains(err.Error(), "synthetic-secret") {
			t.Fatal("bad parser diagnostic", err)
		}
	}
	if saves != 0 {
		t.Fatal("invalid value saved")
	}
	for _, raw := range []string{`{"domain":"cast","name":"receiver","value":"on"}`, `{"domain":"media","name":"sendspin","value":"off"}`, `{"domain":"media","name":"duck","value":"42"}`, `{"domain":"clock","name":"date_pattern","value":"dd.MM.yyyy"}`} {
		if _, err := invoke("set", raw); err != nil {
			t.Fatal("valid settings rejected", err)
		}
	}
	if saves != 4 || secretReads != 0 {
		t.Fatal("wrong writes or secret read")
	}
	out, err = invoke("get", `{"domain":"screen","name":"style"}`)
	if err != nil || out.(map[string]any)["value"] != "wire" {
		t.Fatal("localized wire value", err)
	}
	if _, err := invoke("set", `{"domain":"apps","name":"order","value":"weather,clock"}`); err != nil {
		t.Fatal("entity ID mismatch blocked bound setting", err)
	}
	before := saves
	_, err = invoke("import", `{"values":{"cast":{"receiver":"on"},"media":{"duck":"999"}}}`)
	if err == nil || saves != before {
		t.Fatal("import did not prevalidate all values")
	}
	if _, err := invoke("import", `{"values":{"media":{"duck":"40","sendspin":"on"}}}`); err != nil {
		t.Fatal("duplicate-domain import failed", err)
	}
	if _, err := invoke("export", "{}"); err != nil || secretReads != 0 {
		t.Fatal("export read excluded field", err)
	}
	if err := a.SetGrants(map[string]bool{"audio": false}); err != nil {
		t.Fatal(err)
	}
	if _, err := invoke("get", `{"domain":"cast","name":"receiver"}`); err == nil || err.Error() != "permission_denied" {
		t.Fatal("audio grant bypass", err)
	}
	out, err = invoke("discover", "{}")
	data, _ = json.Marshal(out)
	if err != nil || strings.Contains(string(data), "receiver") {
		t.Fatal("discovery ignored grant", err)
	}
	if err := a.SetGrants(map[string]bool{"settings": false}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []string{"discover", "get", "set", "import", "export"} {
		if _, err := invoke(c, "{}"); err == nil || err.Error() != "permission_denied" {
			t.Fatal("settings revocation bypass", c, err)
		}
	}
}

func TestNodeSettingHardwareAvailability(t *testing.T) {
	was := component.Board()
	defer component.Default().Use(was)
	for _, b := range board.All {
		component.Default().Use(b)
		if NodeSettingAvailable("ring", "muted") != b.Has(board.Ring) {
			t.Fatal("ring availability mismatch", b.Codename)
		}
		if NodeSettingAvailable("microphone", "mute_led") != b.Has(board.MuteLight) {
			t.Fatal("mute LED availability mismatch", b.Codename)
		}
		if NodeSettingAvailable("camera", "ev") != (b.CameraWidth > 0) {
			t.Fatal("camera availability mismatch", b.Codename)
		}
	}
}

func TestNodeSettingDiscoveryNumericBounds(t *testing.T) {
	for _, f := range []setting.Field{{Name: "interval", Kind: "number", Min: 0, Max: 100, Unit: "s"}, {Name: "seed", Kind: "number"}} {
		s := nodeSettingDescription("diag", f)
		if f.Max == 100 && (s.Min == nil || *s.Min != 0 || s.Max == nil || *s.Max != 100 || s.Unit != "s") {
			t.Fatal("declared zero lower bound or unit lost")
		}
		if f.Max == 0 && (s.Min != nil || s.Max != nil) {
			t.Fatal("invented an undeclared zero-width numeric range")
		}
	}
}
