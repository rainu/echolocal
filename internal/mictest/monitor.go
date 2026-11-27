// Package mictest implements a bounded, RAM-only processed microphone test.
package mictest

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

const Rate = 16000
const MaxSamples = 8 * Rate
const Lease = 10 * time.Second
const Retention = 2 * time.Minute

var ErrBusy = errors.New("busy")
var ErrOwner = errors.New("not_owner")

type State struct {
	Phase      string  `json:"phase"`
	Error      string  `json:"error,omitempty"`
	Seconds    float64 `json:"seconds"`
	Processing string  `json:"processing,omitempty"`
	Owned      bool    `json:"owned"`
}
type Options struct {
	Listen     func() (<-chan []int16, func())
	Check      func() error
	Busy       func() bool
	OutputBusy func() bool
	Play       func(context.Context, []int16) error
	Processing func() string
	Changed    func()
}
type Monitor struct {
	generation                 uint64
	resetting                  bool
	o                          Options
	ops                        sync.Mutex
	mu                         sync.Mutex
	active                     atomic.Bool
	owner                      string
	phase, problem, processing string
	pcm                        []int16
	lease, expires             time.Time
	cancel                     context.CancelFunc
	done                       chan struct{}
}

func New(o Options) *Monitor    { return &Monitor{o: o, phase: "idle"} }
func (m *Monitor) Name() string { return "microphone test" }
func (m *Monitor) Active() bool { return m.active.Load() }
func (m *Monitor) State(owner string) State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return State{Phase: m.phase, Error: m.problem, Seconds: float64(len(m.pcm)) / Rate, Processing: m.processing, Owned: m.owner != "" && m.owner == owner}
}
func (m *Monitor) changed() {
	if m.o.Changed != nil {
		m.o.Changed()
	}
}
func (m *Monitor) clearLocked() {
	m.generation++
	clear(m.pcm)
	m.pcm = nil
	m.owner = ""
	m.phase = "idle"
	m.problem = ""
	m.processing = ""
	m.active.Store(false)
	m.resetting = false
}
func (m *Monitor) Command(owner, action string) error {
	if len(owner) < 16 || len(owner) > 128 {
		return ErrOwner
	}
	m.ops.Lock()
	defer m.ops.Unlock()
	m.mu.Lock()
	if m.owner != "" && m.owner != owner {
		m.mu.Unlock()
		return ErrOwner
	}
	if action == "ping" {
		if m.owner == owner {
			m.lease = time.Now().Add(Lease)
		}
		m.mu.Unlock()
		return nil
	}
	if action == "record" {
		if m.active.Load() {
			m.mu.Unlock()
			return ErrBusy
		}
		m.active.Store(true)
		if (m.o.Busy != nil && m.o.Busy()) || (m.o.OutputBusy != nil && m.o.OutputBusy()) {
			m.active.Store(false)
			m.mu.Unlock()
			return ErrBusy
		}
		if err := m.o.Check(); err != nil {
			m.problem = err.Error()
			m.active.Store(false)
			m.mu.Unlock()
			m.changed()
			return err
		}
		clear(m.pcm)
		m.pcm = make([]int16, 0, MaxSamples)
		m.problem = ""
		m.resetting = false
		m.owner = owner
		m.generation++
		m.phase = "recording"
		m.active.Store(true)
		m.processing = m.o.Processing()
		now := time.Now()
		m.lease = now.Add(Lease)
		m.expires = now.Add(Retention)
		ctx, cancel := context.WithCancel(context.Background())
		m.cancel = cancel
		m.done = make(chan struct{})
		done := m.done
		m.mu.Unlock()
		go m.capture(ctx, done)
		m.changed()
		return nil
	}
	if m.owner != owner {
		m.mu.Unlock()
		return ErrOwner
	}
	if action == "play" {
		if m.phase != "ready" || len(m.pcm) == 0 {
			m.mu.Unlock()
			return ErrBusy
		}
		m.active.Store(true)
		if (m.o.Busy != nil && m.o.Busy()) || (m.o.OutputBusy != nil && m.o.OutputBusy()) {
			m.active.Store(false)
			m.mu.Unlock()
			return ErrBusy
		}
		// Playback never subscribes to the microphone. Keep the wake gate closed
		// until the output callback has drained its hardware tail.
		m.generation++
		m.phase = "playing"
		m.problem = ""
		m.active.Store(true)
		ctx, cancel := context.WithCancel(context.Background())
		m.cancel = cancel
		m.done = make(chan struct{})
		done := m.done
		samples := m.pcm
		m.mu.Unlock()
		go m.play(ctx, done, samples)
		m.changed()
		return nil
	}
	if action != "stop" && action != "discard" {
		m.mu.Unlock()
		return errors.New("unknown_action")
	}
	cancel, done := m.cancel, m.done
	m.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
	m.mu.Lock()
	if action == "discard" {
		m.clearLocked()
	}
	m.mu.Unlock()
	m.changed()
	return nil
}
func (m *Monitor) capture(ctx context.Context, done chan struct{}) {
	defer close(done)
	frames, stop := m.o.Listen()
	stop = sync.OnceFunc(stop)
	defer stop()
	deadline := time.NewTimer(9 * time.Second)
	defer deadline.Stop()
	stall := time.NewTimer(time.Second)
	defer stall.Stop()
	var problem string
loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case <-deadline.C:
			problem = "capture_timeout"
			break loop
		case <-stall.C:
			problem = "capture_unavailable"
			break loop
		case frame, ok := <-frames:
			if !ok {
				problem = "capture_unavailable"
				break loop
			}
			if len(frame) == 0 {
				continue
			}
			// Recheck mute before retaining every frame, not just at start.
			if err := m.o.Check(); err != nil {
				problem = err.Error()
				break loop
			}
			stall.Reset(time.Second)
			m.mu.Lock()
			n := min(len(frame), MaxSamples-len(m.pcm))
			m.pcm = append(m.pcm, frame[:n]...)
			full := len(m.pcm) == MaxSamples
			m.mu.Unlock()
			if full {
				break loop
			}
		}
	}
	// Unsubscribe before opening the playback/wake gate.
	stop()
	m.mu.Lock()
	if m.resetting {
		m.mu.Unlock()
		return
	}
	if problem != "" || len(m.pcm) == 0 {
		if problem == "" {
			problem = "no_audio"
		}
		clear(m.pcm)
		m.pcm = nil
		m.phase = "error"
		m.problem = problem
	} else {
		m.phase = "ready"
	}
	m.generation++
	m.active.Store(false)
	m.mu.Unlock()
	m.changed()
}
func (m *Monitor) play(ctx context.Context, done chan struct{}, samples []int16) {
	defer close(done)
	err := m.o.Play(ctx, samples)
	m.mu.Lock()
	if m.resetting {
		m.mu.Unlock()
		return
	}
	m.phase = "ready"
	if err != nil && ctx.Err() == nil {
		m.phase = "error"
		m.problem = "playback_failed"
	}
	m.generation++
	m.active.Store(false)
	m.mu.Unlock()
	m.changed()
}

