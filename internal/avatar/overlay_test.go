package avatar

import (
	"context"
	"image"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type overlayPipe struct{ lines chan string }

func (p overlayPipe) Write(b []byte) (int, error) {
	if strings.HasPrefix(string(b), "V ") {
		p.lines <- string(b)
	}
	return len(b), nil
}
func (p overlayPipe) Close() error { return nil }

func TestTalkOverlayFeedUpdatesAndClearsWithoutRestart(t *testing.T) {
	c := New(t.TempDir(), nil, func() bool { return false }, func() {})
	c.EnableTalkLayout(nil)
	var shown atomic.Bool
	shown.Store(true)
	c.SetOverlay(func() image.Rectangle {
		if shown.Load() {
			return image.Rect(1100, 12, 1268, 788)
		}
		return image.Rectangle{}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := overlayPipe{lines: make(chan string, 32)}
	done := make(chan struct{})
	go func() { defer close(done); c.feed(ctx, 0, p, cancel) }()
	next := func(want string) {
		t.Helper()
		timeout := time.After(time.Second)
		for {
			select {
			case got := <-p.lines:
				if got == want {
					return
				}
			case <-timeout:
				t.Fatal("missing overlay command", want)
			}
		}
	}
	next("V 1100 12 168 776\n")
	c.SetTalkMode(true)
	shown.Store(false)
	next("V 0 0 0 0\n")
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("feed did not stop")
	}
}

type activityPipe struct{ lines chan string }

func (p activityPipe) Write(b []byte) (int, error) {
	if strings.HasPrefix(string(b), "W ") {
		p.lines <- string(b)
	}
	return len(b), nil
}
func (p activityPipe) Close() error { return nil }
func TestActivityApertureScalarFeedClearsWithoutRestart(t *testing.T) {
	c := New(t.TempDir(), nil, func() bool { return false }, func() {})
	c.EnableTalkLayout(nil)
	var working atomic.Bool
	working.Store(true)
	c.SetAgentWorking(working.Load)
	p := activityPipe{make(chan string, 32)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); c.feed(ctx, 0, p, cancel) }()
	next := func(want string) {
		t.Helper()
		timeout := time.After(time.Second)
		for {
			select {
			case got := <-p.lines:
				if got == want {
					return
				}
			case <-timeout:
				t.Fatal("missing scalar", want)
			}
		}
	}
	next("W 1\n")
	working.Store(false)
	next("W 0\n")
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("feed pinned")
	}
}
