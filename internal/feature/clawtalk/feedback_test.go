//go:build !board_biscuit

package clawtalk

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

type testFeedback struct {
	mu     sync.Mutex
	events []string
}

func (s *testFeedback) Cue(v string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, v)
}
func (s *testFeedback) Clear()                        { s.Cue("clear") }
func (s *testFeedback) Wait(ctx context.Context) bool { return ctx.Err() == nil }
func (s *testFeedback) snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.events...)
}
func TestSetupFeedbackFastReadyStopsWithoutCycleDelay(t *testing.T) {
	s := &testFeedback{}
	f := beginFeedback(context.Background(), s, time.Hour)
	began := time.Now()
	if !f.Ready(context.Background()) {
		t.Fatal("not ready")
	}
	f.Finish(false)
	if time.Since(began) > time.Second {
		t.Fatal("waited for progress cycle")
	}
	got := s.snapshot()
	if len(got) != 2 || got[0] != "ack" || got[1] != "ready" {
		t.Fatal(got)
	}
}
func TestSetupFeedbackSlowFailureAndCancellation(t *testing.T) {
	for _, failure := range []bool{true, false} {
		ctx, cancel := context.WithCancel(context.Background())
		s := &testFeedback{}
		f := beginFeedback(ctx, s, time.Millisecond)
		deadline := time.Now().Add(time.Second)
		for len(s.snapshot()) < 3 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if len(s.snapshot()) < 3 {
			t.Fatal("progress did not repeat")
		}
		cancel()
		f.Finish(failure)
		before := s.snapshot()
		time.Sleep(5 * time.Millisecond)
		after := s.snapshot()
		if len(before) != len(after) {
			t.Fatal("progress continued after finish", after)
		}
		last := after[len(after)-1]
		if failure && last != "failure" || !failure && last != "clear" {
			t.Fatal(after)
		}
		for _, e := range after {
			if e == "ready" {
				t.Fatal("false readiness", after)
			}
		}
	}
}
func TestSetupFeedbackCancelledNeverReady(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	f := beginFeedback(ctx, &testFeedback{}, time.Hour)
	cancel()
	if f.Ready(ctx) {
		t.Fatal("cancelled ready")
	}
	f.Finish(false)
}
func TestSetupCuesDistinct(t *testing.T) {
	seen := map[string]bool{}
	for _, cue := range []string{"ack", "progress", "ready", "failure"} {
		notes := SetupNotes(cue)
		if len(notes) == 0 {
			t.Fatal(cue)
		}
		key := fmt.Sprint(notes)
		if seen[key] {
			t.Fatal("duplicate cue", cue)
		}
		seen[key] = true
	}
}

func TestRepeatedPendingWakeDoesNotStartAnotherActivation(t *testing.T) {
	m := &Manager{active: true, generation: 7}
	for i := 0; i < 100; i++ {
		if !m.Route(0, false) {
			t.Fatal("wake not consumed")
		}
	}
	if m.generation != 7 {
		t.Fatal("duplicate activation")
	}
	if _, err := m.StartOwned(0, false); err == nil {
		t.Fatal("duplicate owned activation")
	}
}

func TestSetupDeadlineStopsProgressAndEmitsFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
	defer cancel()
	sink := &testFeedback{}
	f := beginFeedback(ctx, sink, time.Millisecond)
	<-ctx.Done()
	f.Finish(true)
	events := sink.snapshot()
	if len(events) < 4 || events[0] != "ack" || events[len(events)-2] != "clear" || events[len(events)-1] != "failure" {
		t.Fatal("timeout ordering", events)
	}
	time.Sleep(5 * time.Millisecond)
	if len(sink.snapshot()) != len(events) {
		t.Fatal("progress survived timeout")
	}
	for _, event := range events {
		if event == "ready" {
			t.Fatal("timeout became ready")
		}
	}
}

func TestSlowSuccessClearsProgressBeforeReady(t *testing.T) {
	sink := &testFeedback{}
	f := beginFeedback(context.Background(), sink, time.Millisecond)
	deadline := time.Now().Add(time.Second)
	for len(sink.snapshot()) < 3 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(sink.snapshot()) < 3 {
		t.Fatal("no progress")
	}
	if !f.Ready(context.Background()) {
		t.Fatal("not ready")
	}
	f.Finish(false)
	events := sink.snapshot()
	if events[len(events)-2] != "clear" || events[len(events)-1] != "ready" {
		t.Fatal(events)
	}
	time.Sleep(5 * time.Millisecond)
	if len(sink.snapshot()) != len(events) {
		t.Fatal("progress resumed after readiness")
	}
}

type drainingFeedback struct {
	testFeedback
	entered chan struct{}
}

func (s *drainingFeedback) Wait(ctx context.Context) bool {
	close(s.entered)
	<-ctx.Done()
	return false
}
func TestCancellationDuringProgressDrainDoesNotEmitReady(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sink := &drainingFeedback{entered: make(chan struct{})}
	f := beginFeedback(ctx, sink, time.Hour)
	done := make(chan bool, 1)
	go func() { done <- f.Ready(ctx) }()
	<-sink.entered
	cancel()
	if <-done {
		t.Fatal("cancelled drain became ready")
	}
	f.Finish(false)
	for _, event := range sink.snapshot() {
		if event == "ready" {
			t.Fatal("ready after cancellation")
		}
	}
}
