package chatapp

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeRPC struct {
	mu      sync.Mutex
	sendIDs []string
	fail    bool
	gate    chan struct{}
	calls   []string
}

func (f *fakeRPC) Updates() (<-chan struct{}, func(), error) {
	return make(chan struct{}), func() {}, nil
}
func (f *fakeRPC) Call(ctx context.Context, method string, params, result any) error {
	f.mu.Lock()
	f.calls = append(f.calls, method)
	gate := f.gate
	fail := f.fail
	if method == "chat.send" {
		p := params.(map[string]any)
		if p["sessionKey"] != "agent:test:a" || p["sessionId"] != "existing-id" {
			f.mu.Unlock()
			return errors.New("wrong_destination")
		}
		f.sendIDs = append(f.sendIDs, p["idempotencyKey"].(string))
	}
	f.mu.Unlock()
	if gate != nil {
		<-gate
	}
	if fail {
		return errors.New("rpc_delivery_uncertain")
	}
	var payload string
	switch method {
	case "sessions.list":
		payload = `{"sessions":[{"key":"agent:test:a","sessionId":"existing-id","category":"Project"}]}`
	case "sessions.groups.list":
		payload = `{"groups":[{"name":"Project","position":0}]}`
	case "chat.history":
		payload = `{"sessionId":"existing-id","messages":[{"role":"assistant","content":[{"type":"text","text":"**Hello**"}]}],"olderCursor":"older"}`
	default:
		payload = `{}`
	}
	if result != nil {
		return json.Unmarshal([]byte(payload), result)
	}
	return nil
}
func fixture(f *fakeRPC) *App {
	a := New(func() (RPC, error) { return f, nil }, func() {}, func(Session) (uint64, error) { return 1, nil }, func(uint64) bool { return true }, func() bool { return false })
	a.ctx = context.Background()
	a.rpc = f
	a.state.Connected = true
	a.state.Selected = Session{Key: "agent:test:a", SessionID: "existing-id"}
	a.state.Sessions = []Session{a.state.Selected, {Key: "agent:test:b", SessionID: "other"}}
	return a
}
func wait(t *testing.T, fn func() bool) {
	t.Helper()
	end := time.Now().Add(2 * time.Second)
	for time.Now().Before(end) {
		if fn() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timeout")
}
func TestUncertainDeliveryRetainsDestinationDraftAndIdempotency(t *testing.T) {
	f := &fakeRPC{fail: true}
	a := fixture(f)
	a.Draft("Hello")
	a.Send()
	wait(t, func() bool { return !a.Snapshot().Busy })
	if s := a.Snapshot(); s.Notice != "delivery_uncertain" || s.Draft != "Hello" {
		t.Fatal(s.Notice, s.Draft)
	}
	f.mu.Lock()
	f.fail = false
	f.mu.Unlock()
	a.Send()
	wait(t, func() bool { return !a.Snapshot().Busy })
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.sendIDs) != 2 || f.sendIDs[0] != f.sendIDs[1] {
		t.Fatal("retry duplicated identity")
	}
	if a.Snapshot().Draft != "" {
		t.Fatal("accepted draft retained")
	}
}
func TestSwitchBlockedWhileSendInFlightAndLateAckCannotClearNextDraft(t *testing.T) {
	f := &fakeRPC{gate: make(chan struct{})}
	a := fixture(f)
	a.Draft("first")
	a.Send()
	a.Select("agent:test:b")
	if a.Snapshot().Selected.Key != "agent:test:a" {
		t.Fatal("switch allowed during send")
	}
	a.Close()
	a.mu.Lock()
	a.state.Draft = "new view draft"
	a.mu.Unlock()
	close(f.gate)
	wait(t, func() bool { f.mu.Lock(); defer f.mu.Unlock(); return len(f.sendIDs) == 1 })
	time.Sleep(10 * time.Millisecond)
	if a.Snapshot().Draft != "new view draft" {
		t.Fatal("late acknowledgment mutated closed view")
	}
}
func TestTalkAndStoppingFenceSelection(t *testing.T) {
	a := fixture(&fakeRPC{})
	a.Talk()
	a.Select("agent:test:b")
	if a.Snapshot().Selected.Key != "agent:test:a" {
		t.Fatal("speech retargeted")
	}
	a.Talk()
	s := a.Snapshot()
	if !s.Talk || !s.Stopping {
		t.Fatal("stop did not retain ownership fence")
	}
	a.Select("agent:test:b")
	if a.Snapshot().Selected.Key != "agent:test:a" {
		t.Fatal("switch allowed before audio teardown")
	}
}
func TestGroupsHistoryAndSessionReset(t *testing.T) {
	f := &fakeRPC{}
	a := fixture(f)
	a.reload(context.Background(), 0, f)
	s := a.Snapshot()
	if len(s.Groups) != 1 || s.Groups[0] != "Project" || len(s.Messages) != 1 || s.Messages[0].Text() != "**Hello**" {
		t.Fatal("wire contract mismatch")
	}
	a.mu.Lock()
	a.state.Selected.SessionID = "replaced"
	a.mu.Unlock()
	a.reload(context.Background(), 0, f)
	if a.Snapshot().Connected || a.Snapshot().Notice != "session_changed" {
		t.Fatal("reset session silently reused")
	}
}
func TestDraftsStayWithTheirSession(t *testing.T) {
	a := fixture(&fakeRPC{})
	a.Draft("A")
	a.Select("agent:test:b")
	if a.Snapshot().Draft != "" {
		t.Fatal("draft leaked")
	}
	a.Draft("B")
	a.Select("agent:test:a")
	if a.Snapshot().Draft != "A" {
		t.Fatal("draft lost")
	}
}
func TestOnlyTextContentIsRendered(t *testing.T) {
	m := Message{Content: json.RawMessage(`[{"type":"thinking","text":"private reasoning"},{"type":"text","text":"visible"},{"type":"toolCall","text":"private args"}]`)}
	if m.Text() != "visible" {
		t.Fatal("non-text block rendered")
	}
}

func TestRetiredKeyboardAndSendCallbacksCannotTargetNewSelection(t *testing.T) {
	a := fixture(&fakeRPC{})
	a.Select("agent:test:b")
	a.Draft("B")
	a.Draft("old keyboard edit", 0)
	a.Send(0)
	a.Talk(0)
	a.SetModel("other", 0)
	s := a.Snapshot()
	if s.Draft != "B" || s.Busy || s.Talk {
		t.Fatal("stale UI callback changed the new conversation")
	}
}
