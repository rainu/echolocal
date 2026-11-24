//go:build !board_biscuit

package control

import (
	"context"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/ygelfand/echolocal/internal/feature/frontend"
	featureshell "github.com/ygelfand/echolocal/internal/feature/shell"
	"github.com/ygelfand/echolocal/internal/hardware/display"
	hwtouch "github.com/ygelfand/echolocal/internal/hardware/touch"
	connection "github.com/ygelfand/echolocal/internal/openclaw"
	"github.com/ygelfand/libcountertop/pkg/input/textentry"
	"github.com/ygelfand/libcountertop/pkg/input/touch"
)

type desktopEpochKey struct{}

const desktopContactID = -1 << 29 // Separate from hardware, harness and web-desktop IDs.

// desktopInput owns one synthetic finger. Keys are atomic editor strokes: no OS
// key-down state, shortcuts, clipboard access or arbitrary command execution.
type desktopInput struct {
	generation   func() uint64
	epoch        uint64
	mu           sync.Mutex
	ctx          context.Context
	cancel       context.CancelFunc
	allowed      func() bool
	deliver      func(touch.Contact)
	keyboard     func(context.Context, textentry.Request) error
	sw, sh, w, h int
	held         bool
	closed       bool
	blocked      bool
	buttons      byte
	x, y         int
	last         time.Time
	keys         map[uint32]bool
	denied       map[uint32]bool
}

