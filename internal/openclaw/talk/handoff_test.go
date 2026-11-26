package talk

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type handoffAudio struct {
	fakeDevice
	queue atomic.Int32
}

func (a *handoffAudio) Queued() int { return int(a.queue.Load()) }
func TestHandoffRequiresDeliveredConsultationAudibleResponseAndDrain(t *testing.T) {
	a := &handoffAudio{}
	s := NewSession(nil, "agent:test:one", a)
	s.pendingTools.Store(1)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	sent := make(chan struct{})
	finished := make(chan error, 1)
	go func() { finished <- s.announceHandoff(ctx, func() error { close(sent); return nil }) }()
	select {
	case <-sent:
		t.Fatal("confirmation raced consultation")
	case <-time.After(650 * time.Millisecond):
	}
	s.pendingTools.Store(0)
	<-sent
	s.native.responses.Add(1)
	select {
	case err := <-finished:
		t.Fatal("no PCM but completed", err)
	case <-time.After(100 * time.Millisecond):
	}
	a.queue.Store(8)
	s.Playback()
	select {
	case err := <-finished:
		t.Fatal("queue not drained", err)
	case <-time.After(800 * time.Millisecond):
	}
	a.queue.Store(0)
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("drained confirmation did not complete")
	}
}
func TestHandoffMissingOrRejectedConfirmationDoesNotHangUp(t *testing.T) {
	for _, tc := range []struct {
		name       string
		pcm, final bool
		reject     bool
	}{
		{"no_audio", false, true, false}, {"no_completion", true, false, false}, {"rejected", false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewSession(nil, "agent:test:one", &fakeDevice{})
			var closed atomic.Bool
			s.End = func() { closed.Store(true) }
			ctx, cancel := context.WithTimeout(context.Background(), 900*time.Millisecond)
			defer cancel()
			err := s.announceHandoff(ctx, func() error {
				if tc.reject {
					return errors.New("private provider error")
				}
				if tc.pcm {
					s.Playback()
				}
				if tc.final {
					s.native.responses.Add(1)
				}
				return nil
			})
			if err == nil || closed.Load() {
				t.Fatal("unconfirmed handoff ended Talk", err)
			}
		})
	}
}
func TestHandoffCancelledBeforeConfirmationSendsNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := NewSession(nil, "agent:test:one", &fakeDevice{})
	sent := false
	if err := s.announceHandoff(ctx, func() error { sent = true; return nil }); err == nil || sent {
		t.Fatal("cancelled announcement sent")
	}
}
