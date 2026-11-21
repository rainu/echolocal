//go:build !board_biscuit

package openclawconfig

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ygelfand/echolocal/internal/feature/clawtalk"
	"github.com/ygelfand/echolocal/internal/openclaw"
	"github.com/ygelfand/libcountertop/pkg/runtime/webapi"
	"github.com/ygelfand/libcountertop/pkg/say"
	setting "github.com/ygelfand/libcountertop/pkg/settings"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

type memory struct{ v schema.Frontend }

func (m *memory) Read() schema.Frontend                 { return m.v }
func (m *memory) Update(f func(*schema.Frontend)) error { f(&m.v); return nil }

type catalog struct {
	fail    bool
	methods []string
}

func (c *catalog) CallTalk(_ context.Context, method string, _ any, result any) error {
	c.methods = append(c.methods, method)
	if c.fail {
		return errors.New("PRIVATE_GATEWAY_ERROR")
	}
	data := map[string]string{
		"agents.list":   `{"agents":[{"id":"test-agent","identity":{"name":"Test agent"}}]}`,
		"sessions.list": `{"sessions":[{"key":"agent:test-agent:existing","label":"Test session"},{"key":"agent:other:private","label":"Other session"}]}`,
		"talk.catalog":  `{"realtime":{"providers":[{"id":"openai","label":"OpenAI","configured":true,"transports":["webrtc"],"brains":["agent-consult"],"voicesByModel":{"native-test":["test"]}}]}}`,
	}[method]
	if data == "" {
		return errors.New("unexpected RPC")
	}
	return json.Unmarshal([]byte(data), result)
}

