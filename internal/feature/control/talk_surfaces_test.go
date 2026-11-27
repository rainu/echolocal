//go:build !board_biscuit

package control

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ygelfand/echolocal/internal/board"
	"github.com/ygelfand/echolocal/internal/feature/actions"
	"github.com/ygelfand/echolocal/internal/feature/clawtalk"
	connection "github.com/ygelfand/echolocal/internal/openclaw"
	esphome "github.com/ygelfand/go-esphome-device"
	"github.com/ygelfand/libcountertop/pkg/runtime/webapi"
	setting "github.com/ygelfand/libcountertop/pkg/settings"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

type talkSurfaceSettings struct{ value schema.Frontend }

func (s *talkSurfaceSettings) Read() schema.Frontend                  { return s.value }
func (s *talkSurfaceSettings) Update(fn func(*schema.Frontend)) error { fn(&s.value); return nil }

type talkSurfaceActions struct {
	all   []*esphome.Action
	calls int
}

func (*talkSurfaceActions) Listen(func(esphome.State)) func()   { return func() {} }
func (*talkSurfaceActions) States() []esphome.State             { return nil }
func (*talkSurfaceActions) Descriptions() []esphome.Description { return nil }
func (*talkSurfaceActions) Press(string) error                  { return errors.New("not supported") }
func (s *talkSurfaceActions) Actions() []esphome.Action {
	out := []esphome.Action{}
	for _, a := range s.all {
		out = append(out, *a)
	}
	return out
}
func (s *talkSurfaceActions) RunAction(name string, c esphome.Call) (any, error) {
	s.calls++
	for _, a := range s.all {
		if a.Name == name {
			return a.Run(c)
		}
	}
	return nil, errors.New("not found")
}
func TestTransferSurfaceValidationAndOwnerAuthentication(t *testing.T) {
	if !board.Detect().Has(board.Panel) {
		t.Skip("requires panel board pin")
	}
	all := actions.Get().Actions()
	var transfer *esphome.Action
	for _, a := range all {
		if a.Name == "openclaw_talk_transfer" {
			transfer = a
		}
	}
	if transfer == nil {
		t.Fatal("native action missing")
	}
	states := &talkSurfaceActions{all: all}
	owner := webapi.New(webapi.Options{Settings: &talkSurfaceSettings{schema.Frontend{Password: "synthetic-owner"}},
		Registry: &setting.Registry{}, Device: func() webapi.Device { return webapi.Device{Name: "synthetic"} }, States: states}).Handler()
	node := connection.NewNodeAccess(filepath.Join(t.TempDir(), "grants"))
	registerNodeTalk(node)
	for _, tc := range []struct{ id, want string }{
		{"", "talk_invalid_session"}, {"bad", "talk_invalid_session"}, {"agent::x", "talk_invalid_session"},
		{"agent:test:next", "talk_not_ready"},
	} {
		call, _ := esphome.NewCall(map[string]any{"session_id": tc.id})
		_, haErr := transfer.Run(call)
		if haErr == nil || haErr.Error() != tc.want {
			t.Fatal("native", haErr)
		}
		raw, _ := json.Marshal(map[string]string{"sessionId": tc.id})
		_, nodeErr := node.Execute(context.Background(), "echolocal.talk.transfer", raw)
		// Empty input is rejected by the Node schema before the common operation.
		if nodeErr == nil || (tc.id != "" && nodeErr.Error() != tc.want) {
			t.Fatal("node", nodeErr)
		}
		body, _ := json.Marshal(map[string]any{"args": map[string]string{"session_id": tc.id}})
		req := httptest.NewRequest("POST", "https://device.invalid/api/v1/actions/openclaw_talk_transfer", strings.NewReader(string(body)))
		req.SetBasicAuth("owner", "synthetic-owner")
		response := httptest.NewRecorder()
		owner.ServeHTTP(response, req)
		if response.Code != 422 || !strings.Contains(response.Body.String(), tc.want) {
			t.Fatal("HTTP", response.Code, response.Body.String())
		}
	}
	before := states.calls
	for _, operation := range []struct{ method, path, body string }{
		{"GET", "/actions", ""}, {"POST", "/actions/openclaw_talk_transfer", `{"args":{"session_id":"agent:test:next"}}`},
		{"POST", "/actions/openclaw_sessions", `{"args":{}}`}, {"POST", "/actions/openclaw_talk_status", `{"args":{}}`},
	} {
		req := httptest.NewRequest(operation.method, "https://device.invalid/api/v1"+operation.path, strings.NewReader(operation.body))
		response := httptest.NewRecorder()
		owner.ServeHTTP(response, req)
		if response.Code != 401 {
			t.Fatal("unauthorized", response.Code)
		}
	}
	if states.calls != before || clawtalk.Get().Busy() {
		t.Fatal("unauthorized side effect or rejected transfer changed active Talk")
	}
	if err := node.SetGrants(map[string]bool{"talk": false}); err != nil {
		t.Fatal(err)
	}
	if _, err := node.Execute(context.Background(), "echolocal.talk.transfer", []byte(`{"sessionId":"agent:test:next"}`)); err == nil || err.Error() != "permission_denied" {
		t.Fatal("Node grant bypass", err)
	}
}
func TestTalkStatusHasSameBoundedProjection(t *testing.T) {
	if !board.Detect().Has(board.Panel) {
		t.Skip("requires panel board pin")
	}
	node := connection.NewNodeAccess(filepath.Join(t.TempDir(), "grants"))
	registerNodeTalk(node)
	nodeStatus, err := node.Execute(context.Background(), "echolocal.talk.status", []byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	var haStatus any
	for _, a := range actions.Get().Actions() {
		if a.Name == "openclaw_talk_status" {
			call, _ := esphome.NewCall(nil)
			haStatus, err = a.Run(call)
		}
	}
	if err != nil || haStatus == nil {
		t.Fatal(err)
	}
	n, _ := json.Marshal(nodeStatus)
	h, _ := json.Marshal(haStatus)
	if string(n) != string(h) {
		t.Fatal("status diverged")
	}
	var fields map[string]any
	_ = json.Unmarshal(n, &fields)
	if len(fields) != 5 {
		t.Fatal("private status fields added")
	}
	for _, key := range []string{"active", "state", "transfer", "transferReason", "recovery"} {
		if _, ok := fields[key]; !ok {
			t.Fatal(key)
		}
	}
}

func TestTransferDescriptionOwnsAudibleConfirmation(t *testing.T) {
	description, _ := nodeContract("talk.transfer")
	for _, text := range []string{"do not speak an additional handoff confirmation", "repeat one after acceptance", "If rejected, explain the actual error"} {
		if !strings.Contains(description, text) {
			t.Fatalf("missing targeted speech guidance: %s", text)
		}
	}
}
