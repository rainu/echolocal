//go:build !board_biscuit

package clawtalk

import (
	"github.com/ygelfand/echolocal/internal/avatar"
	shellfeature "github.com/ygelfand/echolocal/internal/feature/shell"
	sharedshell "github.com/ygelfand/libcountertop/pkg/display/shell"
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