type fixture struct {
	a        *API
	h        http.Handler
	dir      string
	calls    *catalog
	changed  int
	previews int
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{dir: t.TempDir(), calls: &catalog{}}
	ui := openclaw.New(filepath.Join(f.dir, "ui.json"), "test")
	node := openclaw.NewNode(filepath.Join(f.dir, "node.json"), "test", openclaw.NodeMetadata{Name: "Test Node", Model: "test"})
	node.AttachNodeAccess(openclaw.NewNodeAccess(filepath.Join(f.dir, "permissions.json")))
	g := openclaw.NewConnections(filepath.Join(f.dir, "connection.json"), ui, node, "Test UI", "Test Node")
	f.a = New(Options{Connections: g, Store: clawtalk.NewStore(filepath.Join(f.dir, "slots.json")), Caller: f.calls, Changed: func() { f.changed++ }, Preview: func(string, string) { f.previews++ }})
	w := webapi.New(webapi.Options{Settings: &memory{schema.Frontend{Password: "SYNTHETIC_OWNER_PASSWORD"}}, Registry: &setting.Registry{}, Device: func() webapi.Device { return webapi.Device{Name: "test"} }, Pages: map[string]http.Handler{"openclaw": f.a.Handler()}})
	f.h = w.Handler()
	return f
}
func (f *fixture) request(t *testing.T, method, path string, body any, auth bool) *httptest.ResponseRecorder {
	t.Helper()
	raw := ""
	if body != nil {
		b, e := json.Marshal(body)
		if e != nil {
			t.Fatal(e)
		}
		raw = string(b)
	}
	r := httptest.NewRequest(method, "https://device.invalid/api/v1/openclaw"+path, strings.NewReader(raw))
	if auth {
		r.SetBasicAuth("owner", "SYNTHETIC_OWNER_PASSWORD")
	}
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, r)
	return w
}
func requireCode(t *testing.T, w *httptest.ResponseRecorder, code int) {
	t.Helper()
	if w.Code != code {
		t.Fatalf("status %d, expected %d; %s", w.Code, code, w.Body.String())
	}
}
func TestOwnerAuthenticationProtectsEveryOperation(t *testing.T) {
	f := newFixture(t)
	for _, op := range [][2]string{{"GET", ""}, {"GET", "/status"}, {"PUT", "/connection"}, {"POST", "/actions"}, {"PUT", "/permissions/operator"}, {"PUT", "/permissions/node"}, {"GET", "/slots/1"}, {"PUT", "/slots/1"}, {"GET", "/choices/agents?slot=1"}, {"POST", "/preview"}, {"GET", "/history"}, {"DELETE", "/history/0"}, {"PUT", "/wake/1"}} {
		w := f.request(t, op[0], op[1], map[string]any{}, false)
		requireCode(t, w, 401)
	}
	if f.changed != 0 || f.previews != 0 || len(f.calls.methods) != 0 {
		t.Fatal("unauthorized side effect")
	}
	if entries, _ := os.ReadDir(f.dir); len(entries) != 0 {
		t.Fatal("unauthorized persisted state")
	}
}
func TestConnectionWriteOnlyKeepReplaceClearAndRestart(t *testing.T) {
	f := newFixture(t)
	body := map[string]any{"url": "wss://gateway.invalid", "uiName": "UI", "nodeName": "Node", "token": "SYNTHETIC_SECRET_ONE"}
	w := f.request(t, "PUT", "/connection", body, true)
	requireCode(t, w, 200)
	for _, path := range []string{"", "/slots/1"} {
		w = f.request(t, "GET", path, nil, true)
		requireCode(t, w, 200)
		if strings.Contains(w.Body.String(), "SYNTHETIC_SECRET") {
			t.Fatal("secret exposed")
		}
	}
	delete(body, "token")
	body["uiName"] = "Changed"
	w = f.request(t, "PUT", "/connection", body, true)
	requireCode(t, w, 200)
	if f.a.o.Connections.Editable().Token != "SYNTHETIC_SECRET_ONE" {
		t.Fatal("omitted credential lost")
	}
	body["token"] = "SYNTHETIC_SECRET_TWO"
	requireCode(t, f.request(t, "PUT", "/connection", body, true), 200)
	ui := openclaw.New(filepath.Join(f.dir, "ui.json"), "test")
	node := openclaw.NewNode(filepath.Join(f.dir, "node.json"), "test", openclaw.NodeMetadata{})
	again := openclaw.NewConnections(filepath.Join(f.dir, "connection.json"), ui, node, "UI", "Node")
	if again.Editable().Token != "SYNTHETIC_SECRET_TWO" || again.Editable().UIName != "Changed" {
		t.Fatal("restart lost connection")
	}
	body["token"] = ""
	requireCode(t, f.request(t, "PUT", "/connection", body, true), 200)
	if f.a.o.Connections.Editable().Token != "" {
		t.Fatal("explicit clear failed")
	}
	body["url"] = "http://invalid"
	requireCode(t, f.request(t, "PUT", "/connection", body, true), 422)
	if f.a.o.Connections.Editable().URL != "wss://gateway.invalid" {
		t.Fatal("invalid save changed URL")
	}
	for _, name := range []string{"connection.json", "ui.json", "node.json"} {
		st, e := os.Stat(filepath.Join(f.dir, name))
		if e != nil || st.Mode().Perm() != 0600 {
			t.Fatal("private permissions", name)
		}
	}
}
func TestPermissionsPersistWithoutNodeSelfAdministration(t *testing.T) {
	f := newFixture(t)
	requireCode(t, f.request(t, "PUT", "/permissions/operator", map[string]any{"scopes": []string{"operator.read", "operator.talk"}}, true), 200)
	requireCode(t, f.request(t, "PUT", "/permissions/node", map[string]any{"grants": map[string]bool{"settings": false, "audio": true}}, true), 200)
	again := openclaw.New(filepath.Join(f.dir, "ui.json"), "test")
	if !reflect.DeepEqual(again.RequestedOperatorScopes(), []string{"operator.read", "operator.talk"}) {
		t.Fatal("scopes lost")
	}
	access := openclaw.NewNodeAccess(filepath.Join(f.dir, "permissions.json"))
	if access.Granted("settings") || !access.Granted("audio") {
		t.Fatal("grants lost")
	}
	requireCode(t, f.request(t, "PUT", "/permissions/operator", map[string]any{"scopes": []string{"not-a-scope"}}, true), 422)
	requireCode(t, f.request(t, "PUT", "/permissions/node", map[string]any{"grants": map[string]bool{"not-a-group": true}}, true), 422)
	for _, name := range access.Commands() {
		if strings.Contains(name, "openclawconfig") || strings.Contains(name, "permissions") {
			t.Fatal("owner API leaked into Node")
		}
	}
}
func TestSlotRoundTripNativeAndOwnerShareStore(t *testing.T) {
	f := newFixture(t)
	// Seed via exactly the native persistence path; expose those same values.
	if err := f.a.o.Store.Update(0, func(s *clawtalk.Slot) { s.OpenClaw = true; s.Agent = "test-agent"; s.Resume = true; s.Timeout = 30 }); err != nil {
		t.Fatal(err)
	}
	next := f.a.o.Store.Get(0)
	next.Context = "agent:test-agent:existing"
	next.Provider = "openai"
	next.Model = "native-test"
	next.GreetingPrompt = "Hello"
	next.DeviceContextTemplate = "Use {{node_id}}"
	next.ProgressTone = "none"
	next.StartTone = "ding"
	next.EndTone = "none"
	next.Timeout = 120
	next.Ending.Phrase = "Finish"
	next.Ending.Position = "end"
	next.Ending.Timing = "immediate"
	requireCode(t, f.request(t, "PUT", "/slots/1", next, true), 200)
	saved := clawtalk.NewStore(filepath.Join(f.dir, "slots.json")).Get(0)
	if saved.Context != next.Context || saved.GreetingPrompt != "Hello" || saved.Model != "native-test" || saved.Ending != next.Ending || saved.ProgressTone != "none" || saved.Timeout != 120 {
		t.Fatal("profile changed or lost")
	}
	if f.a.o.Store.Get(1).OpenClaw {
		t.Fatal("sibling slot changed")
	}
	if err := f.a.o.Store.Update(0, func(s *clawtalk.Slot) { s.Timeout = 300 }); err != nil {
		t.Fatal(err)
	}
	w := f.request(t, "GET", "/slots/1", nil, true)
	requireCode(t, w, 200)
	if !strings.Contains(w.Body.String(), `"timeout":300`) {
		t.Fatal("native edit not reflected")
	}
}
func TestSlotInvalidInputsAreAtomicAndDoNotElevate(t *testing.T) {
	f := newFixture(t)
	old := f.a.o.Store.Get(0)
	for _, change := range []func(*clawtalk.Slot){
		func(s *clawtalk.Slot) { s.Timeout = 901 },
		func(s *clawtalk.Slot) { s.Agent = "bad:agent" },
		func(s *clawtalk.Slot) { s.DeviceContextTemplate = "{{unknown}}" },
		func(s *clawtalk.Slot) { s.GreetingPrompt = strings.Repeat("x", 2000) },
		func(s *clawtalk.Slot) { s.Ending.Phrase = "\n" },
		func(s *clawtalk.Slot) { s.Ending.Position = "unknown" },
		func(s *clawtalk.Slot) { s.Ending.Timing = "unknown" },
		func(s *clawtalk.Slot) { s.Agent = "test-agent"; s.Context = "agent:other:secret" },
		func(s *clawtalk.Slot) { s.Provider = "openai"; s.Model = "unadvertised" },
		func(s *clawtalk.Slot) { s.StartTone = "unregistered-tone" },
	} {
		next := old
		change(&next)
		w := f.request(t, "PUT", "/slots/1", next, true)
		requireCode(t, w, 422)
		if f.a.o.Store.Get(0) != old {
			t.Fatal("partial persistence")
		}
		if f.a.o.Connections.UI.RequestedOperatorScopes() == nil {
			t.Fatal("authority changed")
		}
	}
	requireCode(t, f.request(t, "PUT", "/slots/3", old, true), 404)
	requireCode(t, f.request(t, "PUT", "/slots/0", old, true), 404)
}
func TestDynamicChoicesAreSharedFilteredAndErrorsRedacted(t *testing.T) {
	f := newFixture(t)
	for _, kind := range []string{"agents", "models", "sessions"} {
		w := f.request(t, "GET", "/choices/"+kind+"?slot=1&agent=test-agent", nil, true)
		requireCode(t, w, 200)
		if strings.Contains(w.Body.String(), "agent:other:") {
			t.Fatal("wrong-agent session")
		}
	}
	f.calls.fail = true
	w := f.request(t, "GET", "/choices/agents?slot=1", nil, true)
	requireCode(t, w, 503)
	if strings.Contains(w.Body.String(), "PRIVATE_") {
		t.Fatal("raw gateway error")
	}
	// An unrelated edit is still possible offline.
	old := f.a.o.Store.Get(0)
	old.Timeout = 120
	requireCode(t, f.request(t, "PUT", "/slots/1", old, true), 200)
}
func TestDestructiveActionsRequireExplicitConfirmation(t *testing.T) {
	f := newFixture(t)
	requireCode(t, f.request(t, "POST", "/actions", map[string]any{"action": "forget", "role": "ui", "confirm": true}, true), 400)
	for _, action := range []string{"forget", "pair_again"} {
		requireCode(t, f.request(t, "POST", "/actions", map[string]any{"action": action}, true), 422)
	}
	requireCode(t, f.request(t, "DELETE", "/history/0", map[string]any{}, true), 422)
	requireCode(t, f.request(t, "POST", "/actions", map[string]any{"action": "forget", "confirm": true}, true), 200)
	requireCode(t, f.request(t, "POST", "/actions", map[string]any{"action": "disconnect"}, true), 200)
	requireCode(t, f.request(t, "POST", "/actions", map[string]any{"action": "clear_events"}, true), 200)
}
func TestStrictBodiesPromptPreviewAndLocales(t *testing.T) {
	f := newFixture(t)
	requireCode(t, f.request(t, "POST", "/preview", map[string]any{"cue": "context", "template": "Use {{node_id}}"}, true), 200)
	requireCode(t, f.request(t, "POST", "/preview", map[string]any{"cue": "context", "template": "{{unknown}}"}, true), 422)
	requireCode(t, f.request(t, "POST", "/preview", map[string]any{"cue": "ready", "choice": "none"}, true), 204)
	requireCode(t, f.request(t, "POST", "/actions", map[string]any{"action": "connect", "unexpected": true}, true), 400)
	for _, body := range []string{`{"action":"disconnect"}{}`, strings.Repeat("x", 65537)} {
		r := httptest.NewRequest("POST", "https://device.invalid/api/v1/openclaw/actions", strings.NewReader(body))
		r.SetBasicAuth("owner", "SYNTHETIC_OWNER_PASSWORD")
		w := httptest.NewRecorder()
		f.h.ServeHTTP(w, r)
		requireCode(t, w, 400)
	}
	old := say.Chosen()
	defer say.Use(old)
	for _, lang := range []string{"en", "de", "it", "vi"} {
		say.Use(lang)
		l := Labels()
		for k, v := range l {
			if v == "" {
				t.Fatal("missing translation", lang, k)
			}
		}
	}
}

