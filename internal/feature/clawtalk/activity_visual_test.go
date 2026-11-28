//go:build !board_biscuit

package clawtalk

import (
	"testing"
	"time"
)

func TestActivityDotCycleAndLifecycle(t *testing.T) {
	for step, want := range []int{1, 2, 3, 2, 1, 2, 3, 2, 1} {
		if got := ActivityDotCount(time.Duration(step) * activityDotStep); got != want {
			t.Fatal(step, got, want)
		}
	}
	if ActivityDotCount(-time.Hour) != 1 {
		t.Fatal("negative time")
	}
	now := time.Unix(100, 0)
	renderer := &fakeAvatar{}
	p := &AvatarPage{Renderer: renderer, started: true}
	paints := 0
	p.SetActivityRepaint(func() { paints++ })
	p.TickActivity(now, true)
	paints = 0
	p.TickActivity(now.Add(349*time.Millisecond), true)
	if paints != 0 || p.ActivityDots() != 1 {
		t.Fatal("excess frames before dot change")
	}
	p.TickActivity(now.Add(350*time.Millisecond), true)
	if paints != 1 || p.ActivityDots() != 2 {
		t.Fatal("dots did not advance")
	}
	p.TickActivity(now.Add(101*time.Second), true)
	if paints != 2 {
		t.Fatal("stalled timer replayed frames")
	}
	p.ToggleActivityMotion()
	paints = 0
	for i := 0; i < 10; i++ {
		p.TickActivity(now.Add(time.Duration(i)*time.Second), true)
	}
	if paints != 0 || p.ActivityDots() != 3 {
		t.Fatal("static mode animates")
	}
	p.ToggleActivityMotion()
	p.TickActivity(now, true)
	if p.ActivityDots() != 1 {
		t.Fatal("resume did not start cleanly")
	}
	p.TickActivity(now, false)
	paints = 0
	for i := 0; i < 10; i++ {
		p.TickActivity(now.Add(time.Duration(i)*time.Second), false)
	}
	if paints != 0 {
		t.Fatal("idle animates")
	}
	p.Close()
	p.SetActivityRepaint(func() { paints++ })
	p.TickActivity(now, true)
	p.TickActivity(now.Add(time.Second), true)
	if paints != 0 {
		t.Fatal("covered page animates")
	}
	p.Finish()
	p.SetActivityRepaint(func() { paints++ })
	p.TickActivity(now.Add(2*time.Second), true)
	if paints != 0 {
		t.Fatal("finished page animates")
	}
	newer := &AvatarPage{}
	if newer.ActivityPaused() || newer.ActivityDots() != 1 {
		t.Fatal("new call inherited old presentation")
	}
}
