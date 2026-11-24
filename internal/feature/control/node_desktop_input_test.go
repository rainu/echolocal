//go:build !board_biscuit

package control

import (
	"context"
	"github.com/ygelfand/libcountertop/pkg/input/textentry"
	"github.com/ygelfand/libcountertop/pkg/input/touch"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDesktopPointerPolicyScaleAndCancel(t *testing.T) {
	var allow atomic.Bool
	allow.Store(true)
	events := make(chan touch.Contact, 32)
	p := newDesktopInput(context.Background(), 640, 400, 1280, 800, allow.Load, func(c touch.Contact) { events <- c }, nil)
	defer p.Close()
	p.Pointer(context.Background(), 1, 639, 399)
	c := <-events
	if c.Phase != touch.Down || c.X != 1279 || c.Y != 799 {
		t.Fatal("wrong scale/phase", c)
	}
	allow.Store(false)
	select {
	case c = <-events:
		if c.Phase != touch.Cancel {
			t.Fatal("revocation clicked")
		}
	case <-time.After(time.Second):
		t.Fatal("held contact not cancelled")
	}
	allow.Store(true)
	p.Pointer(context.Background(), 1, 100, 100)
	p.Pointer(context.Background(), 0, 100, 100)
	select {
	case <-events:
		t.Fatal("held denied contact resurrected")
	default:
	}
	p.Pointer(context.Background(), 1, 200, 100)
	if (<-events).Phase != touch.Down {
		t.Fatal("fresh down")
	}
	p.Pointer(context.Background(), 1, 220, 100)
	if (<-events).Phase != touch.Move {
		t.Fatal("drag")
	}
	p.Pointer(context.Background(), 0, 220, 100)
	if (<-events).Phase != touch.Up {
		t.Fatal("release")
	}
	p.Pointer(context.Background(), 1, 100, 100)
	<-events
	p.Close()
	if (<-events).Phase != touch.Cancel {
		t.Fatal("close clicked")
	}
}
func TestDesktopDeniedHoverBoundsAndWheel(t *testing.T) {
	var allow atomic.Bool
	var mu sync.Mutex
	var events []touch.Contact
	p := newDesktopInput(context.Background(), 640, 400, 1280, 800, allow.Load, func(c touch.Contact) { mu.Lock(); events = append(events, c); mu.Unlock() }, nil)
	defer p.Close()
	p.Pointer(context.Background(), 1, 1, 1)
	p.Pointer(context.Background(), 0, 1, 1)
	allow.Store(true)
	for _, at := range [][2]int{{-1, 0}, {640, 1}, {0, 400}} {
		p.Pointer(context.Background(), 1, at[0], at[1])
		p.Pointer(context.Background(), 0, 0, 0)
	}
	p.Pointer(context.Background(), 0, 2, 2)
	mu.Lock()
	if len(events) != 0 {
		t.Fatal("invalid/denied/hover caused touch")
	}
	mu.Unlock()
	p.Pointer(context.Background(), 16, 300, 200)
	mu.Lock()
	defer mu.Unlock()
	if len(events) != 10 || events[0].Phase != touch.Down || events[9].Phase != touch.Up || events[8].Y >= events[0].Y {
		t.Fatal("wheel did not swipe")
	}
}
func TestDesktopKeyboardPolicyMappingAndHeldReset(t *testing.T) {
	var allow atomic.Bool
	allow.Store(true)
	var edits []textentry.Request
	p := newDesktopInput(context.Background(), 640, 400, 1280, 800, allow.Load, func(touch.Contact) {}, func(_ context.Context, r textentry.Request) error { edits = append(edits, r); return nil })
	defer p.Close()
	p.Key(context.Background(), true, 'x')
	p.Key(context.Background(), false, 'x')
	p.Key(context.Background(), true, 0x010003bb)
	p.Key(context.Background(), false, 0x010003bb)
	p.Key(context.Background(), true, 0xff08)
	p.Key(context.Background(), false, 0xff08)
	p.Key(context.Background(), true, 0xffe3)
	p.Key(context.Background(), true, 'a')
	p.Key(context.Background(), false, 'a')
	p.Key(context.Background(), true, 'v')
	p.Key(context.Background(), false, 'v')
	p.Key(context.Background(), false, 0xffe3)
	if len(edits) != 4 || edits[0].Text != "x" || edits[1].Text != "λ" || edits[2].Key != "Backspace" || edits[3].Key != "SelectAll" {
		t.Fatal("key mapping")
	}
	allow.Store(false)
	p.Key(context.Background(), true, 'z')
	allow.Store(true)
	p.Key(context.Background(), true, 'z')
	if len(edits) != 4 {
		t.Fatal("held denied key repeated")
	}
	p.Key(context.Background(), false, 'z')
	p.Key(context.Background(), true, 'z')
	if len(edits) != 5 {
		t.Fatal("fresh key missing")
	}
}
func TestDesktopWheelPolicyChangeCancelsWithoutRelease(t *testing.T) {
	var allow atomic.Bool
	allow.Store(true)
	var phases []touch.Phase
	p := newDesktopInput(context.Background(), 640, 400, 1280, 800, allow.Load, func(c touch.Contact) {
		phases = append(phases, c.Phase)
		if c.Phase == touch.Move {
			allow.Store(false)
		}
	}, nil)
	defer p.Close()
	p.Pointer(context.Background(), 16, 300, 200)
	p.Close()
	if phases[len(phases)-1] != touch.Cancel {
		t.Fatal("wheel release caused action")
	}
}

func TestDesktopRapidPermissionEpochDropsOldContact(t *testing.T) {
	var epoch atomic.Uint64
	events := make(chan touch.Contact, 16)
	p := newDesktopInput(context.Background(), 640, 400, 1280, 800, func() bool { return true }, func(c touch.Contact) { events <- c }, nil, epoch.Load)
	defer p.Close()
	p.Pointer(context.Background(), 1, 100, 100)
	<-events
	epoch.Add(2) // off/on before the watchdog could run.
	p.Pointer(context.Background(), 1, 101, 101)
	if (<-events).Phase != touch.Cancel {
		t.Fatal("rapid toggle retained old contact")
	}
	p.Pointer(context.Background(), 0, 101, 101)
	select {
	case <-events:
		t.Fatal("old contact released as click")
	default:
	}
	p.Pointer(context.Background(), 1, 100, 100)
	if (<-events).Phase != touch.Down {
		t.Fatal("fresh gesture failed")
	}
}
