package talk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	connection "github.com/ygelfand/echolocal/internal/openclaw"
	"github.com/ygelfand/libcountertop/pkg/media/rtc"
	"sync/atomic"
	"testing"
	"time"
)

type recoveryFake struct {
	retryable bool
	fail, end func()
	open      func(context.Context) error
	close     func()
	closed    atomic.Int32
}

func (b *recoveryFake) Open(ctx context.Context) error {
	if b.open != nil {
		return b.open(ctx)
	}
	return nil
}
func (b *recoveryFake) Close() error {
	b.closed.Add(1)
	if b.close != nil {
		b.close()
	}
	return nil
}
func (b *recoveryFake) RecoverableFailure() bool { return b.retryable }

func awaitRecovery(t *testing.T, r *Recovery, state string, attempts int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		s := r.Status()
		if s.State == state && s.Attempts == attempts {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("wanted %s/%d got %+v", state, attempts, r.Status())
}
func TestRecoverySerializesRetirementAndFencesOldCallbacks(t *testing.T) {
	made := make(chan *recoveryFake, 8)
	var count atomic.Int32
	var previous *recoveryFake
	r := &Recovery{delays: []time.Duration{time.Millisecond, time.Millisecond}}
	r.Factory = func(ctx context.Context, retry bool, fail, end func()) (RecoveryBackend, error) {
		n := count.Add(1)
		if retry != (n > 1) {
			t.Error("wrong retry/greeting policy")
		}
		if previous != nil && previous.closed.Load() != 1 {
			t.Error("overlapping voice transports")
		}
		b := &recoveryFake{retryable: true, fail: fail, end: end}
		previous = b
		made <- b
		return b, nil
	}
	if err := r.Open(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	first := <-made
	first.fail()
	awaitRecovery(t, r, "connected", 1)
	second := <-made
	first.fail()
	first.end()
	time.Sleep(10 * time.Millisecond)
	if count.Load() != 2 || second.closed.Load() != 0 {
		t.Fatal("stale callbacks touched replacement")
	}
	second.fail()
	awaitRecovery(t, r, "connected", 2)
	third := <-made
	third.fail()
	awaitRecovery(t, r, "failed", 2)
	if count.Load() != 3 {
		t.Fatal("retry budget reset after recovery")
	}
}
func TestRecoveryStopDuringBackoffNeverReopens(t *testing.T) {
	made := make(chan *recoveryFake, 1)
	var count atomic.Int32
	r := &Recovery{delays: []time.Duration{time.Hour}}
	r.Factory = func(ctx context.Context, retry bool, fail, end func()) (RecoveryBackend, error) {
		count.Add(1)
		b := &recoveryFake{retryable: true, fail: fail}
		made <- b
		return b, nil
	}
	if err := r.Open(context.Background()); err != nil {
		t.Fatal(err)
	}
	(<-made).fail()
	awaitRecovery(t, r, "reconnecting", 1)
	r.Close()
	if count.Load() != 1 || r.Status().State != "stopped" {
		t.Fatal("stop restarted Talk")
	}
}
func TestRecoveryCancelsInFlightSetupAndClosesBeforeReturning(t *testing.T) {
	made := make(chan *recoveryFake, 2)
	entered := make(chan struct{})
	var count atomic.Int32
	r := &Recovery{delays: []time.Duration{0}}
	r.Factory = func(ctx context.Context, retry bool, fail, end func()) (RecoveryBackend, error) {
		b := &recoveryFake{retryable: true, fail: fail}
		if count.Add(1) > 1 {
			b.open = func(ctx context.Context) error { close(entered); <-ctx.Done(); return ctx.Err() }
		}
		made <- b
		return b, nil
	}
	if err := r.Open(context.Background()); err != nil {
		t.Fatal(err)
	}
	(<-made).fail()
	<-entered
	r.Close()
	b := <-made
	if b.closed.Load() != 1 {
		t.Fatal("setup leaked")
	}
}
func TestRecoveryDeliberateEndAndPermanentFailureAreFinal(t *testing.T) {
	for _, intentional := range []bool{false, true} {
		t.Run(map[bool]string{false: "permanent", true: "end"}[intentional], func(t *testing.T) {
			made := make(chan *recoveryFake, 1)
			ended, failed := make(chan struct{}, 1), make(chan struct{}, 1)
			r := &Recovery{Ended: func() { ended <- struct{}{} }, Failed: func() { failed <- struct{}{} }}
			r.Factory = func(ctx context.Context, retry bool, fail, end func()) (RecoveryBackend, error) {
				b := &recoveryFake{fail: fail, end: end}
				made <- b
				return b, nil
			}
			if err := r.Open(context.Background()); err != nil {
				t.Fatal(err)
			}
			b := <-made
			if intentional {
				b.end()
				<-ended
				awaitRecovery(t, r, "stopped", 0)
			} else {
				b.fail()
				<-failed
				awaitRecovery(t, r, "failed", 0)
			}
			r.Close()
		})
	}
}
func TestRecoverySetupFailuresConsumeFiniteBudget(t *testing.T) {
	made := make(chan *recoveryFake, 1)
	var count atomic.Int32
	r := &Recovery{delays: []time.Duration{0, 0, 0}}
	r.Factory = func(ctx context.Context, retry bool, fail, end func()) (RecoveryBackend, error) {
		count.Add(1)
		b := &recoveryFake{retryable: true, fail: fail}
		if retry {
			b.open = func(context.Context) error { return errors.New("offline") }
		} else {
			made <- b
		}
		return b, nil
	}
	if err := r.Open(context.Background()); err != nil {
		t.Fatal(err)
	}
	(<-made).fail()
	awaitRecovery(t, r, "failed", 3)
	r.Close()
	if count.Load() != 4 {
		t.Fatal("wrong setup retry budget")
	}
}
func TestRecoveryInitialSetupFailureDoesNotAutoReplay(t *testing.T) {
	var count atomic.Int32
	r := &Recovery{}
	r.Factory = func(context.Context, bool, func(), func()) (RecoveryBackend, error) {
		count.Add(1)
		return &recoveryFake{retryable: true, open: func(context.Context) error { return errors.New("setup") }}, nil
	}
	if err := r.Open(context.Background()); err == nil {
		t.Fatal("setup succeeded")
	}
	r.Close()
	if count.Load() != 1 {
		t.Fatal("initial activation replayed")
	}
}
func TestRecoveryIntentionalEndWinsBufferedFailureDuringOpen(t *testing.T) {
	r := &Recovery{Ended: func() {}}
	r.Factory = func(ctx context.Context, retry bool, fail, end func()) (RecoveryBackend, error) {
		return &recoveryFake{retryable: true, open: func(context.Context) error { fail(); end(); return nil }}, nil
	}
	if err := r.Open(context.Background()); err == nil {
		t.Fatal("already ended setup reported ready")
	}
	r.Close()
	if s := r.Status(); s.Attempts != 0 || s.State != "stopped" {
		t.Fatalf("intentional stop retried: %+v", s)
	}
}
func TestSessionRecoveryClassificationAndEndPhrase(t *testing.T) {
	for _, reason := range []string{"talk_peer_failed", "talk_control_closed", "talk_transport_closed", "control_stream_closed", "rpc_disconnected", "provider_input_unconfirmed"} {
		s := NewSession(nil, "agent:synthetic:session", nil)
		s.reason.Store(reason)
		if !s.RecoverableFailure() {
			t.Fatalf("transport not recoverable: %s", reason)
		}
		s.userTranscript("user", "end conversation", "", true, true)
		if s.RecoverableFailure() {
			t.Fatal("end phrase permits retry")
		}
	}
	for _, reason := range []string{"provider_session_replaced", "provider_session_error", "talk_audio_unavailable", "rpc_rejected", "talk_contract_invalid"} {
		s := NewSession(nil, "agent:synthetic:session", nil)
		s.reason.Store(reason)
		if s.RecoverableFailure() {
			t.Fatalf("permanent failure retried: %s", reason)
		}
	}
}

func TestRecoveryRealWebRTCReopensExactTargetAfterTransportClose(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	target := "agent:synthetic:transferred"
	sessions := make(chan *Session, 2)
	controls := make(chan *fakeControl, 2)
	peers := make(chan *rtc.Link, 2)
	r := &Recovery{delays: []time.Duration{time.Millisecond}}
	r.Factory = func(ctx context.Context, retry bool, fail, end func()) (RecoveryBackend, error) {
		peer, err := rtc.NewAudioOnly(rtc.Media{Audio: &fakeDevice{frames: make(chan []int16)}}, nil)
		if err != nil {
			return nil, err
		}
		c := &fakeControl{event: make(chan connection.TalkEvent, 8), peer: peer, capable: true}
		b := NewSession(c, target, &fakeDevice{frames: make(chan []int16)})
		b.nativeMedia = false
		b.Failure, b.End = fail, end
		sessions <- b
		controls <- c
		peers <- peer
		return b, nil
	}
	if err := r.Open(ctx); err != nil {
		t.Fatal(err)
	}
	first, c1 := <-sessions, <-controls
	if err := first.InterruptTransport(); err != nil {
		t.Fatal(err)
	}
	awaitRecovery(t, r, "connected", 1)
	second, c2 := <-sessions, <-controls
	if first.id == second.id || first.Target != target || second.Target != target ||
		c1.createParams["sessionKey"] != target || c2.createParams["sessionKey"] != target {
		t.Fatal("recovery changed session or reused transport identity")
	}
	r.Close()
	(<-peers).Close()
	(<-peers).Close()
}
func TestUnexpectedClosedEventsAreFailuresNotIntentionalEnds(t *testing.T) {
	for _, native := range []bool{false, true} {
		s := NewSession(nil, "agent:synthetic:kept", nil)
		failed := make(chan struct{}, 1)
		ended := make(chan struct{}, 1)
		s.Failure = func() { failed <- struct{}{} }
		s.End = func() { ended <- struct{}{} }
		ctx, cancel := context.WithCancel(context.Background())
		if native {
			go s.nativeLoop(ctx)
			s.native.inbox <- []byte("{\"type\":\"session.closed\"}")
		} else {
			events := make(chan connection.TalkEvent, 1)
			go s.control(ctx, events)
			var e connection.TalkEvent
			e.VoiceSessionID = s.id
			e.Event.Type = "session.closed"
			events <- e
		}
		select {
		case <-failed:
			if !s.RecoverableFailure() {
				t.Fatal("unexpected close is not recoverable")
			}
		case <-ended:
			t.Fatal("transport close treated as deliberate end")
		case <-time.After(time.Second):
			t.Fatal("close ignored")
		}
		cancel()
	}
}

func TestReplacementRetainsTransferredSessionConfigButNotGreetingOrCallbacks(t *testing.T) {
	c := &fakeControl{}
	s := NewSession(c, "agent:synthetic:handoff", nil)
	s.Provider, s.Model = "selected", "default"
	s.runtimeProvider, s.runtimeModel = "resolved", "resolved-model"
	s.Greeting = "Only speak this once"
	s.Ending = EndSettings{Phrase: "finish now", Position: "end", Timing: "immediate"}
	s.DeviceContext = "synthetic context"
	s.SetInputMuted(true)
	s.End = func() {}
	s.Failure = func() {}
	next := s.Replacement(nil)
	if next.id == s.id || next.Target != s.Target || next.Client != s.Client ||
		next.Provider != "resolved" || next.Model != "resolved-model" ||
		next.Ending != s.Ending || next.DeviceContext != s.DeviceContext || !next.inputMuted.Load() {
		t.Fatal("replacement lost per-Talk configuration")
	}
	if next.Greeting != "" || next.End != nil || next.Failure != nil {
		t.Fatal("replacement replayed startup or retained obsolete callbacks")
	}
}
func TestControllerConnectingSuspendsInactivityDuringRecovery(t *testing.T) {
	c := New()
	if err := c.Start(context.Background(), &recoveryFake{}, 20*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	c.Activity(Connecting)
	time.Sleep(60 * time.Millisecond)
	if !c.Busy() || c.State() != Connecting {
		t.Fatal("recovery lost ownership to inactivity")
	}
	c.Activity(Listening)
	deadline := time.Now().Add(time.Second)
	for c.Busy() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if c.Busy() {
		t.Fatal("recovery permanently disabled inactivity")
	}
}

func TestRecoveryNeverReportsConnectedDuringSlowRetirement(t *testing.T) {
	made := make(chan *recoveryFake, 1)
	closing, release := make(chan struct{}), make(chan struct{})
	r := &Recovery{delays: []time.Duration{time.Hour}}
	r.Factory = func(ctx context.Context, retry bool, fail, end func()) (RecoveryBackend, error) {
		b := &recoveryFake{retryable: true, fail: fail, close: func() { close(closing); <-release }}
		made <- b
		return b, nil
	}
	if err := r.Open(context.Background()); err != nil {
		t.Fatal(err)
	}
	(<-made).fail()
	<-closing
	if r.Status().State == "connected" {
		t.Error("disconnected transport reported connected")
	}
	close(release)
	r.Close()
}

// The Gateway currently reports uncertain input delivery as a message-only
// session.error. Recover the transport, never replay the ambiguous input.
const unconfirmedInputMessage = "OpenClaw could not confirm that input. It was not sent again; check the conversation before retrying."

func TestProviderInputUnconfirmedClassification(t *testing.T) {
	for _, tc := range []struct {
		name, kind, payload, want string
		retry                     bool
	}{
		{"unconfirmed", "session.error", `{"message":"` + unconfirmedInputMessage + `"}`, "provider_input_unconfirmed", true},
		{"malformed", "session.error", `"invalid"`, "provider_session_error", false},
		{"missing", "session.error", `{}`, "provider_session_error", false},
		{"unknown", "session.error", `{"message":"Unknown provider failure"}`, "provider_session_error", false},
		{"auth", "session.error", `{"message":"Unauthorized"}`, "provider_session_error", false},
		{"similar_not_exact", "session.error", `{"message":"could not confirm input"}`, "provider_session_error", false},
		{"replaced", "session.replaced", `{"message":"` + unconfirmedInputMessage + `"}`, "provider_session_replaced", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewSession(nil, "agent:synthetic:handoff", nil)
			failed := make(chan struct{}, 1)
			s.Failure = func() { failed <- struct{}{} }
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			events := make(chan connection.TalkEvent, 2)
			var e connection.TalkEvent
			wire := json.RawMessage(fmt.Sprintf(`{"voiceSessionId":%q,"talkEvent":{"type":%q,"payload":%s}}`, s.id, tc.kind, tc.payload))
			var ok bool
			e, ok = connection.DecodeTalkEvent(wire)
			if !ok {
				t.Fatal("synthetic wire event was rejected")
			}
			if len(e.Event.Payload) != 0 {
				t.Fatal("provider payload escaped privacy filter")
			}
			events <- e
			e.Event.Type, e.Event.Payload = "session.closed", nil
			events <- e
			// Synchronous processing also proves the subsequent close cannot turn a
			// permanent error into a retryable transport close.
			s.control(ctx, events)
			if s.Diagnostics().Reason != tc.want || s.RecoverableFailure() != tc.retry {
				t.Fatalf("reason=%s retry=%v; want %s/%v", s.Diagnostics().Reason, s.RecoverableFailure(), tc.want, tc.retry)
			}
			select {
			case <-failed:
			case <-time.After(time.Second):
				t.Fatal("failure was ignored")
			}
		})
	}
}

type recoveryNoReplayControl struct {
	*fakeControl
	unexpected atomic.Int32
}

func (c *recoveryNoReplayControl) CallTalk(ctx context.Context, method string, params, result any) error {
	switch method {
	case "talk.catalog", "talk.client.create", "talk.client.close":
	default:
		c.unexpected.Add(1)
	}
	return c.fakeControl.CallTalk(ctx, method, params, result)
}

func TestRecoveryRealWebRTCInputUnconfirmedThenClosedNeverReplays(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	const target = "agent:synthetic:transferred"
	sessions := make(chan *Session, 2)
	controls := make(chan *recoveryNoReplayControl, 2)
	peers := make(chan *rtc.Link, 2)
	r := &Recovery{delays: []time.Duration{time.Millisecond}}
	r.Factory = func(ctx context.Context, retry bool, fail, end func()) (RecoveryBackend, error) {
		peer, err := rtc.NewAudioOnly(rtc.Media{Audio: &fakeDevice{frames: make(chan []int16)}}, nil)
		if err != nil {
			return nil, err
		}
		t.Cleanup(func() { peer.Close() })
		c := &recoveryNoReplayControl{fakeControl: &fakeControl{event: make(chan connection.TalkEvent, 8), peer: peer, capable: true}}
		b := NewSession(c, target, &fakeDevice{frames: make(chan []int16)})
		b.nativeMedia = false
		b.Failure, b.End = fail, end
		sessions <- b
		controls <- c
		peers <- peer
		return b, nil
	}
	if err := r.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	first, c1 := <-sessions, <-controls
	var e connection.TalkEvent
	e.VoiceSessionID, e.Event.Type = first.id, "session.error"
	e.Event.Payload, _ = json.Marshal(map[string]string{"message": unconfirmedInputMessage, "token": "synthetic-private-token"})
	wire, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	e, ok := connection.DecodeTalkEvent(wire)
	if !ok || len(e.Event.Payload) != 0 {
		t.Fatal("wire error did not cross the production privacy boundary safely")
	}
	c1.event <- e
	e.Event.Type, e.Event.Payload = "session.closed", nil
	c1.event <- e
	awaitRecovery(t, r, "connected", 1)
	second, c2 := <-sessions, <-controls
	if first.id == second.id || first.Target != target || second.Target != target ||
		c1.createParams["sessionKey"] != target || c2.createParams["sessionKey"] != target {
		t.Fatal("recovery changed session or reused transport identity")
	}
	if c1.unexpected.Load() != 0 || c2.unexpected.Load() != 0 ||
		second.Greeting != "" || second.pendingTools.Load() != 0 || len(second.native.transcripts) != 0 {
		t.Fatal("recovery replayed an input, pending consult or greeting")
	}
	r.Close()
	(<-peers).Close()
	(<-peers).Close()
}
