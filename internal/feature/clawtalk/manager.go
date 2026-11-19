//go:build !board_biscuit

package clawtalk

import (
	"context"
	"errors"
	"github.com/ygelfand/echolocal/internal/board"
	"github.com/ygelfand/echolocal/internal/component"
	"github.com/ygelfand/echolocal/internal/feature/idle"
	"github.com/ygelfand/echolocal/internal/feature/mute"
	claw "github.com/ygelfand/echolocal/internal/feature/openclaw"
	shellfeature "github.com/ygelfand/echolocal/internal/feature/shell"
	"github.com/ygelfand/echolocal/internal/hardware/speaker"
	"github.com/ygelfand/echolocal/internal/layout"
	"github.com/ygelfand/echolocal/internal/openclaw/talk"
	sharedcomponent "github.com/ygelfand/libcountertop/pkg/runtime/component"
	"sync"
	"time"
)

type Manager struct {
	generation  uint64
	history     talk.History
	captions    talk.ConversationLog
	uiDirty     bool
	Store       *Store
	Controller  *talk.Controller
	mu          sync.Mutex
	ctx         context.Context
	active      bool
	cancel      context.CancelFunc
	error       bool
	forcedMuted bool
	lastBackend *talk.Session
	lastAudio   *deviceAudio
}

var Get = sync.OnceValue(func() *Manager {
	m := &Manager{Store: load(layout.StateDir + "/openclaw-assistants.json"), Controller: talk.New(), ctx: context.Background()}
	mute.Get().Changed.Listen(func(on bool) {
		if on {
			m.Mute()
		}
	})
	return m
})

func init() {
	component.Register(sharedcomponent.Device, Get, sharedcomponent.Order(33), component.Needs(board.Panel))
}
func (m *Manager) Name() string { return "openclaw talk" }
func (m *Manager) Run(ctx context.Context) error {
	m.mu.Lock()
	m.ctx = ctx
	m.mu.Unlock()
	<-ctx.Done()
	m.Controller.Stop()
	return nil
}
func (m *Manager) Busy() bool { m.mu.Lock(); defer m.mu.Unlock(); return m.active }
func (m *Manager) Stop() bool {
	m.mu.Lock()
	active := m.active
	cancel := m.cancel
	m.mu.Unlock()
	if !active {
		return false
	}
	if cancel != nil {
		cancel()
	}
	m.Controller.Stop()
	return true
}

// Route returns true when consumed, including unavailable OpenClaw targets.
// It NEVER invokes Home Assistant for an OpenClaw activation.
func (m *Manager) Route(slot int, haBusy bool) bool {
	m.mu.Lock()
	if m.active {
		m.mu.Unlock()
		return true
	}
	if !m.Store.Get(slot).OpenClaw {
		m.mu.Unlock()
		return false
	}
	if haBusy {
		m.mu.Unlock()
		return true
	}
	m.active = true
	m.generation++
	generation := m.generation
	m.captions.Begin(generation)
	m.lastBackend, m.lastAudio = nil, nil
	m.error = false
	m.forcedMuted = false
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancel = cancel
	m.mu.Unlock()
	go m.activate(ctx, cancel, slot, generation)
	return true
}

// StartOwned starts only an already locally configured OpenClaw slot.
// It exposes neither target configuration nor operator credentials.
func (m *Manager) StartOwned(slot int, haBusy bool) (uint64, error) {
	m.mu.Lock()
	if slot < 0 || slot > 1 || m.active || haBusy || !m.Store.Get(slot).OpenClaw {
		m.mu.Unlock()
		return 0, errors.New("talk_unavailable")
	}
	m.active = true
	m.generation++
	generation := m.generation
	m.captions.Begin(generation)
	m.lastBackend, m.lastAudio = nil, nil
	m.error, m.forcedMuted = false, false
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancel = cancel
	m.mu.Unlock()
	go m.activate(ctx, cancel, slot, generation)
	return generation, nil
}

