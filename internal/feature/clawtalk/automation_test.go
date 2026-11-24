//go:build !board_biscuit

package clawtalk

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type automationRPC struct {
	pages []string
	calls []map[string]any
	err   error
}

func (f *automationRPC) CallTalk(ctx context.Context, method string, params, result any) error {
	if method != "sessions.list" {
		return errors.New("unexpected_method")
	}
	f.calls = append(f.calls, params.(map[string]any))
	if f.err != nil {
		return f.err
	}
	if len(f.pages) == 0 {
		return errors.New("missing_page")
	}
	raw := f.pages[0]
	f.pages = f.pages[1:]
	return json.Unmarshal([]byte(raw), result)
}
func TestAutomationSessionsPaginationAndFailure(t *testing.T) {
	f := &automationRPC{pages: []string{
		`{"sessions":[{"key":"agent:test:one","label":"First"},{"key":"agent:test:two","displayName":"Second"}],"hasMore":true,"nextOffset":2}`,
		`{"sessions":[{"key":"agent:test:one","label":"Duplicate"},{"key":"invalid","label":"Skip"},{"key":"agent:other:three","derivedTitle":"Third"},{"key":"agent:test:four"}]}`,
	}}
	rows, err := AutomationSessions(context.Background(), f)
	if err != nil || len(rows) != 4 || rows[0].ID != "agent:test:one" || rows[1].Name != "Second" || rows[2].Name != "Third" || rows[3].Name == "" || strings.Contains(rows[3].Name, "agent:") {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	if len(f.calls) != 2 || f.calls[1]["offset"] != 2 || f.calls[0]["includeDerivedTitles"] != true {
		t.Fatal(f.calls)
	}
	for _, f := range []*automationRPC{
		{err: errors.New("private_error")},
		{pages: []string{`{}`}},
		{pages: []string{`{"sessions":null}`}},
		{pages: []string{`{"sessions":[{"key":"agent:test:one"}],"hasMore":true,"nextOffset":1}`}, err: nil},
		{pages: []string{`{"sessions":[],"hasMore":true,"nextOffset":0}`}},
		{pages: []string{`{"sessions":[{"key":"agent:test:one"}],"hasMore":true,"nextOffset":0}`}},
		{pages: []string{`{"sessions":[{"key":"agent:test:one"}],"hasMore":true,"nextOffset":5000}`}},
	} {
		rows, err := AutomationSessions(context.Background(), f)
		if err == nil || rows != nil || strings.Contains(err.Error(), "private") {
			t.Fatalf("partial success: %+v %v", rows, err)
		}
	}
	rows, err = AutomationSessions(context.Background(), &automationRPC{pages: []string{`{"sessions":[]}`}})
	if err != nil || rows == nil || len(rows) != 0 {
		t.Fatal(rows, err)
	}
}
func TestAutomationOverridesIndependentAndNeverPersist(t *testing.T) {
	p := filepath.Join(t.TempDir(), "slots.json")
	s := load(p)
	if err := s.Update(0, func(v *Slot) {
		v.OpenClaw = true
		v.Agent = "test"
		v.Context = "agent:test:default"
		v.Resume = true
		v.GreetingPrompt = "Saved greeting"
		v.Provider = "openai"
		v.Model = "test-model"
	}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(p)
	original := s.Get(0)
	for _, tc := range []struct{ id, prompt, wantID, wantPrompt string }{
		{"", "", "agent:test:default", "Saved greeting"},
		{"agent:other:chosen", "", "agent:other:chosen", "Saved greeting"},
		{"", "One call only", "agent:test:default", "One call only"},
		{"agent:other:chosen", "One call only", "agent:other:chosen", "One call only"},
	} {
		v, err := automationTarget(s.Get(0), false, tc.id, tc.prompt)
		if err != nil || v.Context != tc.wantID || v.GreetingPrompt != tc.wantPrompt || v.Provider != original.Provider || v.Model != original.Model {
			t.Fatal(v, err)
		}
	}
	fresh := original
	fresh.Resume = false
	v, err := automationTarget(fresh, false, "", "Transient")
	if err != nil || v.Context == original.Context || !strings.HasPrefix(v.Context, "agent:test:echolocal-") {
		t.Fatal(v, err)
	}
	after, _ := os.ReadFile(p)
	if string(after) != string(before) || !reflect.DeepEqual(original, s.Get(0)) {
		t.Fatal("defaults persisted/changed")
	}
	for _, tc := range []struct{ id, prompt string }{
		{"bad", ""}, {"agent::missing", ""}, {"agent:bad agent:x", ""}, {"agent:test:", ""},
		{"", " "}, {"", strings.Repeat("x", 1537)},
	} {
		if _, err := automationTarget(original, false, tc.id, tc.prompt); err == nil {
			t.Fatal("invalid accepted", tc)
		}
	}
	if _, err := automationTarget(original, true, "agent:test:new", ""); err == nil {
		t.Fatal("corrupt store accepted")
	}
}
func TestAutomationBusyDoesNotTouchExistingConversation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := &Manager{active: true, generation: 17, cancel: cancel}
	for i := 0; i < 4; i++ {
		if err := m.StartAutomation(context.Background(), "invalid", "", false); err == nil || err.Error() != "talk_already_active" {
			t.Fatal(err)
		}
	}
	if !m.active || m.generation != 17 || ctx.Err() != nil {
		t.Fatal("active conversation changed")
	}
	m.active = false
	if err := m.StartAutomation(context.Background(), "", "", true); err == nil || err.Error() != "talk_assistant_busy" {
		t.Fatal(err)
	}
}
func TestAutomationStopWaitsForReleaseAndIsIdempotent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{active: true, generation: 5, cancel: cancel}
	go func() { <-ctx.Done(); m.mu.Lock(); m.active = false; m.cancel = nil; m.mu.Unlock() }()
	wait, done := context.WithTimeout(context.Background(), time.Second)
	defer done()
	if err := m.StopAutomation(wait); err != nil {
		t.Fatal(err)
	}
	if err := m.StopAutomation(wait); err != nil {
		t.Fatal(err)
	}
}
func TestAutomationStopCannotCancelReplacement(t *testing.T) {
	old, cancel := context.WithCancel(context.Background())
	newer, newCancel := context.WithCancel(context.Background())
	defer newCancel()
	m := &Manager{active: true, generation: 8, cancel: cancel}
	go func() { <-old.Done(); m.mu.Lock(); m.generation = 9; m.cancel = newCancel; m.mu.Unlock() }()
	wait, done := context.WithTimeout(context.Background(), time.Second)
	defer done()
	if err := m.StopAutomation(wait); err != nil || newer.Err() != nil {
		t.Fatal("new conversation stopped", err)
	}
}
func TestAutomationLaunchReportsSetupOnce(t *testing.T) {
	l := &automationLaunch{result: make(chan error, 1)}
	l.notify(nil)
	l.notify(errors.New("late_failure"))
	if err := <-l.result; err != nil {
		t.Fatal(err)
	}
	select {
	case <-l.result:
		t.Fatal("duplicate result")
	default:
	}
}
