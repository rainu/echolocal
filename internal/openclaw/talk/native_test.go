package talk

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type nativeDevice struct {
	fakeDevice
	blocked atomic.Bool
}

func (d *nativeDevice) BlockOutput(block bool) { d.blocked.Store(block) }
func eventually(t *testing.T, f func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if f() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("expected native state not reached")
}
func emit(s *Session, event string) { s.native.inbox <- []byte(event) }
func nativeHarness(t *testing.T) (*Session, *nativeDevice, context.Context) {
	t.Helper()
	d := &nativeDevice{fakeDevice: fakeDevice{frames: make(chan []int16, 8)}}
	s := NewSession(nil, "agent:synthetic:test", d)
	s.native.legacy = true
	ctx, cancel := context.WithCancel(context.Background())
	go s.nativeLoop(ctx)
	t.Cleanup(func() { cancel(); <-s.native.done })
	return s, d, ctx
}
func TestNativePolicyMatchesImmutableLiveModelProtocol(t *testing.T) {
	for _, model := range []string{"gpt-live-1-codex", "gpt-live-1"} {
		legacy, err := nativeModel(model)
		if err != nil {
			t.Fatal(err)
		}
		n := newNative()
		n.legacy = legacy
		raw, _ := json.Marshal(n.policyEvent())
		var e map[string]any
		json.Unmarshal(raw, &e)
		want := "session.instructions.append"
		if legacy {
			want = "session.context.append"
		}
		if e["type"] != want {
			t.Fatal("used generic Realtime event on native Live model")
		}
	}
	if _, err := nativeModel("unknown"); err == nil {
		t.Fatal("unsupported protocol accepted")
	}
}
func TestNativePolicyAckAndMatchedAgentResultGate(t *testing.T) {
	s, d, _ := nativeHarness(t)
	d.BlockOutput(true)
	emit(s, `{"type":"session.context.appended"}`)
	select {
	case <-s.native.policyReady:
	case <-time.After(time.Second):
		t.Fatal("routing policy not acknowledged")
	}
	emit(s, `{"type":"delegation.created","item":{"id":"request-a","target":"client"}}`)
	eventually(t, func() bool { return s.native.delegations.Load() == 1 })
	if !d.blocked.Load() {
		t.Fatal("output opened before agent reply")
	}
	emit(s, `{"type":"delegation.context.appended","delegation_item_id":"unrelated"}`)
	emit(s, `{"type":"delegation.created","item":{"id":"request-b","target":"client"}}`)
	emit(s, `{"type":"delegation.context.appended","delegation_item_id":"request-a"}`)
	eventually(t, func() bool { return s.native.results.Load() == 1 })
	if !d.blocked.Load() {
		t.Fatal("old result opened newer response")
	}
	emit(s, `{"type":"delegation.context.appended","delegation_item_id":"request-b"}`)
	eventually(t, func() bool { return !d.blocked.Load() })
	if s.pendingTools.Load() != 0 || s.native.results.Load() != 2 {
		t.Fatal("native result tracking failed")
	}
	// Barge-in blocks output immediately, before final transcription.
	emit(s, `{"type":"turn.created","turn":{"role":"user"}}`)
	eventually(t, func() bool { return d.blocked.Load() })
}
func TestNativeRejectsUnbackedAssistantAndKeepsUserCaption(t *testing.T) {
	s, d, _ := nativeHarness(t)
	d.BlockOutput(true)
	emit(s, `{"type":"turn.done","turn":{"id":"user-a","role":"user","transcript":"synthetic question"}}`)
	emit(s, `{"type":"turn.done","turn":{"id":"reply-a","role":"assistant","transcript":"synthetic unbacked answer"}}`)
	eventually(t, func() bool { return s.native.unbacked.Load() == 1 })
	if !d.blocked.Load() || s.native.unbacked.Load() != 1 {
		t.Fatal("generic answer was not suppressed")
	}
	select {
	case item := <-s.native.transcripts:
		if item.role != "user" {
			t.Fatal("unbacked reply stored")
		}
	default:
		t.Fatal("user history missing")
	}
	if len(s.native.transcripts) != 0 {
		t.Fatal("unbacked reply queued")
	}
}

type transcriptControl struct {
	fakeControl
	mu    sync.Mutex
	items []map[string]any
}

func (c *transcriptControl) CallTalk(ctx context.Context, method string, params, result any) error {
	if method == "talk.client.transcript" {
		c.mu.Lock()
		c.items = append(c.items, params.(map[string]any))
		c.mu.Unlock()
	}
	return nil
}
func TestNativeCaptionQueueDrainsAndDeduplicates(t *testing.T) {
	c := &transcriptControl{}
	s := NewSession(c, "agent:synthetic:test", &fakeDevice{})
	s.native.workCtx, s.native.workCancel = context.WithCancel(context.Background())
	defer s.native.workCancel()
	s.appendTranscript(context.Background(), "user", "synthetic question", "one")
	s.appendTranscript(context.Background(), "user", "synthetic question", "one")
	s.appendTranscript(context.Background(), "assistant", "synthetic agent answer", "two")
	go s.transcriptLoop()
	close(s.native.transcriptStop)
	select {
	case <-s.native.transcriptDone:
	case <-time.After(time.Second):
		t.Fatal("history failed to drain")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.items) != 2 || c.items[0]["role"] != "user" || c.items[1]["role"] != "assistant" {
		t.Fatal("history ordering/deduplication failed")
	}
	if c.items[0]["sessionKey"] != s.Target || c.items[0]["voiceSessionId"] != s.id {
		t.Fatal("history escaped activation")
	}
}
func TestStartupSendsOnlySilenceUntilPolicyAndAudioReady(t *testing.T) {
	d := &fakeDevice{frames: make(chan []int16, 1)}
	d.frames <- []int16{321, 321}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gate := make(chan struct{})
	var count atomic.Uint64
	a := preReadyAudio{device: d, ctx: ctx, gate: gate, count: &count}
	frames, release := a.Frames()
	defer release()
	select {
	case frame := <-frames:
		for _, sample := range frame {
			if sample != 0 {
				t.Fatal("microphone escaped startup gate")
			}
		}
	case <-time.After(time.Second):
		t.Fatal("startup timeline stalled")
	}
	if d.started.Load() != 0 {
		t.Fatal("startup opened microphone")
	}
	close(gate)
	deadline := time.After(time.Second)
	for {
		select {
		case frame := <-frames:
			if len(frame) == 2 && frame[0] == 321 {
				return
			}
		case <-deadline:
			t.Fatal("ready audio not forwarded")
		}
	}
}

func TestBargeInCannotReleasePreviousDelegationAudio(t *testing.T) {
	s, d, _ := nativeHarness(t)
	emit(s, `{"type":"delegation.created","item":{"id":"old","target":"client"}}`)
	emit(s, `{"type":"turn.created","turn":{"role":"user"}}`)
	emit(s, `{"type":"delegation.context.appended","delegation_item_id":"old"}`)
	eventually(t, func() bool { return s.native.results.Load() == 1 })
	if !d.blocked.Load() {
		t.Fatal("interrupted response reopened audio")
	}
}
