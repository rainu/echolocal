//go:build !board_biscuit

package clawtalk

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestCaptureMuteDiscardsInputAndClocksSilenceWithoutSource(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	source := make(chan []int16, 8)
	out := make(chan []int16)
	var muted, ready atomic.Bool
	var captured atomic.Int32
	ready.Store(true)
	done := make(chan struct{})
	go func() {
		defer close(done)
		runCapture(ctx, source, out, captureHooks{
			muted: muted.Load, ready: ready.Load, captured: func() { captured.Add(1) }, dropped: func() {},
		})
	}()
	receive := func() []int16 {
		t.Helper()
		select {
		case f := <-out:
			return f
		case <-time.After(time.Second):
			t.Fatal("capture stalled")
			return nil
		}
	}
	// Retry feeds while waiting: production capture intentionally drops rather
	// than buffering when there is no transport consumer.
	feed := func(value int16) {
		select {
		case source <- []int16{value}:
		default:
		}
	}
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	feedDone := make(chan struct{})
	var value atomic.Int32
	value.Store(1234)
	go func() {
		defer close(feedDone)
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				if v := value.Load(); v != 0 {
					feed(int16(v))
				}
			}
		}
	}()
	if f := receive(); len(f) != 1 || f[0] != 1234 {
		t.Fatal("live input missing", f)
	}
	muted.Store(true)
	// Let the pump observe the edge, then deliberately feed nonzero input.
	// Hardware cuts must not be the only defence against forwarding stale PCM.
	for i := 0; i < 5; i++ {
		f := receive()
		for _, v := range f {
			if v != 0 {
				t.Fatal("muted microphone sample forwarded")
			}
		}
	}
	baseline := captured.Load()
	value.Store(0)
	for i := 0; i < 4; i++ {
		f := receive()
		for _, v := range f {
			if v != 0 {
				t.Fatal("nonzero silence")
			}
		}
	}
	if captured.Load() != baseline {
		t.Fatal("silence counted as captured microphone")
	}
	ready.Store(false)
	select {
	case <-out:
		t.Fatal("feedback gate forwarded muted audio")
	case <-time.After(45 * time.Millisecond):
	}
	ready.Store(true)
	// Release drops stale input instead of replaying it.
	for i := 0; i < cap(source); i++ {
		feed(777)
	}
	muted.Store(false)
	value.Store(2345)
	for i := 0; i < 3; i++ {
		f := receive()
		if len(f) != 1 || f[0] != 2345 {
			t.Fatal("stale mute-period input replayed", f)
		}
	}
	cancel()
	<-done
	<-feedDone
}

func TestCaptureCancellationAndClosedSourceDoNotLeak(t *testing.T) {
	for _, closed := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		source := make(chan []int16)
		out := make(chan []int16)
		done := make(chan struct{})
		go func() {
			defer close(done)
			runCapture(ctx, source, out, captureHooks{muted: func() bool { return true }, ready: func() bool { return true }, captured: func() { t.Error("captured while muted") }, dropped: func() {}})
		}()
		if closed {
			close(source)
		} else {
			cancel()
		}
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("capture did not release")
		}
		cancel()
	}
}