// StopOwned cannot stop a newer locally started conversation.
func (m *Manager) StopOwned(generation uint64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.active || m.generation != generation || m.cancel == nil {
		return false
	}
	m.cancel()
	return true
}
func (m *Manager) activate(ctx context.Context, cancel context.CancelFunc, slot int, generation uint64) {
	defer cancel()
	defer func() {
		status := m.Status()
		m.history.Add(talk.ActivationReport{EndedAtMS: time.Now().UnixMilli(), State: status.State, Transport: status.Transport, Captured: status.Captured, InputDropped: status.InputDropped, Decoded: status.Decoded, PlayoutDropped: status.PlayoutDropped, SpeakerUnderruns: status.SpeakerUnderruns, MaxReceiveGapMS: status.MaxReceiveGapMS, MaxPlayoutGapMS: status.MaxPlayoutGapMS})
		m.mu.Lock()
		m.active = false
		m.cancel = nil
		m.mu.Unlock()
		shellfeature.Get().Redraw()
	}()
	sh := shellfeature.Get()
	idle.Get().Wake()
	held := sh.Hold(m.conversationPage())
	defer held.Keep(false)
	v, e := m.Store.Target(slot)
	if e != nil {
		m.setError()
		return
	}
	muted, e := mute.Get().Muted()
	if e != nil || muted {
		m.Mute()
		return
	}
	// One speaker claim owns this entire activation. Preemption ends Talk.
	claim := speaker.Sound().Claim("openclaw-talk", func(claimCtx context.Context, _ *speaker.Player) error {
		select {
		case <-claimCtx.Done():
			// A retiring claim must never cancel the next activation.
			cancel()
		case <-ctx.Done():
		}
		return nil
	})
	_ = claim
	a := newAudio(ctx, func() { m.markOwned(generation, talk.Muted); cancel() })
	a.startTone, a.endTone = v.StartTone, v.EndTone
	bound, bindErr := claw.Get().BindTalk()
	if bindErr != nil {
		a.Close()
		m.setError()
		return
	}
	b := talk.NewSession(bound, v.Context, a)
	b.Provider, b.Model = v.Provider, v.Model
	b.DeviceContext, e = claw.Get().Node.ConversationDeviceContextWithTemplate(v.DeviceContextTemplate)
	if e != nil {
		a.Close()
		m.setError()
		return
	}
	m.mu.Lock()
	m.lastBackend = b
	m.lastAudio = a
	m.mu.Unlock()
	b.Activity = func(state talk.State) { m.activityOwned(generation, state) }
	a.onPlay = b.Playback
	b.UserSpeech = func() { m.userSpeechOwned(generation) }
	b.Caption = func(role, text, id string) { m.captionOwned(generation, role, text, id) }
	b.End = cancel
	b.Failure = func() { m.markOwned(generation, talk.Failed); cancel() }
	if e = m.Controller.Start(ctx, b, time.Duration(v.Timeout)*time.Second); e != nil {
		if ctx.Err() == nil {
			m.setError()
		}
		return
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	var previousState talk.State
	previousMic := false
	for m.Controller.Busy() {
		select {
		case <-ctx.Done():
			m.Controller.Stop()
		case <-ticker.C:
			state := m.state()
			live := microphoneLive(true, a, time.Now().UnixMilli())
			m.mu.Lock()
			dirty := m.uiDirty
			m.uiDirty = false
			m.mu.Unlock()
			// Captions can be long. Do not rebuild the whole page at 10Hz
			// when neither content, lifecycle nor capture status changed.
			if dirty || state != previousState || live != previousMic {
				sh.Redraw()
				previousState, previousMic = state, live
			}
		}
	}
}
func (m *Manager) setError() { m.mu.Lock(); m.error = true; m.mu.Unlock(); shellfeature.Get().Redraw() }
func (m *Manager) state() talk.State {
	m.mu.Lock()
	bad := m.error
	muted := m.forcedMuted
	m.mu.Unlock()
	if muted {
		return talk.Muted
	}
	if bad {
		return talk.Failed
	}
	return m.Controller.State()
}

func (m *Manager) Mute() {
	m.mu.Lock()
	m.forcedMuted = true
	cancel := m.cancel
	m.mu.Unlock()
	m.Controller.Mute()
	if cancel != nil {
		cancel()
	}
}

type Status struct {
	Recent                                                                      []talk.ActivationReport
	Decoded, SilenceSkipped, PlayoutUnderruns, PlayoutDropped, SpeakerUnderruns uint64
	MaxReceiveGapMS, MaxPlayoutGapMS                                            int64
	PeakBufferedFrames                                                          int

	State                                           talk.State
	Active                                          bool
	Transport                                       talk.Diagnostics
	Captured, InputDropped, Played, OutputDisplaced uint64
}

func (m *Manager) Status() Status {
	m.mu.Lock()
	active, b, a := m.active, m.lastBackend, m.lastAudio
	m.mu.Unlock()
	out := Status{State: m.state(), Active: active, Recent: m.history.Snapshot()}
	if b != nil {
		out.Transport = b.Diagnostics()
	}
	if a != nil {
		out.Decoded = a.decoded.Load()
		out.SilenceSkipped = a.silenceSkipped.Load()
		out.MaxReceiveGapMS = a.maxReceiveGapMS.Load()
		out.MaxPlayoutGapMS = a.maxPlayoutGapMS.Load()
		a.playMu.Lock()
		out.PlayoutUnderruns = a.playout.Underruns
		out.PlayoutDropped = a.playout.Dropped
		out.PeakBufferedFrames = a.playout.PeakFrames
		a.playMu.Unlock()
		if now := speaker.Get().Underruns(); now >= a.underrunBase.Load() {
			out.SpeakerUnderruns = now - a.underrunBase.Load()
		}
		out.Captured = a.captured.Load()
		out.InputDropped = a.drops.Load()
		out.Played = a.played.Load()
		out.OutputDisplaced = a.overflows.Load()
	}
	return out
}

func (m *Manager) markOwned(generation uint64, state talk.State) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.active || m.generation != generation {
		return
	}
	if state == talk.Failed {
		m.error = true
	}
	if state == talk.Muted {
		m.forcedMuted = true
	}
}

func (m *Manager) activityOwned(generation uint64, state talk.State) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active && m.generation == generation {
		m.Controller.Activity(state)
	}
}
func (m *Manager) userSpeechOwned(generation uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active && m.generation == generation {
		m.Controller.UserSpeech()
	}
}
