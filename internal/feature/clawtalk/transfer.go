//go:build !board_biscuit

package clawtalk

import (
	"context"
	"errors"
	"sync"
	"time"

	connection "github.com/ygelfand/echolocal/internal/openclaw"
	"github.com/ygelfand/echolocal/internal/openclaw/talk"
	"github.com/ygelfand/libcountertop/pkg/say"
)

type transferRequest struct {
	ctx        context.Context
	cancel     context.CancelFunc
	generation uint64
	backend    *talk.Session
	bound      *connection.TalkBinding
	previous   Slot
	target     Slot
	delivered  chan bool
	announce   func(context.Context, string) error
	open       func(*transferRequest, Slot) error
	stopSetup  func() bool
}

// TransferResult is an acceptance receipt, not a claim that the target opened.
type TransferResult struct {
	Accepted  bool   `json:"accepted"`
	State     string `json:"state"`
	Message   string `json:"message"`
	once      sync.Once
	delivered func(bool)
}

func (r *TransferResult) NodeResultDelivered(ok bool) {
	if r == nil {
		return
	}
	r.once.Do(func() { r.delivered(ok) })
}

// PrepareTransfer acts on the device's current Talk, irrespective of invocation
// origin. The local generation only fences this operation after it is accepted.
func (m *Manager) PrepareTransfer(ctx context.Context, target string) (*TransferResult, error) {
	agent, valid := sessionAgent(target)
	if !valid {
		return nil, errors.New("talk_invalid_session")
	}
	m.mu.Lock()
	if m.transfer != nil {
		m.mu.Unlock()
		return nil, errors.New("talk_transfer_busy")
	}
	b := m.lastBackend
	if !m.active || b == nil || b.Diagnostics().Stage != "ready" {
		m.mu.Unlock()
		return nil, errors.New("talk_not_ready")
	}
	if b.Target == target {
		m.mu.Unlock()
		return nil, errors.New("talk_target_unchanged")
	}
	bound, ok := b.Client.(*connection.TalkBinding)
	if !ok {
		m.mu.Unlock()
		return nil, errors.New("talk_connection_unavailable")
	}
	base := m.ctx
	if base == nil {
		base = context.Background()
	}
	work, cancel := context.WithTimeout(base, 150*time.Second)
	r := &transferRequest{ctx: work, cancel: cancel, generation: m.generation, backend: b, bound: bound,
		previous: m.currentSlot, target: m.currentSlot, delivered: make(chan bool, 1)}
	r.announce, r.open = b.AnnounceHandoff, m.openTransferred
	r.target.Context, r.target.Agent, r.target.Resume, r.target.OpenClaw = target, agent, true, true
	m.transfer, m.transferState, m.transferReason = r, "validating", ""
	m.mu.Unlock()

	// Validate the complete accessible catalogue before any audio/voice teardown.
	lookup, done := context.WithTimeout(ctx, 15*time.Second)
	stop := context.AfterFunc(work, done)
	sessions, err := AutomationSessions(lookup, bound)
	stop()
	done()
	found := false
	for _, s := range sessions {
		if s.ID == target {
			found = true
			break
		}
	}
	if err == nil && !found {
		err = errors.New("talk_invalid_session")
	}
	m.mu.Lock()
	current := m.active && m.generation == r.generation && m.lastBackend == b && work.Err() == nil
	m.mu.Unlock()
	if err == nil && (!current || ctx.Err() != nil) {
		err = errors.New("talk_transfer_cancelled")
	}
	if err != nil {
		m.finishTransfer(r, "failed", err.Error())
		return nil, err
	}
	m.setTransferState(r, "accepted", "")
	go m.runTransfer(r)
	return &TransferResult{Accepted: true, State: "accepted",
		Message:   "Transfer accepted, not completed. Finish this consultation promptly; do not poll or wait for completion within it. Echo announces and reconnects after consultation ends. Inspect talk.status later for completion/recovery.",
		delivered: func(ok bool) { r.delivered <- ok }}, nil
}

// CancelTransfer is invoked on Node disconnect/grant revocation. It never stops
// a completed transfer or a later unrelated activation.
func (m *Manager) CancelTransfer() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.transfer != nil {
		m.transfer.cancel()
	}
}
func (m *Manager) setTransferState(r *transferRequest, state, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.transfer == r {
		m.transferState, m.transferReason = state, reason
	}
}
func (m *Manager) finishTransfer(r *transferRequest, state, reason string) {
	m.mu.Lock()
	if m.transfer == r {
		// Detach setup cancellation only at the successful ownership commit.
		// Disconnect/Stop before this point must still cancel the new backend.
		if r.stopSetup != nil {
			stopped := r.stopSetup()
			if !stopped || r.ctx.Err() != nil {
				state, reason = "cancelled", "talk_transfer_cancelled"
			}
		}
		m.transfer = nil
		m.transferState, m.transferReason = state, reason
	}
	m.mu.Unlock()
	r.cancel()
}

