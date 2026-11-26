//go:build !board_biscuit

package clawtalk

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func countCue(events []string, want string) int {
	n := 0
	for _, e := range events {
		if e == want {
			n++
		}
	}
	return n
}
func waitFeedback(t *testing.T, check func() bool) {
	t.Helper()
	end := time.Now().Add(time.Second)
	for time.Now().Before(end) {
		if check() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("feedback condition not reached")
}
func TestRecoveryFeedbackOneLossPerOutageAndStopsAfterReady(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sink := &testFeedback{}
	f := newRecoveryFeedback(ctx, sink, time.Millisecond)
	defer f.Close()
	f.Lost()
	f.Lost()
	waitFeedback(t, func() bool { return countCue(sink.snapshot(), "recovery-progress") > 0 })
	if !f.Ready(ctx, nil) {
		t.Fatal("not ready")
	}
	events := sink.snapshot()
	if countCue(events, "recovery-lost") != 1 || countCue(events, "recovery-ready") != 1 ||
		events[len(events)-1] != "recovery-ready" {
		t.Fatal(events)
	}
	time.Sleep(5 * time.Millisecond)
	if len(sink.snapshot()) != len(events) {
		t.Fatal("progress continued after ready")
	}
	f.Lost()
	if !f.Ready(ctx, func() bool { return true }) {
		t.Fatal("muted transport did not finish")
	}
	events = sink.snapshot()
	if countCue(events, "recovery-lost") != 2 || countCue(events, "recovery-ready") != 1 {
		t.Fatal("muted recovery announced listening", events)
	}
	sink.Cue("normal-end")
	f.Close()
	if last := sink.snapshot(); last[len(last)-1] != "normal-end" {
		t.Fatal("recovery cleanup truncated normal end", last)
	}
}
func TestRecoveryFeedbackCancellationStopsImmediately(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sink := &blockedFeedback{waiting: make(chan struct{}, 2)}
	f := newRecoveryFeedback(ctx, sink, time.Millisecond)
	f.Lost()
	select {
	case <-sink.waiting:
	case <-time.After(time.Second):
		t.Fatal("no progress")
	}
	began := time.Now()
	cancel()
	f.Close()
	if time.Since(began) > 200*time.Millisecond {
		t.Fatal("cancel blocked on clip")
	}
	before := sink.snapshot()
	time.Sleep(5 * time.Millisecond)
	after := sink.snapshot()
	if len(before) != len(after) || countCue(after, "recovery-ready") != 0 {
		t.Fatal(after)
	}
}

type gatedRecoverySink struct {
	testFeedback
	entered chan chan struct{}
}

func (s *gatedRecoverySink) Wait(ctx context.Context) bool {
	gate := make(chan struct{})
	select {
	case s.entered <- gate:
	case <-ctx.Done():
		return false
	}
	select {
	case <-gate:
		return ctx.Err() == nil
	case <-ctx.Done():
		return false
	}
}
func TestRecoveryReadyCueDrainsBeforeCaptureGate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sink := &gatedRecoverySink{entered: make(chan chan struct{}, 4)}
	f := newRecoveryFeedback(ctx, sink, time.Hour)
	defer f.Close()
	f.Lost()
	a := &deviceAudio{recovering: true, recoveryNotice: f}
	result := make(chan error, 1)
	go func() { result <- a.ReadyTone(ctx) }()
	var first, second chan struct{}
	select {
	case first = <-sink.entered:
	case <-time.After(time.Second):
		t.Fatal("no pre-ready drain")
	}
	if a.ready.Load() || countCue(sink.snapshot(), "recovery-ready") != 0 {
		t.Fatal("premature listening")
	}
	close(first)
	select {
	case second = <-sink.entered:
	case <-time.After(time.Second):
		t.Fatal("no ready drain")
	}
	if a.ready.Load() || countCue(sink.snapshot(), "recovery-ready") != 1 {
		t.Fatal("capture opened before cue drain")
	}
	close(second)
	if err := <-result; err != nil || !a.ready.Load() {
		t.Fatal("capture gate did not open", err)
	}
}
func TestRecoveryReadyFailureResumesProgressWithoutAnotherLoss(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	attempt, abort := context.WithCancel(ctx)
	sink := &gatedRecoverySink{entered: make(chan chan struct{}, 4)}
	f := newRecoveryFeedback(ctx, sink, time.Hour)
	defer f.Close()
	f.Lost()
	done := make(chan bool, 1)
	go func() { done <- f.Ready(attempt, nil) }()
	select {
	case <-sink.entered:
	case <-time.After(time.Second):
		t.Fatal("no ready drain")
	}
	abort()
	if <-done {
		t.Fatal("failed attempt became ready")
	}
	events := sink.snapshot()
	if countCue(events, "recovery-lost") != 1 || countCue(events, "recovery-progress") != 1 ||
		countCue(events, "recovery-ready") != 0 {
		t.Fatal(events)
	}
}
func TestRecoveryFailureCueAndExplicitStopAreDifferent(t *testing.T) {
	for _, stop := range []bool{false, true} {
		t.Run(fmt.Sprint(stop), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			sink := &testFeedback{}
			f := newRecoveryFeedback(ctx, sink, time.Hour)
			f.Lost()
			if stop {
				cancel()
			}
			f.Fail()
			f.Close()
			events := sink.snapshot()
			want := 1
			if stop {
				want = 0
			}
			if countCue(events, "recovery-failure") != want || countCue(events, "recovery-ready") != 0 {
				t.Fatal(events)
			}
		})
	}
}
func TestRecoveryCuePCMIsDistinctNonSilentAndProgressIsSubtle(t *testing.T) {
	seen := map[string]bool{}
	peak := func(pcm []int16) int32 {
		var max int32
		for _, s := range pcm {
			x := int32(s)
			if x < 0 {
				x = -x
			}
			if x > max {
				max = x
			}
		}
		return max
	}
	for _, cue := range []string{"recovery-lost", "recovery-progress", "recovery-ready", "recovery-failure"} {
		notes := fmt.Sprint(SetupNotes(cue))
		if notes == "[]" || seen[notes] {
			t.Fatal("missing or identical cue", cue)
		}
		seen[notes] = true
		if peak(tonePCM("", cue)) == 0 {
			t.Fatal("silent cue", cue)
		}
	}
	if peak(tonePCM("", "recovery-progress")) >= peak(tonePCM("", "recovery-ready")) {
		t.Fatal("progress not quieter")
	}
}