func TestInvalidProfileCannotEnableTalkOrChangeAuthority(t *testing.T) {
	f := newFixture(t)
	if err := f.a.o.Connections.UI.SetOperatorScopes([]string{}); err != nil {
		t.Fatal(err)
	}
	next := f.a.o.Store.Get(0)
	next.OpenClaw = true
	next.GreetingPrompt = strings.Repeat("x", 2000)
	requireCode(t, f.request(t, "PUT", "/slots/1", next, true), 422)
	if len(f.a.o.Connections.UI.RequestedOperatorScopes()) != 0 || f.a.o.Store.Get(0).OpenClaw {
		t.Fatal("failed validation changed authority or routing")
	}
}
func TestWakeSelectionUsesInjectedSharedCatalogAndWriter(t *testing.T) {
	f := newFixture(t)
	ids := []string{"", ""}
	saves := 0
	f.a.o.Store.Update(1, func(s *clawtalk.Slot) { s.OpenClaw = true })
	f.a.o.WakeChoices = func() []clawtalk.Choice {
		return []clawtalk.Choice{{Value: "", Label: "Off"}, {Value: "test-wake", Label: "Test wake"}}
	}
	f.a.o.WakeIDs = func() []string { return append([]string{}, ids...) }
	f.a.o.SaveWake = func(n int, value string) error { ids[n] = value; saves++; return nil }
	requireCode(t, f.request(t, "PUT", "/wake/2", map[string]any{"value": "test-wake"}, true), 200)
	if saves != 1 || ids[0] != "" || ids[1] != "test-wake" {
		t.Fatal("wrong wake slot")
	}
	requireCode(t, f.request(t, "PUT", "/wake/2", map[string]any{"value": "unknown"}, true), 422)
	if saves != 1 {
		t.Fatal("invalid wake persisted")
	}
	f.a.o.Busy = func() bool { return true }
	requireCode(t, f.request(t, "POST", "/preview", map[string]any{"cue": "ready", "choice": "none"}, true), 409)
	if f.previews != 0 {
		t.Fatal("preview preempted active Talk")
	}
}
