package talk

import (
	"context"
	"errors"
	connection "github.com/ygelfand/echolocal/internal/openclaw"
	"sync/atomic"
	"testing"
	"time"
)

type activityReply struct {
	row connection.SessionActivity
	err error
}
type fakeActivityReader struct {
	replies chan activityReply
	targets chan string
}

func (f fakeActivityReader) ReadSessionActivity(ctx context.Context, target string) (connection.SessionActivity, error) {
	select {
	case f.targets <- target:
	case <-ctx.Done():
		return connection.SessionActivity{}, ctx.Err()
	}
	select {
	case r := <-f.replies:
		return r.row, r.err
	case <-ctx.Done():
		return connection.SessionActivity{}, ctx.Err()
	}
}
func TestActivityNormalToolBackgroundAndTransfer(t *testing.T) {
	s := NewSession(nil, "agent:test:one", &fakeDevice{})
	f := fakeActivityReader{make(chan activityReply), make(chan string, 8)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); s.pollActivity(ctx, f, time.Millisecond) }()
	send := func(id string, working bool, err error) {
		t.Helper()
		select {
		case target := <-f.targets:
			if target != s.Target {
				t.Fatal("wrong target")
			}
		case <-time.After(time.Second):
			t.Fatal("read not issued")
		}
		f.replies <- activityReply{connection.SessionActivity{SessionID: id, Working: working}, err}
	}
	wait := func(want AgentActivity) {
		t.Helper()
		deadline := time.Now().Add(time.Second)
		for s.AgentActivity() != want {
			if time.Now().After(deadline) {
				t.Fatal("activity", s.AgentActivity(), want)
			}
			time.Sleep(time.Millisecond)
		}
	}
	wait(AgentActivity{})
	send("generation1", false, nil)
	wait(AgentActivity{Available: true})
	send("generation1", true, nil)
	wait(AgentActivity{true, true}) // normal agent turn
	// A long tool or child run remains active while playback/capture changes.
	var speech atomic.Int32
	s.Activity = func(State) { speech.Add(1) }
	s.Playback()
	send("generation1", true, nil)
	wait(AgentActivity{true, true})
	send("generation1", false, nil)
	wait(AgentActivity{Available: true}) // complete/error/cancel in metadata
	send("", false, errors.New("disconnected"))
	wait(AgentActivity{})
	send("generation1", true, nil)
	wait(AgentActivity{true, true})
	send("generation2", true, nil) // same key reset must never adopt a new incarnation
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("observer survived replacement")
	}
	wait(AgentActivity{})
	if speech.Load() != 1 {
		t.Fatal("observer emitted speech/activity")
	}
	// A transferred activation owns a different observer; old values are not copied.
	newer := NewSession(nil, "agent:test:two", &fakeDevice{})
	if newer.AgentActivity() != (AgentActivity{}) {
		t.Fatal("transfer inherited activity")
	}
}
func TestActivityFreshnessAndTerminalFences(t *testing.T) {
	s := NewSession(nil, "agent:test:one", &fakeDevice{})
	s.agentActivity.Store(&activitySample{AgentActivity: AgentActivity{true, true}, at: time.Now().Add(-activityFreshness - time.Millisecond)})
	if s.AgentActivity() != (AgentActivity{}) {
		t.Fatal("stale sample shown")
	}
	s.agentActivity.Store(&activitySample{AgentActivity: AgentActivity{true, true}, at: time.Now()})
	s.fail("synthetic")
	if s.AgentActivity() != (AgentActivity{}) {
		t.Fatal("failure retained activity")
	}
	s = NewSession(nil, "agent:test:one", &fakeDevice{})
	f := fakeActivityReader{make(chan activityReply), make(chan string, 2)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); s.pollActivity(ctx, f, time.Millisecond) }()
	<-f.targets
	f.replies <- activityReply{row: connection.SessionActivity{SessionID: "one", Working: true}}
	<-f.targets
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("blocked read ignored cancellation")
	}
	if s.AgentActivity() != (AgentActivity{}) {
		t.Fatal("stop retained activity")
	}
}

