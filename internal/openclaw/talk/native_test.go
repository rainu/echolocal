package talk

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type nativeDevice struct {
	fakeDevice
	stops atomic.Int32
}

func (d *nativeDevice) StopOutput() { d.stops.Add(1) }

// Regression sentinel: no semantic playback gate may be used by Live Talk.
func (d *nativeDevice) BlockOutput(bool) { panic("live output must not be gated") }
func eventually(t *testing.T, f func() bool) {
	t.Helper()
	until := time.Now().Add(time.Second)
	for time.Now().Before(until) {
		if f() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("native state not reached")
}
func emit(s *Session, event string) { s.native.inbox <- []byte(event) }
func nativeHarness(t *testing.T) (*Session, *nativeDevice, context.Context) {
	d := &nativeDevice{fakeDevice: fakeDevice{frames: make(chan []int16, 8)}}
	s := NewSession(nil, "agent:synthetic:test", d)
	s.native.legacy.Store(true)
	ctx, cancel := context.WithCancel(context.Background())
	go s.nativeLoop(ctx)
	t.Cleanup(func() { cancel(); <-s.native.done })
	return s, d, ctx
}
func TestLiveControlReadyDoesNotNeedPolicyInjection(t *testing.T) {
	s, _, ctx := nativeHarness(t)
	// There is intentionally no peer or control sender. Open must neither send
	// replacement instructions nor wait for an append acknowledgement.
	hooks := s.controlLink(ctx)
	hooks.Opened()
	hooks.Opened()
	select {
	case <-s.native.channelReady:
	default:
		t.Fatal("extra policy roundtrip required")
	}
}
func TestLiveDirectFollowupsAreNeverSuppressedOrOmitted(t *testing.T) {
	s, _, _ := nativeHarness(t)
	for _, e := range []string{
		`{"type":"turn.done","turn":{"id":"q1","role":"user","transcript":"synthetic question"}}`,
		`{"type":"turn.done","turn":{"id":"a1","role":"assistant","transcript":"synthetic direct answer"}}`,
		`{"type":"turn.done","turn":{"id":"q2","role":"user","transcript":"synthetic followup"}}`,
		`{"type":"turn.done","turn":{"id":"a2","role":"assistant","transcript":"synthetic remembered answer"}}`,
		`{"type":"turn.done","turn":{"id":"q3","role":"user","transcript":"synthetic followup two"}}`,
		`{"type":"turn.done","turn":{"id":"a3","role":"assistant","transcript":"synthetic live response"}}`,
	} {
		emit(s, e)
	}
	eventually(t, func() bool { return len(s.native.transcripts) == 6 })
	if s.native.responses.Load() != 3 || s.native.delegations.Load() != 0 {
		t.Fatal("direct live conversation changed")
	}
	for i := 0; i < 6; i++ {
		item := <-s.native.transcripts
		want := "user"
		if i%2 == 1 {
			want = "assistant"
		}
		if item.role != want {
			t.Fatal("caption order changed")
		}
	}
}
func TestLiveConsultationKeepsAcknowledgementsAndChatter(t *testing.T) {
	s, d, _ := nativeHarness(t)
	emit(s, `{"type":"delegation.created","item":{"id":"work","target":"client"}}`)
	emit(s, `{"type":"turn.done","turn":{"id":"ack","role":"assistant","transcript":"synthetic acknowledgement"}}`)
	eventually(t, func() bool { return len(s.native.transcripts) == 1 })
	if s.pendingTools.Load() != 1 || d.stops.Load() != 0 {
		t.Fatal("consultation gated audio")
	}
	emit(s, `{"type":"delegation.context.appended","delegation_item_id":"unrelated"}`)
	emit(s, `{"type":"delegation.context.appended","delegation_item_id":"work"}`)
	eventually(t, func() bool { return s.native.results.Load() == 1 })
	if s.pendingTools.Load() != 0 {
		t.Fatal("native work tracking failed")
	}
}
func TestLiveInterruptionClearsOnlyOldQueueAndKeepsFutureReplies(t *testing.T) {
	s, d, _ := nativeHarness(t)
	emit(s, `{"type":"turn.created","turn":{"role":"user"}}`)
	emit(s, `{"type":"turn.done","turn":{"id":"q","role":"user","transcript":"synthetic interruption"}}`)
	emit(s, `{"type":"turn.done","turn":{"id":"a","role":"assistant","transcript":"synthetic response after interruption"}}`)
	eventually(t, func() bool { return len(s.native.transcripts) == 2 })
	if d.stops.Load() != 1 || s.native.responses.Load() != 1 {
		t.Fatal("interruption blocked future live response")
	}
}
func TestFirstPCMMetricIsOncePerFinalUtterance(t *testing.T) {
	s, _, _ := nativeHarness(t)
	s.native.responseAt.Store(time.Now().Add(-25 * time.Millisecond).UnixNano())
	s.Playback()
	s.Playback()
	if s.native.timedResponses.Load() != 1 || s.native.firstPCMMS.Load() < 25 {
		t.Fatal("first PCM timing incorrect")
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
func TestStartupSendsOnlySilenceUntilTransportAndAudioReady(t *testing.T) {
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

func TestNativeFinalCaptionReachesDisplayBeforeEnd(t *testing.T) {
	s, _, _ := nativeHarness(t)
	s.Ending = EndSettings{Phrase: "Unterhaltung beenden", Position: "start", Timing: "turn_end"}
	var mu sync.Mutex
	roles := []string{}
	s.Caption = func(role, text, id string) { mu.Lock(); roles = append(roles, role); mu.Unlock() }
	ended := make(chan struct{})
	s.End = func() { close(ended) }
	emit(s, `{"type":"turn.done","turn":{"id":"a","role":"assistant","transcript":"synthetic answer"}}`)
	emit(s, `{"type":"turn.done","turn":{"id":"u","role":"user","transcript":"Unterhaltung beenden, bitte"}}`)
	select {
	case <-ended:
	case <-time.After(time.Second):
		t.Fatal("punctuated end command did not terminate native session")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(roles) != 2 || roles[0] != "assistant" || roles[1] != "user" {
		t.Fatal("display missed final captions")
	}
}

func TestNativeRejectionIsDiagnosedWithoutProviderPayload(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, nil)))
	defer slog.SetDefault(previous)
	s, _, _ := nativeHarness(t)
	emit(s, `{"type":"error","error":{"type":"invalid_request_error","code":"secret-code-sentinel","param":"secret-param-sentinel","message":"Invalid content secret-message-sentinel"}}`)
	<-s.native.done
	if s.Diagnostics().Reason != "talk_control_rejected" {
		t.Fatal("rejection reason lost")
	}
	for _, secret := range []string{"secret-code-sentinel", "secret-param-sentinel", "secret-message-sentinel"} {
		if strings.Contains(output.String(), secret) {
			t.Fatal("provider payload leaked")
		}
	}
	if !strings.Contains(output.String(), "mentions_content=true") {
		t.Fatal("missing safe classification")
	}
}

func TestDeviceContextWaitsForActualUserAndIsSentOnce(t *testing.T) {
	s, _, ctx := nativeHarness(t)
	// Direct first-speech hook avoids transport dependence in this test.
	var sent atomic.Int32
	s.native.deviceContext = deviceContextAppend("gpt-live-1-codex", "synthetic device metadata")
	s.native.sendDeviceContext = func(event map[string]any) error { sent.Add(1); return nil }
	s.controlLink(ctx).Opened()
	emit(s, `{"type":"session.context.appended"}`)
	emit(s, `{"type":"turn.done","turn":{"role":"assistant","id":"old","transcript":"Goodbye"}}`)
	eventually(t, func() bool { return s.native.responses.Load() == 1 })
	if sent.Load() != 0 {
		t.Fatal("startup/history triggered device context")
	}
	emit(s, `{"type":"turn.created","turn":{"role":"user"}}`)
	eventually(t, func() bool { return sent.Load() == 1 })
	emit(s, `{"type":"turn.done","turn":{"role":"user","id":"first","transcript":"synthetic request"}}`)
	emit(s, `{"type":"turn.created","turn":{"role":"user"}}`)
	eventually(t, func() bool { return s.utterances.Load() == 1 })
	if sent.Load() != 1 {
		t.Fatal("context sent again on followup")
	}
}

func TestPublicDeviceContextStartsOnInputNotAssistant(t *testing.T) {
	s, _, _ := nativeHarness(t)
	s.native.legacy.Store(false)
	var sent atomic.Int32
	s.native.deviceContext = deviceContextAppend("gpt-live-1", "synthetic")
	s.native.sendDeviceContext = func(map[string]any) error { sent.Add(1); return nil }
	emit(s, `{"type":"session.output_transcript.delta"}`)
	emit(s, `{"type":"session.input_transcript.delta"}`)
	emit(s, `{"type":"session.input_transcript.delta"}`)
	eventually(t, func() bool { return sent.Load() == 1 })
	if sent.Load() != 1 {
		t.Fatal("public context must be sent once on input")
	}
}
func TestDeviceContextSendFailureRetainsExplicitReason(t *testing.T) {
	s, _, _ := nativeHarness(t)
	s.native.deviceContext = deviceContextAppend("gpt-live-1-codex", "synthetic")
	s.native.sendDeviceContext = func(map[string]any) error { return context.Canceled }
	emit(s, `{"type":"turn.created","turn":{"role":"user"}}`)
	select {
	case <-s.failed:
	case <-time.After(time.Second):
		t.Fatal("context failure ignored")
	}
	if s.Diagnostics().Reason != "talk_device_context_unavailable" {
		t.Fatal("context failure reason lost")
	}
}
