//go:build !board_biscuit

package clawtalk

import (
	"github.com/ygelfand/echolocal/internal/openclaw/talk"
	"testing"
)

func TestTurnIndicatorUsesDeviceIOAndLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name                            string
		state                           talk.State
		active, muted, capture, playing bool
		want                            string
	}{
		{"ready capture", talk.Listening, true, false, true, false, "turn_listening"},
		{"capture stale", talk.Listening, true, false, false, false, "turn_waiting"},
		{"audio prefill", talk.Speaking, true, false, true, false, "turn_waiting"},
		{"playing", talk.Speaking, true, false, true, true, "turn_speaking"},
		{"playback tail", talk.Listening, true, false, true, true, "turn_speaking"},
		{"user interruption drained", talk.Listening, true, false, true, false, "turn_listening"},
		{"provider cleared", talk.Speaking, true, false, true, false, "turn_waiting"},
		{"processing", talk.Thinking, true, false, true, false, "turn_waiting"},
		{"muted capture stale", talk.Muted, true, true, true, false, "turn_muted"},
		{"input mute preserves playback", talk.Muted, true, true, false, true, "turn_speaking"},
		{"reconnect clears stale IO", talk.Connecting, true, false, true, true, "turn_connecting"},
		{"transfer inactive", talk.Listening, false, false, true, true, "turn_disconnected"},
		{"stop clears", talk.Ended, true, false, true, true, "turn_disconnected"},
		{"failure clears", talk.Failed, true, false, true, true, "turn_disconnected"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := turnIndicator(tc.state, tc.active, tc.muted, tc.capture, tc.playing)
			if got.Phase != tc.want || got.Muted != tc.muted {
				t.Fatalf("got %+v, want %s muted=%t", got, tc.want, tc.muted)
			}
		})
	}
}

func TestTurnIndicatorNoActivationCannotClaimListening(t *testing.T) {
	m := &Manager{Controller: talk.New()}
	if got := m.turnIndicator(); got.Phase != "turn_disconnected" {
		t.Fatal(got)
	}
}
