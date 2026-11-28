//go:build !board_biscuit

package clawtalk

import (
	"github.com/ygelfand/echolocal/internal/config"
	"github.com/ygelfand/echolocal/internal/hardware/speaker"
	"github.com/ygelfand/echolocal/internal/openclaw/talk"
	"time"
)

// TurnIndicator describes observable device I/O, not provider packet arrival,
// avatar animation or inferred agent work. Muting is input-only.
type TurnIndicator struct {
	Phase string
	Muted bool
}

func turnIndicator(state talk.State, active, muted, capture, playing bool) TurnIndicator {
	cue := TurnIndicator{Phase: "turn_waiting", Muted: muted}
	if !active || state == talk.Ended || state == talk.Failed {
		cue.Phase = "turn_disconnected"
	} else if state == talk.Connecting {
		cue.Phase = "turn_connecting"
	} else if playing {
		cue.Phase = "turn_speaking"
	} else if muted {
		cue.Phase = "turn_muted"
	} else if state == talk.Listening && capture {
		cue.Phase = "turn_listening"
	}
	return cue
}

func (m *Manager) turnIndicator() TurnIndicator {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.Controller.State()
	if m.error {
		state = talk.Failed
	}
	a := m.lastAudio
	live, playing := false, false
	muted := m.forcedMuted
	if a != nil {
		muted = muted || a.inputMuted.Load() || a.hardwareMuted.Load()
		live = microphoneLive(m.active, a, time.Now().UnixMilli())
		// Exclude prefill and post-drain lifecycle grace periods. Only the device
		// voice lane, owned by this activation, qualifies as assistant playback.
		a.playMu.Lock()
		playing = a.ctx.Err() == nil && a.ready.Load() && speaker.Get().Lane(config.StreamVoice).Queued() > 0
		a.playMu.Unlock()
	}
	return turnIndicator(state, m.active, muted, live, playing)
}
