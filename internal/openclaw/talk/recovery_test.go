package talk

import (
	"context"
	"errors"
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
	for _, reason := range []string{"talk_peer_failed", "talk_control_closed", "talk_transport_closed", "control_stream_closed", "rpc_disconnected"} {
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
