//go:build !board_biscuit

package clawtalk

import (
	"context"
	"errors"
	"github.com/ygelfand/echolocal/internal/avatar"
	"github.com/ygelfand/echolocal/internal/board"
	"github.com/ygelfand/echolocal/internal/component"
	"github.com/ygelfand/echolocal/internal/feature/idle"
	"github.com/ygelfand/echolocal/internal/feature/mictest"
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
	transfer       *transferRequest
	transferState  string
	transferReason string
	currentSlot    Slot
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
	recovery       *talk.Recovery
	lastAudio      *deviceAudio
	avatarRenderer *avatar.Controller
}

var Get = sync.OnceValue(func() *Manager {
	m := &Manager{Store: load(layout.StateDir + "/openclaw-assistants.json"), Controller: talk.New(), ctx: context.Background()}
	mute.Get().Changed.Listen(m.SetMuted)
	return m
})

func init() {
	mictest.PauseWhen(func() bool { return Get().Busy() })
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
func (m *Manager) Busy() bool { m.mu.Lock(); defer m.mu.Unlock(); return m.active || m.transfer != nil }
func (m *Manager) Stop() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.transfer != nil {
		m.transfer.cancel()
	}
	if !m.active {
		return m.transfer != nil
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
	if mictest.Active() || m.active || m.transfer != nil {
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
	m.recovery = nil
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
	if mictest.Active() || slot < 0 || slot > 1 || m.active || m.transfer != nil || haBusy || !m.Store.Get(slot).OpenClaw {
		m.mu.Unlock()
		return 0, errors.New("talk_unavailable")
	}
	m.active = true
	m.generation++
	generation := m.generation
	m.captions.Begin(generation)
	m.lastBackend, m.lastAudio = nil, nil
	m.recovery = nil
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
	if m.transfer != nil && m.transfer.generation == generation {
		m.transfer.cancel()
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
		// Natural end/provider failure while waiting for confirmation cancels
		// the pending command. Only the deliberate retirement keeps it alive.
		if m.transfer != nil && m.transfer.generation == generation && m.transferState != "opening" {
			m.transfer.cancel()
		}
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
	var activityPage *AvatarPage
	if selected == nil || launch != nil {
		page := m.standalonePage()
		activityPage, _ = page.(*AvatarPage)
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
	bound, bindErr := claw.Get().BindTalk()
	if launch != nil {
		bound, bindErr = launch.bound, nil
	}
	if bindErr != nil {
		if ctx.Err() == nil {
			m.setError()
		}
		return
	}
	deviceContext, err := claw.Get().Node.ConversationDeviceContextWithTemplate(v.DeviceContextTemplate)
	if err != nil {
		if ctx.Err() == nil {
			m.setError()
		}
		return
	}
	ending := v.Ending.Defaults(say.Chosen())
	greeting := v.GreetingText(say.Chosen())
	notice := newRecoveryFeedback(ctx, &deviceFeedback{}, 2*time.Second)
	defer notice.Close()
	recovery := &talk.Recovery{}
	m.mu.Lock()
	m.currentSlot = v
	m.recovery = recovery
	m.mu.Unlock()
	recovery.Factory = func(attempt context.Context, retry bool, failed, ended func()) (talk.RecoveryBackend, error) {
		a := newAudio(attempt, nil)
		a.startTone, a.endTone = v.StartTone, v.EndTone
		a.recovering = retry
		if retry {
			a.recoveryNotice = notice
		}
		if !retry {
			a.feedback = feedback
		}
		b := talk.NewSession(bound, v.Context, a)
		b.Provider, b.Model = v.Provider, v.Model
		if !retry {
			b.Greeting = greeting
		}
		b.Ending = ending
		b.DeviceContext = deviceContext
		if retry {
			m.mu.Lock()
			previous := m.lastBackend
			m.mu.Unlock()
			if previous != nil {
				b = previous.Replacement(a)
			}
		}
		// Identity and attempt context fence callbacks across replacement transports
		// inside the same logical generation, not just across separate Talks.
		owned := func(fn func()) {
			m.mu.Lock()
			defer m.mu.Unlock()
			if m.active && m.generation == generation && m.lastBackend == b && attempt.Err() == nil {
				fn()
			}
		}
		a.onMute = func(on bool) { owned(func() { m.setMutedLocked(on) }) }
		b.Activity = func(state talk.State) { owned(func() { m.Controller.Activity(state) }) }
		a.onPlay = b.Playback
		b.UserSpeech = func() { owned(func() { m.Controller.UserSpeech() }) }
		b.Caption = func(role, text, id string) {
			owned(func() {
				m.captions.Append(generation, role, text, id)
				m.uiDirty = true
			})
		}
		b.PartialCaption = func(role, delta string) {
			owned(func() {
				m.captions.Delta(generation, role, delta)
				m.uiDirty = true
			})
		}
		b.ClearPartial = func() { owned(func() { m.captions.DropPending(generation); m.uiDirty = true }) }
		b.End = ended
		b.Failure = func() {
			if b.RecoverableFailure() {
				a.silentClose.Store(true)
			}
			failed()
		}
		m.mu.Lock()
		m.lastBackend, m.lastAudio = b, a
		b.SetInputMuted(m.forcedMuted)
		a.inputMuted.Store(m.forcedMuted)
		m.captions.DropPending(generation)
		m.uiDirty = true
		m.mu.Unlock()
		return b, nil
	}
	recovery.State = func(state talk.State) {
		if state == talk.Connecting {
			notice.Lost()
		}
		m.activityOwned(generation, state)
		sh.Redraw()
	}
	recovery.Ended = cancel
	recovery.Failed = func() {
		if ctx.Err() == nil {
			m.markOwned(generation, talk.Failed)
			notice.Fail()
			cancel()
		}
	}
	recovery.Opened = func() { launch.notify(nil) }
	if e = m.Controller.Start(ctx, recovery, time.Duration(v.Timeout)*time.Second); e != nil {
		m.mu.Lock()
		b := m.lastBackend
		m.mu.Unlock()
		reason := ""
		if b != nil {
			reason = b.Diagnostics().Reason
		}
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
	m.mu.Lock()
	m.Controller.SetMuted(m.forcedMuted)
	m.mu.Unlock()
	launch.notify(nil)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	var previousState talk.State
	var previousActivity talk.AgentActivity
	var previousTitle string
	previousMic := false
	for m.Controller.Busy() {
		select {
		case <-ctx.Done():
			m.Controller.Stop()
		case <-ticker.C:
			state := m.state()
			m.mu.Lock()
			a := m.lastAudio
			m.mu.Unlock()
			live := microphoneLive(true, a, time.Now().UnixMilli())
			m.mu.Lock()
			dirty := m.uiDirty
			m.uiDirty = false
			m.mu.Unlock()
			// Captions can be long. Do not rebuild the whole page at 10Hz
			// when neither content, lifecycle nor capture status changed.
			activity := m.agentActivity()
			title := m.sessionTitle()
			m.Controller.SetAgentWorking(activity.Working)
			if activityPage != nil {
				activityPage.TickActivity(time.Now(), activity.Working)
			}
			if dirty || state != previousState || live != previousMic || activity != previousActivity || title != previousTitle {
				previousTitle = title
				previousActivity = activity
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
	muted := m.active && m.forcedMuted
	m.mu.Unlock()
	if muted {
		return talk.Muted
	}
	if bad {
		return talk.Failed
	}
	return m.Controller.State()
}

// Mute is input-only. The dedicated Stop/StopOwned path still ends Talk.
func (m *Manager) Mute() { m.SetMuted(true) }
func (m *Manager) SetMuted(on bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setMutedLocked(on)
}
func (m *Manager) setMutedOwned(generation uint64, on bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.active || m.generation != generation {
		return
	}
	m.setMutedLocked(on)
}
func (m *Manager) setMutedLocked(on bool) {
	if !m.active {
		return
	}
	m.forcedMuted = on
	if m.lastBackend != nil {
		m.lastBackend.SetInputMuted(on)
	}
	if m.lastAudio != nil {
		m.lastAudio.inputMuted.Store(on)
		m.lastAudio.lastCaptureMS.Store(0)
	}
	m.Controller.SetMuted(on)
	m.uiDirty = true
}

type Status struct {
	AgentActivity                                                               talk.AgentActivity
	Avatar                                                                      *avatar.State
	OutputMouth                                                                 float64
	Recent                                                                      []talk.ActivationReport
	Decoded, SilenceSkipped, PlayoutUnderruns, PlayoutDropped, SpeakerUnderruns uint64
	MaxReceiveGapMS, MaxPlayoutGapMS                                            int64
	PeakBufferedFrames                                                          int

	Recovery                                        talk.RecoveryStatus
	Transfer, TransferReason                        string
	State                                           talk.State
	Active                                          bool
	Transport                                       talk.Diagnostics
	Captured, InputDropped, Played, OutputDisplaced uint64
}

func (m *Manager) Status() Status {
	m.mu.Lock()
	active, b, a, renderer := m.active || m.transfer != nil, m.lastBackend, m.lastAudio, m.avatarRenderer
	transferState, transferReason := m.transferState, m.transferReason
	recovery := m.recovery
	m.mu.Unlock()
	out := Status{AgentActivity: m.agentActivity(), State: m.state(), Active: active, Transfer: transferState, TransferReason: transferReason, Recent: m.history.Snapshot()}
	if recovery != nil {
		out.Recovery = recovery.Status()
	}
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
	if mictest.Active() || m.active || m.transfer != nil || haBusy {
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
	m.recovery = nil
	m.avatarRenderer = nil
	m.error, m.forcedMuted = false, false
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancel = cancel
	m.mu.Unlock()
	go m.activate(ctx, cancel, 0, generation, &v, nil)
	return generation, nil
}

// InterruptTransport is intentionally local-control-only, not a Node command.
func (m *Manager) InterruptTransport() error {
	m.mu.Lock()
	b := m.lastBackend
	ready := m.active && b != nil && m.recovery != nil && m.recovery.Status().State == "connected"
	m.mu.Unlock()
	if !ready {
		return errors.New("talk_not_ready")
	}
	return b.InterruptTransport()
}

// InterruptNativeProvider is local-control-only with one fixed diagnostic
// error; it accepts no target or payload and is never advertised as a Node RPC.
func (m *Manager) InterruptNativeProvider() error {
	m.mu.Lock()
	b := m.lastBackend
	ready := m.active && b != nil && m.recovery != nil && m.recovery.Status().State == "connected"
	m.mu.Unlock()
	if !ready {
		return errors.New("talk_not_ready")
	}
	return b.InterruptNativeProvider()
}