func newDesktopInput(ctx context.Context, sw, sh, w, h int, allowed func() bool, deliver func(touch.Contact), keyboard func(context.Context, textentry.Request) error, epochs ...func() uint64) *desktopInput {
	ctx, cancel := context.WithCancel(ctx)
	p := &desktopInput{ctx: ctx, cancel: cancel, sw: sw, sh: sh, w: w, h: h, allowed: allowed, deliver: deliver, keyboard: keyboard, keys: map[uint32]bool{}, denied: map[uint32]bool{}}
	if len(epochs) > 0 {
		p.generation = epochs[0]
		p.epoch = p.generation()
	}
	go func() {
		tick := time.NewTicker(50 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				p.Close()
				return
			case <-tick.C:
				p.mu.Lock()
				p.check()
				p.mu.Unlock()
			}
		}
	}()
	return p
}
func (p *desktopInput) emit(phase touch.Phase) {
	p.deliver(touch.Contact{ID: desktopContactID, X: p.x, Y: p.y, Phase: phase, At: time.Now()})
}
func (p *desktopInput) clear() {
	if p.held {
		p.emit(touch.Cancel)
		p.held = false
	}
	if p.buttons&1 != 0 {
		p.blocked = true
	}
	for k := range p.keys {
		if len(p.denied) < 64 {
			p.denied[k] = true
		}
	}
	p.keys = map[uint32]bool{}
}
func (p *desktopInput) check() bool {
	if p.generation != nil {
		if epoch := p.generation(); epoch != p.epoch {
			p.clear()
			p.epoch = epoch
		}
	}
	ok := !p.closed && p.ctx.Err() == nil && p.allowed()
	if !ok || p.held && time.Since(p.last) > 5*time.Second {
		p.clear()
		return false
	}
	return true
}
func (p *desktopInput) Close() {
	p.cancel()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.closed = true
	p.clear()
}
func desktopCoordinate(v, stream, native int) int {
	if native <= 1 || stream <= 1 {
		return 0
	}
	return v * (native - 1) / (stream - 1)
}
func (p *desktopInput) Pointer(ctx context.Context, mask byte, x, y int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.buttons = mask
	if mask&1 == 0 {
		p.blocked = false
	}
	if ctx.Err() != nil || !p.check() {
		if mask&1 != 0 {
			p.blocked = true
		}
		return
	}
	if x < 0 || y < 0 || x >= p.sw || y >= p.sh || mask&0x86 != 0 {
		p.clear()
		return
	}
	p.x = desktopCoordinate(x, p.sw, p.w)
	p.y = desktopCoordinate(y, p.sh, p.h)
	if p.blocked {
		return
	}
	p.last = time.Now()
	if mask&1 != 0 {
		if !p.held {
			p.held = true
			p.emit(touch.Down)
		} else {
			p.emit(touch.Move)
		}
	} else if p.held {
		p.emit(touch.Up)
		p.held = false
	}
	// Wheel buttons are short bounded touch swipes, never OS wheel injection.
	if mask&0x78 != 0 && mask&1 == 0 {
		dx, dy := 0, 0
		distance := min(p.w, p.h) / 4
		switch {
		case mask&8 != 0:
			dy = distance
		case mask&16 != 0:
			dy = -distance
		case mask&32 != 0:
			dx = distance
		case mask&64 != 0:
			dx = -distance
		}
		sx, sy := p.x, p.y
		if dx != 0 {
			sx = max(distance, min(p.w-distance-1, sx))
		}
		if dy != 0 {
			sy = max(distance, min(p.h-distance-1, sy))
		}
		p.x, p.y = sx, sy
		p.held = true
		p.emit(touch.Down)
		for i := 1; i <= 8; i++ {
			timer := time.NewTimer(8 * time.Millisecond)
			select {
			case <-p.ctx.Done():
				timer.Stop()
				p.clear()
				return
			case <-timer.C:
			}
			if ctx.Err() != nil || !p.check() {
				return
			}
			p.x = sx + dx*i/8
			p.y = sy + dy*i/8
			p.emit(touch.Move)
		}
		if p.check() {
			p.emit(touch.Up)
			p.held = false
		}
	}
}
func desktopKey(symbol uint32, ctrl, other bool) (textentry.Request, bool) {
	if other {
		return textentry.Request{}, false
	}
	if ctrl {
		if symbol == 'a' || symbol == 'A' {
			return textentry.Request{Action: "key", Key: "SelectAll"}, true
		}
		return textentry.Request{}, false
	}
	keys := map[uint32]string{0xff08: "Backspace", 0xffff: "Delete", 0xff0d: "Enter", 0xff1b: "Escape", 0xff51: "ArrowLeft", 0xff52: "ArrowUp", 0xff53: "ArrowRight", 0xff54: "ArrowDown", 0xff50: "Home", 0xff57: "End"}
	if key, ok := keys[symbol]; ok {
		return textentry.Request{Action: "key", Key: key}, true
	}
	var r rune
	if symbol >= 0x01000000 && symbol <= 0x0110ffff {
		r = rune(symbol - 0x01000000)
	} else if symbol <= 255 {
		r = rune(symbol)
	} else {
		return textentry.Request{}, false
	}
	if !utf8.ValidRune(r) || !unicode.IsPrint(r) {
		return textentry.Request{}, false
	}
	return textentry.Request{Action: "type", Text: string(r)}, true
}
func (p *desktopInput) Key(ctx context.Context, down bool, symbol uint32) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !down {
		delete(p.keys, symbol)
		delete(p.denied, symbol)
		return
	}
	if ctx.Err() != nil || !p.check() {
		if len(p.denied) < 64 {
			p.denied[symbol] = true
		}
		return
	}
	if p.denied[symbol] || len(p.keys) >= 64 {
		return
	}
	p.keys[symbol] = true
	ctrl := p.keys[0xffe3] || p.keys[0xffe4]
	other := p.keys[0xffe9] || p.keys[0xffea] || p.keys[0xffeb] || p.keys[0xffec] || p.keys[0xffe7] || p.keys[0xffe8]
	request, ok := desktopKey(symbol, ctrl, other)
	if !ok || p.keyboard == nil {
		return
	}
	callCtx, cancel := context.WithTimeout(context.WithValue(p.ctx, desktopEpochKey{}, p.epoch), 250*time.Millisecond)
	defer cancel()
	_ = p.keyboard(callCtx, request) // Never log key/text content or editor results.
}
func nodeDesktopInput(ctx context.Context, a *connection.NodeAccess, sw, sh int) *desktopInput {
	w, h := display.Get().Size()
	orientation := display.Get().Orientation()
	allowed := func() bool {
		nw, nh := display.Get().Size()
		return a.Granted("input") && nodeInputAllowed() && nw == w && nh == h && display.Get().Orientation() == orientation
	}
	return newDesktopInput(ctx, sw, sh, w, h, allowed, func(c touch.Contact) { hwtouch.Get().Feed(c) }, func(ctx context.Context, r textentry.Request) error {
		_, err := frontend.Get().Input().CallGuarded(ctx, r, func(v textentry.View) bool {
			expected, _ := ctx.Value(desktopEpochKey{}).(uint64)
			return !v.Password && allowed() && a.InputGeneration() == expected
		})
		if err == nil {
			featureshell.Get().Touch()
		} // Accepted remote typing is UI activity too.
		return err
	}, a.InputGeneration)
}
