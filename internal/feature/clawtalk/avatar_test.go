//go:build !board_biscuit

package clawtalk

import (
	"github.com/ygelfand/echolocal/internal/avatar"
	shellfeature "github.com/ygelfand/echolocal/internal/feature/shell"
	sharedshell "github.com/ygelfand/libcountertop/pkg/display/shell"
	"sync"
	"testing"
	"time"
)

func TestStandaloneAvatarReleaseRestoresPreviousScreen(t *testing.T) {
	s := shellfeature.Get()
	s.Close()
	defer s.Close()
	previous := &sharedshell.Page{Title: "previous"}
	s.Push(previous)
	renderer := avatar.New(t.TempDir(), nil, func() bool { return false }, func() {})
	page := &AvatarPage{CaptionPage: &CaptionPage{&sharedshell.Page{Title: "Talk"}}, Renderer: renderer}
	held := s.Hold(page)
	if s.Top() != page {
		t.Fatal("not shown")
	}
	s.Pop()
	if s.Top() != page {
		t.Fatal("explicit stop control must own the activation")
	}
	page.Finish()
	held.Release()
	if s.Top() != previous || !page.closed {
		t.Fatal("temporary surface leaked")
	}
	// A queued first GUI frame after teardown must not launch a new renderer.
	page.Start(avatar.Viewport{X: 640, Y: 80, Size: 640})
	time.Sleep(30 * time.Millisecond)
	if renderer.Snapshot().Running {
		t.Fatal("late render start")
	}
}

type fakeAvatar struct {
	mu     sync.Mutex
	state  avatar.State
	starts int
}

func (f *fakeAvatar) Snapshot() avatar.State      { f.mu.Lock(); defer f.mu.Unlock(); return f.state }
func (f *fakeAvatar) Start()                      { f.mu.Lock(); defer f.mu.Unlock(); f.starts++; f.state.Running = true }
func (f *fakeAvatar) Stop()                       { f.mu.Lock(); defer f.mu.Unlock(); f.state.Running = false }
func (f *fakeAvatar) SetViewport(avatar.Viewport) {}
func TestAvatarTemporaryCoverResumesButFinishedPageCannotRestart(t *testing.T) {
	renderer := &fakeAvatar{}
	p := &AvatarPage{Renderer: renderer}
	wait := func(want int) {
		t.Helper()
		end := time.Now().Add(time.Second)
		for time.Now().Before(end) {
			renderer.mu.Lock()
			n := renderer.starts
			renderer.mu.Unlock()
			if n == want {
				return
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatal("renderer start count", want)
	}
	v := avatar.Viewport{X: 640, Y: 80, Size: 640}
	p.Start(v)
	wait(1)
	p.Start(v)
	time.Sleep(10 * time.Millisecond)
	wait(1)
	p.Close()
	p.Start(v)
	wait(2)
	p.Finish()
	p.Start(v)
	time.Sleep(20 * time.Millisecond)
	wait(2)
	if renderer.Snapshot().Running {
		t.Fatal("finished surface retained")
	}
}
