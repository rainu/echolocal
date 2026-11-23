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