func (m *Manager) runTransfer(r *transferRequest) {
	state, reason := "cancelled", "talk_transfer_cancelled"
	defer func() {
		if errors.Is(r.ctx.Err(), context.Canceled) {
			state, reason = "cancelled", "talk_transfer_cancelled"
		}
		if errors.Is(r.ctx.Err(), context.DeadlineExceeded) {
			state, reason = "failed", "talk_transfer_timeout"
		}
		m.finishTransfer(r, state, reason)
	}()
	timer := time.NewTimer(12 * time.Second)
	defer timer.Stop()
	select {
	case ok := <-r.delivered:
		if !ok {
			reason = "talk_result_delivery_failed"
			return
		}
	case <-timer.C:
		reason = "talk_result_delivery_timeout"
		return
	case <-r.ctx.Done():
		return
	}
	m.mu.Lock()
	current := m.active && m.generation == r.generation && m.lastBackend == r.backend && r.ctx.Err() == nil
	m.mu.Unlock()
	if !current {
		return
	}
	m.setTransferState(r, "confirming", "")
	text := "Say briefly: I am switching our voice conversation to the selected session now."
	if say.Chosen() == "de" {
		text = "Sag kurz auf Deutsch: Ich wechsle unser Sprachgespräch jetzt zur ausgewählten Session."
	}
	confirm, done := context.WithTimeout(r.ctx, 45*time.Second)
	err := r.announce(confirm, text)
	done()
	if err != nil {
		state, reason = "failed", err.Error()
		return
	}
	m.mu.Lock()
	if m.transfer != r || !m.active || m.generation != r.generation || r.ctx.Err() != nil {
		m.mu.Unlock()
		return
	}
	m.transferState = "opening"
	// Only retire the generation captured at acceptance, never a newer Talk.
	if m.cancel != nil {
		m.cancel()
	}
	m.mu.Unlock()
	if err = m.waitTransferRelease(r.ctx); err != nil {
		state, reason = "failed", err.Error()
		return
	}
	if err = r.open(r, r.target); err == nil {
		if r.ctx.Err() != nil {
			return
		}
		state, reason = "completed", ""
		return
	}
	reason = "talk_target_open_failed"
	if r.ctx.Err() != nil {
		return
	}
	m.setTransferState(r, "recovering", reason)
	recovery := r.previous
	// Recovery is announced by the restored voice; never claim the target opened.
	recovery.GreetingPrompt = "Briefly explain that switching the voice session failed and the previous session has been restored."
	if say.Chosen() == "de" {
		recovery.GreetingPrompt = "Erkläre kurz auf Deutsch, dass der Sessionwechsel fehlgeschlagen ist und die bisherige Session wiederhergestellt wurde."
	}
	if err = r.open(r, recovery); err == nil {
		state = "recovered"
		return
	}
	state, reason = "failed", "talk_recovery_failed"
}

// Reservation remains held across close/create and recovery. Every public
// start path checks it; no public stop/start tools are chained.
func (m *Manager) waitTransferRelease(ctx context.Context) error {
	wait, done := context.WithTimeout(ctx, 15*time.Second)
	defer done()
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		m.mu.Lock()
		released := !m.active
		m.mu.Unlock()
		if released {
			return nil
		}
		select {
		case <-wait.Done():
			return errors.New("talk_close_timeout")
		case <-tick.C:
		}
	}
}
func (m *Manager) openTransferred(r *transferRequest, v Slot) error {
	m.mu.Lock()
	if m.transfer != r || m.active || r.ctx.Err() != nil {
		m.mu.Unlock()
		return errors.New("talk_transfer_cancelled")
	}
	base := m.ctx
	if base == nil {
		base = context.Background()
	}
	ctx, cancel := context.WithCancel(base)
	m.active = true
	m.generation++
	gen := m.generation
	m.cancel = cancel
	m.captions.Begin(gen)
	m.lastBackend, m.lastAudio, m.avatarRenderer = nil, nil, nil
	m.error, m.forcedMuted = false, false
	m.mu.Unlock()
	stop := context.AfterFunc(r.ctx, cancel)
	launch := &automationLaunch{bound: r.bound, result: make(chan error, 1)}
	finished := make(chan struct{})
	go func() { defer close(finished); m.activate(ctx, cancel, 0, gen, &v, launch) }()
	select {
	case err := <-launch.result:
		if err != nil {
			stop()
			cancel()
			<-finished
		} else {
			r.stopSetup = stop
		}
		return err
	case <-r.ctx.Done():
		cancel()
		<-finished
		stop()
		return errors.New("talk_transfer_cancelled")
	}
}
