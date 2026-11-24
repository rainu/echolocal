//go:build !board_biscuit

package clawtalk

import (
	"context"
	"sync"
	"time"
)

// setupFeedback owns one cancellable sequence, never an overlay that survives
// cancellation. The sink must enqueue serial PCM and clear only its own lane.
type feedbackSink interface {
	Cue(string)
	Clear()
	Wait(context.Context) bool
}

type setupFeedback struct {
	sink   feedbackSink
	cancel context.CancelFunc
	done   chan struct{}
	stop   sync.Once
	ready  bool
}

func beginFeedback(ctx context.Context, sink feedbackSink, interval time.Duration) *setupFeedback {
	loop, cancel := context.WithCancel(ctx)
	f := &setupFeedback{sink: sink, cancel: cancel, done: make(chan struct{})}
	// Acknowledgment precedes any network work.
	sink.Cue("ack")
	go func() {
		defer close(f.done)
		progress := false
		defer func() {
			// Preserve the short acknowledgment on a very fast successful setup.
			// Cancellation still clears it; a pending ring is always truncated.
			if progress || ctx.Err() != nil {
				sink.Clear()
			}
		}()
		timer := time.NewTimer(interval)
		defer timer.Stop()
		for {
			select {
			case <-loop.Done():
				return
			case <-timer.C:
				if loop.Err() != nil {
					return
				}
				progress = true
				sink.Cue("progress")
				// Custom clips may exceed the repeat interval. Never queue a backlog.
				if !sink.Wait(loop) {
					return
				}
				timer.Reset(interval)
			}
		}
	}()
	return f
}
func (f *setupFeedback) Stop() {
	f.stop.Do(func() { f.cancel(); <-f.done })
}
func (f *setupFeedback) Ready(ctx context.Context) bool {
	f.Stop()
	if ctx.Err() != nil || !f.sink.Wait(ctx) || ctx.Err() != nil {
		return false
	}
	f.sink.Cue("ready")
	if !f.sink.Wait(ctx) {
		f.sink.Clear()
		return false
	}
	f.ready = true
	return true
}
func (f *setupFeedback) Finish(failed bool) {
	f.Stop()
	if failed && !f.ready {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if f.sink.Wait(ctx) {
			f.sink.Cue("failure")
			f.sink.Wait(ctx)
		}
	} else if !f.ready {
		f.sink.Clear()
	}
}
