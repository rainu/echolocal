//go:build !board_biscuit

package clawtalk

import (
	"context"
	"github.com/ygelfand/echolocal/internal/openclaw/talk"
	"testing"
	"time"
)

func TestCompactMicrophoneStatusTracksRealCapture(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &deviceAudio{ctx: ctx}
	m := &Manager{lastAudio: a}
	if m.microphoneStatus() != "mic_off" {
		t.Fatal("inactive capture")
	}
	m.active = true
	if m.microphoneStatus() != "mic_wait" {
		t.Fatal("no frames reported as capture")
	}
	a.lastCaptureMS.Store(time.Now().UnixMilli())
	if m.microphoneStatus() != "mic_live" {
		t.Fatal("fresh capture missing")
	}
	m.forcedMuted = true
	if m.microphoneStatus() != "mic_muted" {
		t.Fatal("mute privacy missing")
	}
	m.forcedMuted = false
	a.lastCaptureMS.Store(time.Now().Add(-time.Second).UnixMilli())
	if m.microphoneStatus() != "mic_wait" {
		t.Fatal("stale capture shown as live")
	}
	a.lastCaptureMS.Store(time.Now().UnixMilli())
	cancel()
	if m.microphoneStatus() != "mic_wait" {
		t.Fatal("closed capture shown as live")
	}
}
func TestManagerTitleDoesNotGuessFromConfiguredSlot(t *testing.T) {
	m := &Manager{Controller: talk.New(), currentSlot: Slot{ContextLabel: "Configured but not bound"}}
	if m.sessionTitle() != "" {
		t.Fatal("inactive title guessed")
	}
	m.active = true
	m.lastBackend = talk.NewSession(nil, "agent:synthetic:one", &deviceAudio{})
	if m.sessionTitle() != "" {
		t.Fatal("unobserved title guessed")
	}
}
