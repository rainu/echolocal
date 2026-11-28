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
