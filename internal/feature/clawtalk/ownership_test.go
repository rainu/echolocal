//go:build !board_biscuit

package clawtalk

import (
	"context"
	"github.com/ygelfand/echolocal/internal/openclaw/talk"
	"testing"
	"time"
)

func TestRetiredProviderCannotChangeNextActivationState(t *testing.T) {
	m := &Manager{active: true, generation: 2}
	m.markOwned(1, talk.Failed)
	m.markOwned(1, talk.Muted)
	if m.error || m.forcedMuted {
		t.Fatal("old callback changed current call")
	}
	m.markOwned(2, talk.Failed)
	if !m.error {
		t.Fatal("current failure not reported")
	}
}

type ownedBackend struct{}

func (ownedBackend) Open(context.Context) error { return nil }
func (ownedBackend) Close() error               { return nil }
func TestRetiredActivityCannotOverwriteCurrentConversation(t *testing.T) {
	c := talk.New()
	if e := c.Start(context.Background(), ownedBackend{}, time.Hour); e != nil {
		t.Fatal(e)
	}
	defer c.Stop()
	m := &Manager{active: true, generation: 2, Controller: c}
	m.activityOwned(1, talk.Thinking)
	if c.State() != talk.Listening {
		t.Fatal("retired activity changed current call")
	}
	m.activityOwned(2, talk.Thinking)
	m.userSpeechOwned(1)
	if c.State() != talk.Thinking {
		t.Fatal("retired speech reset current activity")
	}
}

func TestMuteRetainsOwnedActivationAndIgnoresRetiredEdges(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := talk.New()
	if e := c.Start(ctx, ownedBackend{}, time.Hour); e != nil {
		t.Fatal(e)
	}
	defer c.Stop()
	a := &deviceAudio{ctx: ctx}
	b := talk.NewSession(nil, "agent:synthetic:context", a)
	m := &Manager{active: true, generation: 2, Controller: c, cancel: cancel, lastAudio: a, lastBackend: b}
	m.setMutedOwned(1, true)
	if m.forcedMuted {
		t.Fatal("retired mute changed current call")
	}
	m.SetMuted(true)
	if !m.Busy() || !c.Busy() || ctx.Err() != nil || m.state() != talk.Muted || !a.inputMuted.Load() {
		t.Fatal("mute ended call")
	}
	m.activityOwned(2, talk.Speaking)
	m.setMutedOwned(1, false)
	if m.state() != talk.Muted {
		t.Fatal("retired unmute changed state")
	}
	m.SetMuted(false)
	if !m.Busy() || !c.Busy() || ctx.Err() != nil || m.state() != talk.Speaking || a.inputMuted.Load() {
		t.Fatal("unmute lost call")
	}
	if !m.StopOwned(2) || ctx.Err() == nil {
		t.Fatal("dedicated hangup stopped working")
	}
}

func TestMuteOutsideTalkDoesNotStartOrRetainMutedTalkState(t *testing.T) {
	m := &Manager{Controller: talk.New()}
	m.SetMuted(true)
	m.SetMuted(false)
	if m.Busy() || m.Controller.Busy() || m.state() != talk.Ended {
		t.Fatal("inactive mute changed Talk")
	}
}
