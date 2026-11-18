//go:build !board_biscuit

package clawtalk

import (
	"context"
	"github.com/ygelfand/echolocal/internal/board"
	"github.com/ygelfand/echolocal/internal/component"
	"github.com/ygelfand/echolocal/internal/feature/idle"
	"github.com/ygelfand/echolocal/internal/feature/mute"
	claw "github.com/ygelfand/echolocal/internal/feature/openclaw"
	shellfeature "github.com/ygelfand/echolocal/internal/feature/shell"
	"github.com/ygelfand/echolocal/internal/hardware/speaker"
	"github.com/ygelfand/echolocal/internal/layout"
	"github.com/ygelfand/echolocal/internal/openclaw/talk"
	"github.com/ygelfand/libcountertop/pkg/display/menu"
	"github.com/ygelfand/libcountertop/pkg/display/shell"
	sharedcomponent "github.com/ygelfand/libcountertop/pkg/runtime/component"
	"github.com/ygelfand/libcountertop/pkg/say"
	"sync"
	"time"
)

type Manager struct {
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
	page        *shell.Page
}

var Get = sync.OnceValue(func() *Manager {
	m := &Manager{Store: load(layout.StateDir + "/openclaw-assistants.json"), Controller: talk.New(), ctx: context.Background()}
	m.page = m.conversationPage()
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
	m.error = false
	m.forcedMuted = false
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancel = cancel
	m.mu.Unlock()
	go m.activate(ctx, cancel, slot)
	return true
}
func (m *Manager) activate(ctx context.Context, cancel context.CancelFunc, slot int) {
	defer cancel()
	defer func() { m.mu.Lock(); m.active = false; m.cancel = nil; m.mu.Unlock(); shellfeature.Get().Redraw() }()
	sh := shellfeature.Get()
	idle.Get().Wake()
	held := sh.Hold(m.page)
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
			m.Stop()
		case <-ctx.Done():
		}
		return nil
	})
	_ = claim
	a := newAudio(ctx, m.Mute)
	a.onPlay = func() { m.Controller.Activity(talk.Speaking) }
	bound, bindErr := claw.Get().BindTalk()
	if bindErr != nil {
		a.Close()
		m.setError()
		return
	}
	b := talk.NewSession(bound, v.Context, a)
	m.mu.Lock()
	m.lastBackend = b
	m.lastAudio = a
	m.mu.Unlock()
	b.Activity = m.Controller.Activity
	b.End = func() { m.Stop() }
	b.Failure = m.Controller.Fail
	if e = m.Controller.Start(ctx, b, time.Duration(v.Timeout)*time.Second); e != nil {
		if ctx.Err() == nil {
			m.setError()
		}
		return
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for m.Controller.Busy() {
		select {
		case <-ctx.Done():
			m.Controller.Stop()
		case <-ticker.C:
			sh.Redraw()
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
func (m *Manager) conversationPage() *shell.Page {
	return &shell.Page{
		Title: "OpenClaw",
		Build: func() ([]menu.Row, []func(int)) {
			return []menu.Row{{Label: say.T("clawtalk.state." + string(m.state())), Hint: say.T("clawtalk.end_hint"), Kind: menu.Plain}}, []func(int){nil}
		},
		Actions: []shell.Action{{Label: say.T("clawtalk.end"), MinHeight: 72, Role: shell.Danger, Do: func() { m.Stop() }}},
	}
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
	State                                           talk.State
	Active                                          bool
	Transport                                       talk.Diagnostics
	Captured, InputDropped, Played, OutputDisplaced uint64
}

func (m *Manager) Status() Status {
	m.mu.Lock()
	active, b, a := m.active, m.lastBackend, m.lastAudio
	m.mu.Unlock()
	out := Status{State: m.state(), Active: active}
	if b != nil {
		out.Transport = b.Diagnostics()
	}
	if a != nil {
		out.Captured = a.captured.Load()
		out.InputDropped = a.drops.Load()
		out.Played = a.played.Load()
		out.OutputDisplaced = a.overflows.Load()
	}
	return out
}