func TestRecoveryReadyChecksMuteAfterPendingDrain(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sink := &gatedRecoverySink{entered: make(chan chan struct{}, 4)}
	f := newRecoveryFeedback(ctx, sink, time.Hour)
	defer f.Close()
	f.Lost()
	a := &deviceAudio{recovering: true, recoveryNotice: f}
	done := make(chan error, 1)
	go func() { done <- a.ReadyTone(ctx) }()
	var gate chan struct{}
	select {
	case gate = <-sink.entered:
	case <-time.After(time.Second):
		t.Fatal("no drain")
	}
	a.inputMuted.Store(true)
	close(gate)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if countCue(sink.snapshot(), "recovery-ready") != 0 || !a.inputMuted.Load() {
		t.Fatal("mute was ignored during readiness")
	}
}
func TestRecoveryCancellationDuringReadyCueKeepsCaptureClosed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	attempt, abort := context.WithCancel(ctx)
	sink := &gatedRecoverySink{entered: make(chan chan struct{}, 4)}
	f := newRecoveryFeedback(ctx, sink, time.Hour)
	defer f.Close()
	f.Lost()
	a := &deviceAudio{recovering: true, recoveryNotice: f}
	done := make(chan error, 1)
	go func() { done <- a.ReadyTone(attempt) }()
	var gate chan struct{}
	select {
	case gate = <-sink.entered:
	case <-time.After(time.Second):
		t.Fatal("no pending drain")
	}
	close(gate)
	select {
	case <-sink.entered:
	case <-time.After(time.Second):
		t.Fatal("ready cue did not start")
	}
	abort()
	if err := <-done; err == nil || a.ready.Load() {
		t.Fatal("capture opened despite cancelled ready cue")
	}
	events := sink.snapshot()
	if countCue(events, "recovery-progress") != 1 || countCue(events, "clear") == 0 {
		t.Fatal("failed ready did not clear/resume outage", events)
	}
}
