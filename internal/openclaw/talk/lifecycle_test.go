package talk

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type backend struct {
	opened, closed atomic.Int32
	open           func(context.Context) error
	close          func() error
}

func (b *backend) Open(ctx context.Context) error {
	b.opened.Add(1)
	if b.open != nil {
		return b.open(ctx)
	}
	return nil
}
func (b *backend) Close() error {
	b.closed.Add(1)
	if b.close != nil {
		return b.close()
	}
	return nil
}
func await(t *testing.T, fn func() bool) {
	t.Helper()
	until := time.Now().Add(time.Second)
	for time.Now().Before(until) {
		if fn() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition not reached")
}
func TestExclusiveActivationAndStop(t *testing.T) {
	c := New()
	entered := make(chan struct{})
	release := make(chan struct{})
	b := &backend{open: func(ctx context.Context) error {
		close(entered)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}}
	done := make(chan error, 1)
	go func() { done <- c.Start(context.Background(), b, time.Hour) }()
	<-entered
	other := &backend{}
	if c.State() != Connecting || !errors.Is(c.Start(context.Background(), other, time.Hour), ErrBusy) {
		t.Fatal("duplicate accepted")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if c.State() != Listening {
		t.Fatal("not ready")
	}
	c.Stop()
	c.Stop()
	c.Fail()
	if b.closed.Load() != 1 || other.opened.Load() != 0 || c.State() != Ended {
		t.Fatal("non-idempotent shutdown")
	}
}
func TestMuteDuringOpenRetainsOwnershipAndStopStillCancels(t *testing.T) {
	c := New()
	entered := make(chan struct{})
	b := &backend{open: func(ctx context.Context) error { close(entered); <-ctx.Done(); return ctx.Err() }}
	done := make(chan error, 1)
	go func() { done <- c.Start(context.Background(), b, time.Hour) }()
	<-entered
	c.Mute()
	if !c.Busy() || c.State() != Muted || b.closed.Load() != 0 {
		t.Fatal("mute cancelled setup")
	}
	c.SetMuted(false)
	if c.State() != Connecting {
		t.Fatal("lost connecting state")
	}
	c.Stop()
	if err := <-done; err == nil {
		t.Fatal("stopped setup succeeded")
	}
	if c.State() != Ended || b.closed.Load() != 1 {
		t.Fatal("stop did not terminate")
	}
}

func TestMuteSuspendsExpiryAndPreservesResponseActivity(t *testing.T) {
	c := New()
	b := &backend{}
	if err := c.Start(context.Background(), b, 30*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	defer c.Stop()
	for _, state := range []State{Listening, Thinking, Speaking} {
		c.Activity(state)
		c.Mute()
		c.Mute()
		time.Sleep(65 * time.Millisecond)
		c.Activity(Speaking)
		if !c.Busy() || c.State() != Muted || b.closed.Load() != 0 {
			t.Fatal("mute lost call")
		}
		c.SetMuted(false)
		if c.State() != Speaking {
			t.Fatal("activity while muted was lost")
		}
	}
	c.Mute()
	c.Activity(Listening)
	time.Sleep(65 * time.Millisecond)
	if !c.Busy() {
		t.Fatal("expiry while muted")
	}
	c.SetMuted(false)
	await(t, func() bool { return !c.Busy() })
	if c.State() != Ended || b.opened.Load() != 1 || b.closed.Load() != 1 {
		t.Fatal("unmute restarted or expiry broken")
	}
	c.SetMuted(false)
	if c.Busy() {
		t.Fatal("unmute restarted ended call")
	}
}

func TestMutedConversationCanStillEndOrFail(t *testing.T) {
	for _, failure := range []bool{false, true} {
		c := New()
		b := &backend{}
		if err := c.Start(context.Background(), b, time.Hour); err != nil {
			t.Fatal(err)
		}
		c.Mute()
		want := Ended
		if failure {
			c.Fail()
			want = Failed
		} else {
			c.Stop()
		}
		if c.State() != want || c.Busy() || b.closed.Load() != 1 {
			t.Fatal("mute blocked termination")
		}
	}
}

func TestInactivitySuspendedWhileResponseInProgress(t *testing.T) {
	c := New()
	b := &backend{}
	if err := c.Start(context.Background(), b, 30*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	c.Activity(Thinking)
	time.Sleep(60 * time.Millisecond)
	if c.State() != Thinking {
		t.Fatal("expired during thinking")
	}
	c.Activity(Speaking)
	time.Sleep(60 * time.Millisecond)
	if c.State() != Speaking {
		t.Fatal("expired during speech")
	}
	c.Activity(Listening)
	await(t, func() bool { return c.State() == Ended })
	if b.closed.Load() != 1 {
		t.Fatal("timeout did not close")
	}
}
func TestFailureAndContextCancellation(t *testing.T) {
	for _, fail := range []bool{true, false} {
		c := New()
		ctx, cancel := context.WithCancel(context.Background())
		b := &backend{}
		if fail {
			b.open = func(context.Context) error { return errors.New("secret upstream error") }
		}
		err := c.Start(ctx, b, time.Hour)
		if fail {
			if err == nil || err.Error() != "conversation_open_failed" || c.State() != Failed {
				t.Fatal("failure classification")
			}
		} else {
			cancel()
			await(t, func() bool { return c.State() == Ended })
		}
		cancel()
		if b.closed.Load() != 1 {
			t.Fatal("close count")
		}
	}
}
func TestOwnershipRetainedUntilResourcesReleased(t *testing.T) {
	c := New()
	entered := make(chan struct{})
	release := make(chan struct{})
	b := &backend{close: func() error { close(entered); <-release; return nil }}
	if err := c.Start(context.Background(), b, time.Hour); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { c.Stop(); close(done) }()
	<-entered
	if !errors.Is(c.Start(context.Background(), &backend{}, time.Hour), ErrBusy) {
		t.Fatal("overlapping audio owners")
	}
	close(release)
	<-done
	if err := c.Start(context.Background(), &backend{}, time.Hour); err != nil {
		t.Fatal(err)
	}
	c.Stop()
}

func TestCancelledConnectingCallDoesNotReportProviderFailure(t *testing.T) {
	for i := 0; i < 20; i++ {
		c := New()
		ctx, cancel := context.WithCancel(context.Background())
		ready := make(chan struct{})
		b := &backend{open: func(ctx context.Context) error { close(ready); <-ctx.Done(); return ctx.Err() }}
		done := make(chan error, 1)
		go func() { done <- c.Start(ctx, b, time.Hour) }()
		<-ready
		cancel()
		<-done
		await(t, func() bool { return !c.Busy() })
		if c.State() != Ended {
			t.Fatal("user cancellation presented as provider failure")
		}
	}
}
