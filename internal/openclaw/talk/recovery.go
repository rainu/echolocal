package talk

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// Recovery owns one logical Talk across replacement voice transports. Factory
// must snapshot routing/configuration outside this loop and must not replay
// greetings or delivered turns when retry is true.
type RecoveryBackend interface {
	Backend
	RecoverableFailure() bool
}
type RecoveryFactory func(ctx context.Context, retry bool, failed, ended func()) (RecoveryBackend, error)

type Recovery struct {
	Factory RecoveryFactory
	State   func(State)
	Failed  func()
	Ended   func()
	Opened  func()
	// delays is private so production always has a finite, non-resetting budget.
	delays []time.Duration
	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
	closed bool
	status RecoveryStatus
}
type RecoveryStatus struct {
	State    string `json:"state"`
	Attempts int    `json:"attempts"`
	Reason   string `json:"reason,omitempty"`
}

func (r *Recovery) Status() RecoveryStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status
}
func (r *Recovery) update(state string, attempts int, reason string) {
	r.mu.Lock()
	r.status = RecoveryStatus{state, attempts, reason}
	r.mu.Unlock()
}
func (r *Recovery) Open(parent context.Context) error {
	r.mu.Lock()
	if r.closed || r.done != nil {
		r.mu.Unlock()
		return errors.New("talk_recovery_closed")
	}
	ctx, cancel := context.WithCancel(parent)
	r.cancel, r.done = cancel, make(chan struct{})
	r.mu.Unlock()
	ready := make(chan error, 1)
	go r.run(ctx, ready)
	return <-ready
}
func (r *Recovery) Close() error {
	r.mu.Lock()
	r.closed = true
	cancel, done := r.cancel, r.done
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	return nil
}
func (r *Recovery) run(ctx context.Context, ready chan<- error) {
	defer close(r.done)
	notified := false
	notify := func(err error) {
		if !notified {
			notified = true
			ready <- err
		}
	}
	defer func() { notify(context.Canceled) }()
	delays := r.delays
	if delays == nil {
		delays = []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}
	}
	attempts := 0
	recovering := false
	for {
		if ctx.Err() != nil {
			r.update("stopped", attempts, "")
			return
		}
		if recovering {
			if attempts >= len(delays) {
				r.update("failed", attempts, "talk_recovery_exhausted")
				notify(errors.New("talk_recovery_exhausted"))
				if r.Failed != nil {
					r.Failed()
				}
				return
			}
			delay := delays[attempts]
			attempts++
			r.update("reconnecting", attempts, "")
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				r.update("stopped", attempts, "")
				return
			case <-timer.C:
			}
		}
		attempt, cancel := context.WithCancel(ctx)
		var intentional atomic.Bool
		events := make(chan bool, 1) // true = failure; false = deliberate end
		emit := func(failure bool) {
			select {
			case events <- failure:
			default:
			}
		}
		b, err := r.Factory(attempt, recovering, func() { emit(true) }, func() { intentional.Store(true); emit(false) })
		if err == nil {
			err = b.Open(attempt)
		}
		if err != nil {
			retryable := b != nil && b.RecoverableFailure()
			cancel()
			if b != nil {
				b.Close()
			}
			if ctx.Err() != nil {
				r.update("stopped", attempts, "")
				return
			}
			if recovering && retryable {
				continue
			}
			r.update("failed", attempts, "talk_connection_unavailable")
			notify(err)
			if r.Failed != nil {
				r.Failed()
			}
			return
		}
		// Do not announce success if an end/failure arrived during setup.
		var failure bool
		pending := false
		select {
		case failure = <-events:
			pending = true
		default:
		}
		if !pending && ctx.Err() == nil {
			r.update("connected", attempts, "")
			if r.State != nil {
				r.State(Listening)
			}
			notify(nil)
			if r.Opened != nil {
				r.Opened()
			}
			select {
			case <-ctx.Done():
			case failure = <-events:
				pending = true
			}
		}
		stopping := intentional.Load()
		if deliberate, ok := b.(interface{ IntentionalEnd() bool }); ok {
			stopping = stopping || deliberate.IntentionalEnd()
		}
		if stopping {
			pending, failure = true, false
		}
		retryable := pending && failure && b.RecoverableFailure()
		// Retirement cancels callbacks before closing. The factory must silence
		// the old transport's end cue on a recoverable failure.
		if ctx.Err() != nil || stopping {
			r.update("stopped", attempts, "")
		} else if retryable {
			r.update("reconnecting", attempts, "")
			if r.State != nil {
				r.State(Connecting)
			}
		} else {
			r.update("failed", attempts, "talk_connection_unavailable")
		}
		cancel()
		b.Close()
		if ctx.Err() != nil || (pending && !failure) {
			r.update("stopped", attempts, "")
			if pending && !failure && ctx.Err() == nil && r.Ended != nil {
				r.Ended()
			}
			return
		}
		if !retryable {
			r.update("failed", attempts, "talk_connection_unavailable")
			notify(errors.New("talk_connection_unavailable"))
			if r.Failed != nil {
				r.Failed()
			}
			return
		}
		recovering = true
	}
}