// Run checks mute, competing voice operation and abandoned browser/device pages.
// Even a crashed browser cannot keep an abandoned microphone listener alive.
func (m *Monitor) Run(ctx context.Context) error {
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	defer m.reset("")
	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-tick.C:
			m.mu.Lock()
			owner := m.owner
			expired := owner != "" && (now.After(m.lease) || now.After(m.expires))
			active := m.active.Load()
			phase := m.phase
			generation := m.generation
			m.mu.Unlock()
			if expired {
				m.resetIf("expired", generation)
				continue
			}
			if !active {
				continue
			}
			problem := ""
			if phase == "recording" && m.o.OutputBusy != nil && m.o.OutputBusy() {
				problem = "output_busy"
			}
			if m.o.Busy != nil && m.o.Busy() {
				problem = "busy"
			}
			if phase == "recording" {
				if err := m.o.Check(); err != nil {
					problem = err.Error()
				}
			}
			if problem != "" {
				m.resetIf(problem, generation)
			}
		}
	}
}
func (m *Monitor) reset(problem string) { m.resetIf(problem, 0) }
func (m *Monitor) resetIf(problem string, generation uint64) {
	m.ops.Lock()
	defer m.ops.Unlock()
	m.mu.Lock()
	if generation != 0 && m.generation != generation {
		m.mu.Unlock()
		return
	}
	if problem == "expired" && time.Now().Before(m.lease) && time.Now().Before(m.expires) {
		m.mu.Unlock()
		return
	}
	m.resetting = true
	cancel, done := m.cancel, m.done
	m.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
	m.mu.Lock()
	m.clearLocked()
	m.cancel = nil
	m.done = nil
	if problem != "" {
		m.phase = "error"
		m.problem = problem
	}
	m.mu.Unlock()
	m.changed()
}
