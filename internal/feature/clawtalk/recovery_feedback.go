//go:build !board_biscuit

package clawtalk

import (
	"context"
	"sync"
	"time"
)

// One feedback sequence owns an outage across all its retry attempts. It is
// separate from startup feedback and is joined before this activation retires.
type recoveryFeedback struct {
	ctx      context.Context
	sink     feedbackSink
	interval time.Duration
	mu       sync.Mutex
	active   *setupFeedback
	closed   bool
}
type recoverySink struct{ feedbackSink }

func (s recoverySink) Cue(cue string) { s.feedbackSink.Cue("recovery-" + cue) }

func newRecoveryFeedback(ctx context.Context, sink feedbackSink, interval time.Duration) *recoveryFeedback {
	return &recoveryFeedback{ctx: ctx, sink: recoverySink{sink}, interval: interval}
}
func (r *recoveryFeedback) Lost() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.ctx.Err() != nil || r.active != nil {
		return
	}
	r.active = beginFeedbackCue(r.ctx, r.sink, r.interval, "lost")
}
func (r *recoveryFeedback) Ready(ctx context.Context, muted func() bool) bool {
	r.mu.Lock()
	f := r.active
	closed := r.closed
	r.mu.Unlock()
	if f == nil || closed || r.ctx.Err() != nil {
		return false
	}
	ok := f.ReadyWhen(ctx, func() bool { return muted == nil || !muted() })
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.active != f {
		return false
	}
	if ok && !r.closed && r.ctx.Err() == nil {
		r.active = nil
		return true
	}
	// A transport can fail while the ready cue drains. Continue this outage,
	// without repeating its loss cue or leaving a stopped progress loop behind.
	f.sink.Clear()
	if !r.closed && r.ctx.Err() == nil {
		r.active = beginFeedbackCue(r.ctx, r.sink, r.interval, "progress")
	}
	return false
}
func (r *recoveryFeedback) Fail() {
	r.mu.Lock()
	f := r.active
	r.active = nil
	closed := r.closed
	r.mu.Unlock()
	if f == nil {
		return
	}
	f.Stop()
	if closed || r.ctx.Err() != nil {
		r.sink.Clear()
		return
	}
	ctx, cancel := context.WithTimeout(r.ctx, 2*time.Second)
	defer cancel()
	if !r.sink.Wait(ctx) || ctx.Err() != nil {
		r.sink.Clear()
		return
	}
	r.sink.Cue("failure")
	if !r.sink.Wait(ctx) {
		r.sink.Clear()
	}
}
func (r *recoveryFeedback) Close() {
	r.mu.Lock()
	r.closed = true
	f := r.active
	r.active = nil
	r.mu.Unlock()
	if f != nil {
		f.Stop()
		r.sink.Clear()
	}
}