func TestVerifiedWorkSuspendsIdleWithoutInventingVoiceState(t *testing.T) {
	c := New()
	if err := c.Start(context.Background(), &backend{}, 30*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	defer c.Stop()
	c.SetAgentWorking(true)
	for _, state := range []State{Listening, Speaking, Thinking} {
		c.Activity(state)
		time.Sleep(60 * time.Millisecond)
		if !c.Busy() || c.State() != state {
			t.Fatal("voice state replaced or work expired", c.State())
		}
	}
	c.Activity(Listening)
	c.SetMuted(true)
	c.SetMuted(false)
	time.Sleep(60 * time.Millisecond)
	if !c.Busy() || c.State() != Listening {
		t.Fatal("unmute armed expiry during work")
	}
	c.SetAgentWorking(false)
	await(t, func() bool { return !c.Busy() })
	if c.State() != Ended {
		t.Fatal("idle observation did not release expiry")
	}
}

// Exercise the provider's raw control channel, not an internal activity setter.
func TestNativeDelegationDrivesBadgeWithoutSessionRun(t *testing.T) {
	for _, public := range []bool{false, true} {
		name := "legacy"
		if public {
			name = "public"
		}
		t.Run(name, func(t *testing.T) {
			s, _, ctx := nativeHarness(t)
			s.nativeEnabled.Store(true)
			s.agentActivity.Store(&activitySample{AgentActivity: AgentActivity{Available: true}, at: time.Now()})
			input := s.controlLink(ctx).Received
			created := `{"type":"delegation.created","item":{"id":"work","target":"client"}}`
			finished := `{"type":"delegation.context.appended","delegation_item_id":"work"}`
			unrelated := `{"type":"delegation.context.appended","delegation_item_id":"other"}`
			if public {
				created = `{"type":"session.delegation.created","delegation":{"id":"work","target":"client"}}`
				finished = `{"type":"session.commentary.appended","delegation_id":"work"}`
				unrelated = `{"type":"session.commentary.appended","delegation_id":"other"}`
			}
			input([]byte(`{"type":"turn.done","turn":{"id":"speech","role":"assistant","transcript":"neutral"}}`))
			eventually(t, func() bool { return s.native.responses.Load() == 1 })
			if s.AgentActivity().Working {
				t.Fatal("speech invented agent work")
			}
			input([]byte(`{"type":"session.delegation.created","delegation":{"id":"not-client","target":"server"}}`))
			input([]byte(`{"type":"session.delegation.created","delegation":{"target":"client"}}`))
			input([]byte(created))
			eventually(t, func() bool { return s.native.delegations.Load() == 1 })
			if got := s.AgentActivity(); got != (AgentActivity{true, true}) {
				t.Fatal("pending native task not shown", got)
			}
			input([]byte(created)) // duplicate must not require a second result
			input([]byte(unrelated))
			input([]byte(`{"type":"turn.done","turn":{"id":"ack","role":"assistant","transcript":"neutral"}}`))
			eventually(t, func() bool { return s.native.responses.Load() == 2 })
			if !s.AgentActivity().Working {
				t.Fatal("acknowledgement or unrelated result cleared work")
			}
			s.agentActivity.Store(nil) // metadata failure cannot erase a live native request
			if !s.AgentActivity().Working {
				t.Fatal("metadata gap hid native work")
			}
			input([]byte(finished))
			eventually(t, func() bool { return s.native.results.Load() == 1 })
			eventually(t, func() bool { return !s.AgentActivity().Working })
			if s.native.delegations.Load() != 1 {
				t.Fatal("duplicate task counted twice")
			}
		})
	}
}

func TestNativeActivitySupersessionAndTerminalFences(t *testing.T) {
	for _, terminal := range []string{"result", "error", "session.closed", "channel.closed", "cancel"} {
		t.Run(terminal, func(t *testing.T) {
			s := NewSession(nil, "agent:synthetic:test", &nativeDevice{fakeDevice: fakeDevice{frames: make(chan []int16, 8)}})
			s.nativeEnabled.Store(true)
			ctx, cancel := context.WithCancel(context.Background())
			go s.nativeLoop(ctx)
			defer func() { cancel(); <-s.native.done }()
			hooks := s.controlLink(ctx)
			input := hooks.Received
			input([]byte(`{"type":"session.delegation.created","delegation":{"id":"old","target":"client"}}`))
			eventually(t, func() bool { return s.native.delegations.Load() == 1 })
			if !s.AgentActivity().Working {
				t.Fatal("old delegation not active")
			}
			input([]byte(`{"type":"session.delegation.created","delegation":{"id":"new","target":"client"}}`))
			input([]byte(`{"type":"session.commentary.appended","delegation_id":"old"}`))
			input([]byte(`{"type":"turn.done","turn":{"id":"barrier","role":"assistant","transcript":"neutral"}}`))
			eventually(t, func() bool { return s.native.responses.Load() == 1 })
			if !s.AgentActivity().Working {
				t.Fatal("late old result cleared new request")
			}
			switch terminal {
			case "result":
				input([]byte(`{"type":"session.commentary.appended","delegation_id":"new"}`))
			case "error":
				input([]byte(`{"type":"error","error":{"type":"server_error","code":"server_error","message":"synthetic"}}`))
			case "session.closed":
				input([]byte(`{"type":"session.closed"}`))
			case "channel.closed":
				hooks.Closed()
			case "cancel":
				cancel()
			}
			eventually(t, func() bool { return !s.AgentActivity().Working })
			if terminal == "result" {
				s.agentActivity.Store(&activitySample{AgentActivity: AgentActivity{true, true}, at: time.Now()})
				if !s.AgentActivity().Working {
					t.Fatal("native result erased independently running work")
				}
			}
		})
	}
}
