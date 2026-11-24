//go:build !board_biscuit

package clawtalk

import (
	"context"
	"errors"
	"github.com/ygelfand/echolocal/internal/avatar"
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
	"github.com/ygelfand/libcountertop/pkg/say"
	"strings"
	"sync"
	"time"
)

type Manager struct {
	generation     uint64
	history        talk.History
	captions       talk.ConversationLog
	uiDirty        bool
	Store          *Store
	Controller     *talk.Controller
	mu             sync.Mutex
	ctx            context.Context
	active         bool
	cancel         context.CancelFunc
	error          bool
	forcedMuted    bool
	lastBackend    *talk.Session
	lastAudio      *deviceAudio
	avatarRenderer *avatar.Controller
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
	defer m.mu.Unlock()
	if !m.active {
		return false
	}
	if m.cancel != nil {
		m.cancel()
	}
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
	m.avatarRenderer = nil
	m.error = false
	m.forcedMuted = false
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancel = cancel
	m.mu.Unlock()
	go m.activate(ctx, cancel, slot, generation, nil, nil)
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
	m.avatarRenderer = nil
	m.error, m.forcedMuted = false, false
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancel = cancel
	m.mu.Unlock()
	go m.activate(ctx, cancel, slot, generation, nil, nil)
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
func (m *Manager) activate(ctx context.Context, cancel context.CancelFunc, slot int, generation uint64, selected *Slot, launch *automationLaunch) {
	startErr := errors.New("talk_start_failed")
	defer func() { launch.notify(startErr) }()
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
	if ctx.Err() != nil {
		return
	}
	muted, e := mute.Get().Muted()
	if e != nil || muted {
		startErr = errors.New("talk_microphone_muted")
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
	sink := &deviceFeedback{progressChoice: m.Store.Get(slot).ProgressTone}
	feedback := beginFeedback(ctx, sink, 900*time.Millisecond)
	defer func() {
		m.mu.Lock()
		failed, muted := m.error, m.forcedMuted
		m.mu.Unlock()
		// Only this generation may determine its setup outcome.
		feedback.Finish(failed && !muted)
	}()
	sh := shellfeature.Get()
	idle.Get().Wake()
	if selected == nil || launch != nil {
		page := m.standalonePage()
		held := sh.Hold(page)
		defer func() {
			if p, ok := page.(*AvatarPage); ok {
				p.Finish()
			}
			held.Release()
		}()
	}
	var v Slot
	if selected == nil {
		v, e = m.Store.Target(slot)
	} else {
		v = *selected
	}
	if e != nil {
		if ctx.Err() == nil {
			m.setError()
		}
		return
	}
	sink.readyChoice = v.StartTone
	a := newAudio(ctx, func() { m.markOwned(generation, talk.Muted); cancel() })
	a.startTone, a.endTone = v.StartTone, v.EndTone
	a.feedback = feedback
	bound, bindErr := claw.Get().BindTalk()
	if launch != nil {
		bound, bindErr = launch.bound, nil
	}
	if bindErr != nil {
		a.Close()
		if ctx.Err() == nil {
			m.setError()
		}
		return
	}
	b := talk.NewSession(bound, v.Context, a)
	b.Provider, b.Model = v.Provider, v.Model
	b.Greeting = v.GreetingText(say.Chosen())
	b.Ending = v.Ending.Defaults(say.Chosen())
	b.DeviceContext, e = claw.Get().Node.ConversationDeviceContextWithTemplate(v.DeviceContextTemplate)
	if e != nil {
		a.Close()
		if ctx.Err() == nil {
			m.setError()
		}
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
	b.PartialCaption = func(role, delta string) {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.active && m.generation == generation {
			m.captions.Delta(generation, role, delta)
			m.uiDirty = true
		}
	}
	b.ClearPartial = func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.captions.DropPending(generation)
		m.uiDirty = true
	}
	b.End = cancel
	b.Failure = func() {
		if ctx.Err() == nil {
			m.markOwned(generation, talk.Failed)
			cancel()
		}
	}
	if e = m.Controller.Start(ctx, b, time.Duration(v.Timeout)*time.Second); e != nil {
		reason := b.Diagnostics().Reason
		if strings.HasPrefix(reason, "rpc_") || reason == "talk_connection_changed" || reason == "talk_control_unavailable" || reason == "control_stream_closed" {
			startErr = errors.New("talk_connection_unavailable")
		}
		if ctx.Err() != nil {
			startErr = errors.New("talk_start_cancelled")
		}
		if ctx.Err() == nil {
			m.setError()
		}
		return
	}
	launch.notify(nil)
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
	Avatar                                                                      *avatar.State
	OutputMouth                                                                 float64
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
	active, b, a, renderer := m.active, m.lastBackend, m.lastAudio, m.avatarRenderer
	m.mu.Unlock()
	out := Status{State: m.state(), Active: active, Recent: m.history.Snapshot()}
	if renderer != nil {
		s := renderer.Snapshot()
		out.Avatar = &s
		out.OutputMouth = speaker.Get().VoiceLevel()
	}
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

// StartSession uses an existing selected session without rewriting assistant
// slots, generating a new chat, or taking the app out of the foreground.
func (m *Manager) StartSession(key string, haBusy bool) (uint64, error) {
	parts := strings.SplitN(key, ":", 3)
	if len(parts) != 3 || parts[0] != "agent" || parts[1] == "" || parts[2] == "" {
		return 0, errors.New("talk_target_missing")
	}
	m.mu.Lock()
	if m.active || haBusy {
		m.mu.Unlock()
		return 0, errors.New("talk_unavailable")
	}
	v := m.Store.Get(0)
	v.Context = key
	v.Agent = parts[1]
	v.OpenClaw = true
	v.Resume = true
	m.active = true
	m.generation++
	generation := m.generation
	m.captions.Begin(generation)
	m.lastBackend, m.lastAudio = nil, nil
	m.avatarRenderer = nil
	m.error, m.forcedMuted = false, false
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancel = cancel
	m.mu.Unlock()
	go m.activate(ctx, cancel, 0, generation, &v, nil)
	return generation, nil
}
