package talk

import (
	"context"
	"errors"
	connection "github.com/ygelfand/echolocal/internal/openclaw"
	"testing"
	"time"
)

func TestBoundSessionTitleRenameMissingErrorAndReplacement(t *testing.T) {
	s := NewSession(nil, "agent:synthetic:one", &fakeDevice{})
	f := fakeActivityReader{make(chan activityReply), make(chan string)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); s.pollActivity(ctx, f, time.Millisecond) }()
	send := func(id, title string, err error) {
		t.Helper()
		select {
		case key := <-f.targets:
			if key != s.Target {
				t.Fatal("wrong target")
			}
		case <-time.After(time.Second):
			t.Fatal("no metadata read")
		}
		f.replies <- activityReply{row: connection.SessionActivity{SessionID: id, Title: title}, err: err}
	}
	wait := func(want string) {
		t.Helper()
		deadline := time.Now().Add(time.Second)
		for s.SessionTitle() != want {
			if time.Now().After(deadline) {
				t.Fatal("wrong title", s.SessionTitle(), want)
			}
			time.Sleep(time.Millisecond)
		}
	}
	send("one", "Original", nil)
	wait("Original")
	send("one", "Renamed", nil)
	wait("Renamed")
	send("one", "", nil)
	wait("")
	send("one", "Recovered", nil)
	wait("Recovered")
	send("", "", errors.New("disconnected"))
	wait("")
	send("one", "Recovered again", nil)
	wait("Recovered again")
	send("replaced", "Wrong incarnation", nil)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("observer retained replacement")
	}
	wait("")
	if newer := s.Replacement(&fakeDevice{}); newer.SessionTitle() != "" {
		t.Fatal("reconnect inherited old title")
	}
	if transferred := NewSession(nil, "agent:synthetic:two", &fakeDevice{}); transferred.SessionTitle() != "" {
		t.Fatal("transfer inherited old title")
	}
}
func TestSessionTitleStaleFailureCloseFences(t *testing.T) {
	for _, terminal := range []string{"stale", "failure", "close"} {
		s := NewSession(nil, "agent:synthetic:one", &fakeDevice{})
		s.agentActivity.Store(&activitySample{title: "Synthetic", at: time.Now()})
		if s.SessionTitle() != "Synthetic" {
			t.Fatal("fresh title missing")
		}
		switch terminal {
		case "stale":
			s.agentActivity.Store(&activitySample{title: "Synthetic", at: time.Now().Add(-activityFreshness - time.Second)})
		case "failure":
			s.fail("synthetic")
		case "close":
			s.activityStopped.Store(true) // Close's synchronous terminal fence.
		}
		if s.SessionTitle() != "" {
			t.Fatal("terminal title retained", terminal)
		}
	}
}
