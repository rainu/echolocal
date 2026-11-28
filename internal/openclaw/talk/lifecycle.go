// Package talk coordinates exclusive, continuous voice sessions.
// Transport, target identifiers and audio remain behind Backend.
package talk

import (
	"context"
	"errors"
	"sync"
	"time"
)

type State string

const (
	Connecting State = "connecting"
	Listening  State = "listening"
	Thinking   State = "thinking"
	Speaking   State = "speaking"
	Ended      State = "ended"
	Failed     State = "error"
	Muted      State = "muted"
)

var ErrBusy = errors.New("conversation_busy")

// Backend.Open must finish only when transport AND audio are ready.
// Close must release all partially acquired resources even when Open fails.
// Implementations must honor cancellation; they must not replay requests.
type Backend interface {
	Open(context.Context) error
	Close() error
}
type Controller struct {
	mu      sync.Mutex
	state   State
	current *activation
}
type activation struct {
	backend      Backend
	cancel       context.CancelFunc
	once         sync.Once
	timer        *time.Timer
	timeout      time.Duration
	deadline     time.Time
	ending       bool
	muted        bool
	agentWorking bool
}

func New() *Controller { return &Controller{state: Ended} }
func (c *Controller) State() State {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.current != nil && c.current.muted {
		return Muted
	}
	return c.state
}

// Start claims ownership before opening any transport. Duplicate activations
// cannot acquire another microphone, including while the first is connecting.
func (c *Controller) Start(ctx context.Context, b Backend, timeout time.Duration) error {
	if b == nil || timeout <= 0 {
		return errors.New("invalid_conversation")
	}
	c.mu.Lock()
	if c.current != nil {
		c.mu.Unlock()
		return ErrBusy
	}
	ctx, cancel := context.WithCancel(ctx)
	a := &activation{backend: b, cancel: cancel, timeout: timeout}
	c.current, c.state = a, Connecting
	c.mu.Unlock()
	go func() { <-ctx.Done(); c.finish(a, Ended) }()
	if err := b.Open(ctx); err != nil {
		outcome := Failed
		if ctx.Err() != nil {
			outcome = Ended
		}
		c.finish(a, outcome)
		return errors.New("conversation_open_failed")
	}
	c.mu.Lock()
	if c.current != a {
		c.mu.Unlock()
		return context.Canceled
	}
	c.state = Listening
	c.arm(a)
	c.mu.Unlock()
	return nil
}
func (c *Controller) arm(a *activation) {
	if a.muted || a.agentWorking {
		return
	}
	if a.timer != nil {
		a.timer.Stop()
	}
	a.deadline = time.Now().Add(a.timeout)
	a.timer = time.AfterFunc(a.timeout, func() {
		c.mu.Lock()
		expired := c.current == a && c.state == Listening && !a.muted && !a.agentWorking && !time.Now().Before(a.deadline)
		if expired {
			a.ending = true
		}
		c.mu.Unlock()
		if expired {
			c.finish(a, Ended)
		}
	})
}

// Activity is called only from verified audio/control events, never from
// arbitrary inbound messages. Agent responses suspend inactivity expiry.
func (c *Controller) Activity(s State) {
	c.mu.Lock()
	defer c.mu.Unlock()
	a := c.current
	if a == nil || a.ending || (c.state != Listening && c.state != Thinking && c.state != Speaking && c.state != Connecting) {
		return
	}
	switch s {
	case Thinking, Speaking, Connecting:
		if a.timer != nil {
			a.timer.Stop()
			a.timer = nil
		}
	case Listening:
		c.arm(a)
	default:
		return
	}
	c.state = s
}
func (c *Controller) Stop() { c.end(Ended) }

// SetMuted changes input presentation and suspends local inactivity expiry.
// Capture/transport gating belongs to the audio backend; ownership, response
// activity and the selected session survive. Unmute never starts a new call.
func (c *Controller) SetMuted(on bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	a := c.current
	if a == nil || a.ending || a.muted == on {
		return
	}
	a.muted = on
	if on {
		if a.timer != nil {
			a.timer.Stop()
			a.timer = nil
		}
	} else if c.state == Listening {
		c.arm(a)
	}
}
func (c *Controller) Mute() { c.SetMuted(true) }
func (c *Controller) Fail() { c.end(Failed) }
func (c *Controller) end(s State) {
	c.mu.Lock()
	a := c.current
	c.mu.Unlock()
	if a != nil {
		c.finish(a, s)
	}
}
func (c *Controller) finish(a *activation, s State) {
	a.once.Do(func() {
		c.mu.Lock()
		if a.timer != nil {
			a.timer.Stop()
		}
		// Retain ownership until Close has returned.
		c.mu.Unlock()
		a.cancel()
		err := a.backend.Close()
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.current == a {
			c.current = nil
			c.state = s
			if err != nil {
				c.state = Failed
			}
		}
	})
}

func (c *Controller) Busy() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.current != nil }

// UserSpeech keeps the truthful Listening state while suspending inactivity for
// a provider-confirmed in-progress utterance. Final speech moves to Thinking.
func (c *Controller) UserSpeech() {
	c.mu.Lock()
	defer c.mu.Unlock()
	a := c.current
	if a == nil || a.ending {
		return
	}
	if a.timer != nil {
		a.timer.Stop()
		a.timer = nil
	}
	c.state = Listening
}

// SetAgentWorking only suspends idle expiry for fresh, verified session work.
// It does not replace Listening/Speaking/Muted or request any speech.
func (c *Controller) SetAgentWorking(on bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	a := c.current
	if a == nil || a.ending || a.agentWorking == on {
		return
	}
	a.agentWorking = on
	if on {
		if a.timer != nil {
			a.timer.Stop()
			a.timer = nil
		}
	} else if c.state == Listening {
		c.arm(a)
	}
}
