//go:build !board_biscuit

package gui

import (
	"fmt"
	gogui "github.com/go-gui-org/go-gui/gui"
	"github.com/ygelfand/echolocal/internal/avatar"
	featureshell "github.com/ygelfand/echolocal/internal/feature/shell"
	"math"
	"testing"
)

type avatarGestureController struct{ state avatar.State }

func (c *avatarGestureController) Snapshot() avatar.State     { return c.state }
func (c *avatarGestureController) Configure(o avatar.Options) { c.state.Options = o }
func (c *avatarGestureController) Start()                     { c.state.Running = true; c.state.Ready = true }
func (c *avatarGestureController) Stop()                      { c.state.Running = false }
func (*avatarGestureController) SetViewport(avatar.Viewport)  {}
func avatarGestureFixture(t *testing.T) (*gogui.Window, *avatarView, *avatarGestureController) {
	t.Helper()
	c := &avatarGestureController{state: avatar.State{Running: true, Ready: true, Notice: "running"}}
	v := &avatarView{app: c}
	featureshell.Get().Push(v)
	w := gogui.NewWindow(gogui.WindowCfg{State: new(int), Width: 1280, Height: 800})
	w.SetView(v.build)
	w.FrameFn()
	t.Cleanup(func() {
		w.MouseCancel()
		if featureshell.Get().Top() == v {
			featureshell.Get().Pop()
		}
	})
	return w, v, c
}
func avatarTouchEvent(w *gogui.Window, typ gogui.EventType, x, y float32) {
	w.EventFn(&gogui.Event{Type: typ, NumTouches: 1, Touches: [8]gogui.TouchPoint{{Identifier: 42, PosX: x, PosY: y, ToolType: gogui.TouchToolFinger, Changed: true}}})
	w.FrameFn()
}
func avatarSwipe(w *gogui.Window, x1, y1, x2, y2 float32) {
	avatarTouchEvent(w, gogui.EventTouchesBegan, x1, y1)
	for i := 1; i <= 10; i++ {
		a := float32(i) / 10
		avatarTouchEvent(w, gogui.EventTouchesMoved, x1+(x2-x1)*a, y1+(y2-y1)*a)
	}
	avatarTouchEvent(w, gogui.EventTouchesEnded, x2, y2)
}
func TestAvatarTouchDragAndRelease(t *testing.T) {
	w, _, c := avatarGestureFixture(t)
	avatarSwipe(w, 900, 300, 1220, 300)
	if math.Abs(c.state.Options.Yaw-90) > .01 {
		t.Fatalf("touch did not rotate live: %v", c.state.Options.Yaw)
	}
	w.EventFn(&gogui.Event{Type: gogui.EventMouseMove, MouseX: 800, MouseY: 300})
	w.FrameFn()
	if math.Abs(c.state.Options.Yaw-90) > .01 {
		t.Fatal("rotation continued after release")
	}
}
func TestAvatarTouchStartsOnlyInsidePreview(t *testing.T) {
	for _, tc := range []struct {
		name string
		x, y float32
	}{{"credit", 900, 40}, {"left controls", 200, 750}, {"below preview", 900, 770}} {
		t.Run(tc.name, func(t *testing.T) {
			w, _, c := avatarGestureFixture(t)
			avatarSwipe(w, tc.x, tc.y, 1100, 400)
			if c.state.Options.Yaw != 0 {
				t.Fatal("outside touch rotated preview", c.state.Options.Yaw)
			}
		})
	}
}
func TestAvatarVerticalAndStoppedTouchIgnored(t *testing.T) {
	w, _, c := avatarGestureFixture(t)
	avatarSwipe(w, 900, 300, 900, 600)
	if c.state.Options.Yaw != 0 || math.Abs(c.state.Options.Pitch-84.375) > .01 {
		t.Fatal("vertical tilt missing", c.state.Options)
	}
	c.state.Running = false
	avatarSwipe(w, 900, 300, 1220, 300)
	if c.state.Options.Yaw != 0 {
		t.Fatal("stopped preview rotated")
	}
}
func TestAvatarTouchCancelReleasesGesture(t *testing.T) {
	w, _, c := avatarGestureFixture(t)
	avatarTouchEvent(w, gogui.EventTouchesBegan, 900, 300)
	avatarTouchEvent(w, gogui.EventTouchesMoved, 1060, 300)
	avatarTouchEvent(w, gogui.EventTouchesCancelled, 1060, 300)
	before := c.state.Options.Yaw
	w.EventFn(&gogui.Event{Type: gogui.EventMouseMove, MouseX: 1200, MouseY: 300})
	w.FrameFn()
	if c.state.Options.Yaw != before {
		t.Fatal("cancelled drag continued")
	}
	avatarSwipe(w, 900, 300, 1060, 300)
	if math.Abs(c.state.Options.Yaw-before-45) > .01 {
		t.Fatal("next drag did not work")
	}
}

type avatarCover struct{}

func (*avatarCover) Covers() bool { return true }
func TestAvatarTouchCannotRotateAfterLeavingLab(t *testing.T) {
	w, _, c := avatarGestureFixture(t)
	avatarTouchEvent(w, gogui.EventTouchesBegan, 900, 300)
	avatarTouchEvent(w, gogui.EventTouchesMoved, 1060, 300)
	cover := &avatarCover{}
	featureshell.Get().Push(cover)
	defer featureshell.Get().Pop()
	before := c.state.Options.Yaw
	w.SetView(func(*gogui.Window) gogui.View { return gogui.Column(gogui.ContainerCfg{Sizing: gogui.FillFill}) })
	w.FrameFn()
	avatarTouchEvent(w, gogui.EventTouchesMoved, 1200, 300)
	avatarTouchEvent(w, gogui.EventTouchesEnded, 1200, 300)
	if c.state.Options.Yaw != before {
		t.Fatal("hidden lab rotated")
	}
}

func TestAvatarTouchSingleMoveAppliesReleasePosition(t *testing.T) {
	w, _, c := avatarGestureFixture(t)
	avatarTouchEvent(w, gogui.EventTouchesBegan, 900, 300)
	avatarTouchEvent(w, gogui.EventTouchesMoved, 1220, 300)
	avatarTouchEvent(w, gogui.EventTouchesEnded, 1220, 300)
	if math.Abs(c.state.Options.Yaw-90) > .01 {
		t.Fatal("quick swipe lost release position", c.state.Options.Yaw)
	}
}

func avatarTwoTouch(w *gogui.Window, typ gogui.EventType, x1, x2 float32, secondOnly bool) {
	w.EventFn(&gogui.Event{Type: typ, NumTouches: 2, Touches: [8]gogui.TouchPoint{
		{Identifier: 42, PosX: x1, PosY: 300, Changed: !secondOnly, ToolType: gogui.TouchToolFinger},
		{Identifier: 43, PosX: x2, PosY: 300, Changed: true, ToolType: gogui.TouchToolFinger},
	}})
	w.FrameFn()
}
func TestAvatarPinchRealTouchPipeline(t *testing.T) {
	w, _, c := avatarGestureFixture(t)
	avatarTouchEvent(w, gogui.EventTouchesBegan, 850, 300)
	avatarTwoTouch(w, gogui.EventTouchesBegan, 850, 1050, true)
	avatarTwoTouch(w, gogui.EventTouchesMoved, 750, 1150, false)
	if math.Abs(c.state.Options.Zoom-2) > .01 || c.state.Options.Yaw != 0 || c.state.Options.Pitch != 0 {
		t.Fatal("pinch", c.state.Options)
	}
	avatarTwoTouch(w, gogui.EventTouchesMoved, 850, 1050, false)
	if math.Abs(c.state.Options.Zoom-1) > .01 {
		t.Fatal("reverse pinch", c.state.Options)
	}
	avatarTwoTouch(w, gogui.EventTouchesEnded, 850, 1050, false)
	before := c.state.Options
	avatarTwoTouch(w, gogui.EventTouchesMoved, 750, 1150, false)
	if c.state.Options != before {
		t.Fatal("ended pinch continued")
	}
	avatarSwipe(w, 900, 300, 1060, 460)
	if math.Abs(c.state.Options.Yaw-45) > .01 || math.Abs(c.state.Options.Pitch-45) > .01 {
		t.Fatal("drag after pinch", c.state.Options)
	}
}
func TestAvatarPinchCancelScopeAndLimits(t *testing.T) {
	for _, outside := range []bool{false, true} {
		t.Run(fmt.Sprint(outside), func(t *testing.T) {
			w, _, c := avatarGestureFixture(t)
			x := float32(850)
			if outside {
				x = 600
			}
			avatarTouchEvent(w, gogui.EventTouchesBegan, x, 300)
			avatarTwoTouch(w, gogui.EventTouchesBegan, x, 1050, true)
			avatarTwoTouch(w, gogui.EventTouchesMoved, 650, 1250, false)
			if outside && c.state.Options.Zoom != 0 {
				t.Fatal("outside origin zoomed")
			}
			avatarTwoTouch(w, gogui.EventTouchesCancelled, 650, 1250, false)
			before := c.state.Options
			avatarTwoTouch(w, gogui.EventTouchesMoved, 700, 1200, false)
			if c.state.Options != before {
				t.Fatal("cancel continued")
			}
		})
	}
	w, _, c := avatarGestureFixture(t)
	avatarTwoTouch(w, gogui.EventTouchesBegan, 900, 1000, false)
	avatarTwoTouch(w, gogui.EventTouchesMoved, 700, 1200, false)
	if c.state.Options.Zoom != 5 {
		t.Fatal("upper zoom limit", c.state.Options.Zoom)
	}
	avatarTwoTouch(w, gogui.EventTouchesEnded, 700, 1200, false)
	c.state.Options.Zoom = 3
	avatarTwoTouch(w, gogui.EventTouchesBegan, 900, 1000, false)
	avatarTwoTouch(w, gogui.EventTouchesMoved, 700, 1200, false)
	if c.state.Options.Zoom != 9.99 {
		t.Fatal("upper zoom limit", c.state.Options.Zoom)
	}
	avatarTwoTouch(w, gogui.EventTouchesMoved, 949, 951, false)
	if c.state.Options.Zoom != .5 {
		t.Fatal("lower zoom limit", c.state.Options.Zoom)
	}
}
func TestAvatarPinchHiddenLab(t *testing.T) {
	w, _, c := avatarGestureFixture(t)
	avatarTwoTouch(w, gogui.EventTouchesBegan, 850, 1050, false)
	featureshell.Get().Push(&avatarCover{})
	defer featureshell.Get().Pop()
	avatarTwoTouch(w, gogui.EventTouchesMoved, 750, 1150, false)
	if c.state.Options.Zoom != 0 {
		t.Fatal("hidden lab zoomed")
	}
}

func TestAvatarPinchOneFingerLiftWaitsForAll(t *testing.T) {
	w, _, c := avatarGestureFixture(t)
	avatarTwoTouch(w, gogui.EventTouchesBegan, 850, 1050, false)
	avatarTwoTouch(w, gogui.EventTouchesMoved, 750, 1150, false)
	avatarTwoTouch(w, gogui.EventTouchesEnded, 750, 1150, true)
	before := c.state.Options
	avatarTouchEvent(w, gogui.EventTouchesMoved, 950, 500)
	avatarTouchEvent(w, gogui.EventTouchesEnded, 950, 500)
	if c.state.Options != before {
		t.Fatal("remaining finger changed pose", c.state.Options, before)
	}
	avatarSwipe(w, 900, 300, 1060, 300)
	if math.Abs(c.state.Options.Yaw-before.Yaw-45) > .01 {
		t.Fatal("fresh gesture failed", c.state.Options)
	}
}
func TestAvatarPinchThirdFingerAndStop(t *testing.T) {
	w, _, c := avatarGestureFixture(t)
	avatarTwoTouch(w, gogui.EventTouchesBegan, 850, 1050, false)
	w.EventFn(&gogui.Event{Type: gogui.EventTouchesBegan, NumTouches: 1, Touches: [8]gogui.TouchPoint{{Identifier: 44, PosX: 900, PosY: 400, Changed: true}}})
	w.FrameFn()
	avatarTwoTouch(w, gogui.EventTouchesMoved, 750, 1150, false)
	if c.state.Options.Zoom != 0 {
		t.Fatal("third finger zoomed")
	}
	c.state.Running = false
	avatarTwoTouch(w, gogui.EventTouchesMoved, 800, 1100, false)
	if c.state.Options.Zoom != 0 {
		t.Fatal("stopped pinch zoomed")
	}
}

func avatarPairXY(w *gogui.Window, typ gogui.EventType, x1, y1, x2, y2 float32) {
	w.EventFn(&gogui.Event{Type: typ, NumTouches: 2, Touches: [8]gogui.TouchPoint{
		{Identifier: 42, PosX: x1, PosY: y1, Changed: true, ToolType: gogui.TouchToolFinger},
		{Identifier: 43, PosX: x2, PosY: y2, Changed: true, ToolType: gogui.TouchToolFinger},
	}})
	w.FrameFn()
}
func TestAvatarParallelPanDoesNotRotateOrZoom(t *testing.T) {
	w, _, c := avatarGestureFixture(t)
	c.state.Options = avatar.Options{Yaw: 35, Pitch: -20, Zoom: 2}
	avatarPairXY(w, gogui.EventTouchesBegan, 800, 300, 1000, 300)
	avatarPairXY(w, gogui.EventTouchesMoved, 864, 332, 1064, 332)
	o := c.state.Options
	if math.Abs(o.PanX-.1) > .0001 || math.Abs(o.PanY-.05) > .0001 || o.Zoom != 2 || o.Yaw != 35 || o.Pitch != -20 {
		t.Fatal(o)
	}
	// Once pan owns the gesture, spacing drift must not introduce zoom.
	avatarPairXY(w, gogui.EventTouchesMoved, 844, 340, 1124, 340)
	if c.state.Options.Zoom != 2 {
		t.Fatal("pan changed zoom")
	}
	avatarPairXY(w, gogui.EventTouchesEnded, 864, 364, 1064, 364)
	if math.Abs(c.state.Options.PanY-.1) > .0001 {
		t.Fatal("final release lost", c.state.Options)
	}
	before := c.state.Options
	avatarPairXY(w, gogui.EventTouchesMoved, 900, 400, 1100, 400)
	if c.state.Options != before {
		t.Fatal("released pan continued")
	}
	avatarSwipe(w, 900, 300, 1060, 300)
	if math.Abs(c.state.Options.Yaw-80) > .01 || c.state.Options.PanX != before.PanX {
		t.Fatal("single finger after pan", c.state.Options)
	}
}
func TestAvatarPinchDriftDoesNotPan(t *testing.T) {
	w, _, c := avatarGestureFixture(t)
	c.state.Options = avatar.Options{Zoom: 1, PanX: .2, PanY: -.1, Yaw: 30, Pitch: 20}
	avatarPairXY(w, gogui.EventTouchesBegan, 850, 300, 1050, 300)
	avatarPairXY(w, gogui.EventTouchesMoved, 755, 303, 1155, 303)
	if c.state.Options.Zoom != 2 || c.state.Options.PanX != .2 || c.state.Options.PanY != -.1 {
		t.Fatal(c.state.Options)
	}
	avatarPairXY(w, gogui.EventTouchesMoved, 775, 323, 1175, 323)
	if c.state.Options.PanX != .2 || c.state.Options.PanY != -.1 || c.state.Options.Yaw != 30 || c.state.Options.Pitch != 20 {
		t.Fatal("pinch drift", c.state.Options)
	}
}
func TestAvatarTwoFingerJitterAndReset(t *testing.T) {
	w, v, c := avatarGestureFixture(t)
	before := c.state.Options
	avatarPairXY(w, gogui.EventTouchesBegan, 850, 300, 1050, 300)
	avatarPairXY(w, gogui.EventTouchesMoved, 852, 301, 1049, 299)
	if c.state.Options != before {
		t.Fatal("jitter changed view", c.state.Options)
	}
	avatarPairXY(w, gogui.EventTouchesMoved, 914, 364, 1114, 364)
	v.touchReset.Add(1)
	c.state.Options = avatar.Options{Zoom: 1}
	avatarPairXY(w, gogui.EventTouchesMoved, 950, 400, 1150, 400)
	if c.state.Options != (avatar.Options{Zoom: 1}) {
		t.Fatal("stale gesture after reset", c.state.Options)
	}
}
func TestAvatarPanScopeCancelAndLift(t *testing.T) {
	for _, kind := range []string{"outside", "third", "cancel", "stop", "lift"} {
		t.Run(kind, func(t *testing.T) {
			w, _, c := avatarGestureFixture(t)
			x := float32(850)
			if kind == "outside" {
				x = 600
			}
			avatarPairXY(w, gogui.EventTouchesBegan, x, 300, 1050, 300)
			if kind == "third" {
				w.EventFn(&gogui.Event{Type: gogui.EventTouchesBegan, NumTouches: 1, Touches: [8]gogui.TouchPoint{{Identifier: 44, PosX: 900, PosY: 400, Changed: true}}})
			}
			if kind == "cancel" {
				avatarPairXY(w, gogui.EventTouchesCancelled, x, 300, 1050, 300)
			}
			if kind == "stop" {
				c.state.Running = false
			}
			if kind == "lift" {
				avatarTwoTouch(w, gogui.EventTouchesEnded, x, 1050, true)
			}
			before := c.state.Options
			avatarPairXY(w, gogui.EventTouchesMoved, x+64, 364, 1114, 364)
			if c.state.Options != before {
				t.Fatal(kind, c.state.Options)
			}
		})
	}
}
func TestAvatarPanBounds(t *testing.T) {
	w, _, c := avatarGestureFixture(t)
	avatarPairXY(w, gogui.EventTouchesBegan, 850, 300, 1050, 300)
	avatarPairXY(w, gogui.EventTouchesMoved, 5000, -5000, 5200, -5000)
	if c.state.Options.PanX != 5 || c.state.Options.PanY != -5 {
		t.Fatal(c.state.Options)
	}
}

func TestAvatarPanSerializedContactEvents(t *testing.T) {
	w, _, c := avatarGestureFixture(t)
	avatarTouchEvent(w, gogui.EventTouchesBegan, 850, 300)
	avatarTwoTouch(w, gogui.EventTouchesBegan, 850, 1050, true)
	avatarTouchEvent(w, gogui.EventTouchesMoved, 914, 364)
	if c.state.Options != (avatar.Options{}) {
		t.Fatal("first contact classified early", c.state.Options)
	}
	w.EventFn(&gogui.Event{Type: gogui.EventTouchesMoved, NumTouches: 1, Touches: [8]gogui.TouchPoint{{Identifier: 43, PosX: 1114, PosY: 364, Changed: true, ToolType: gogui.TouchToolFinger}}})
	w.FrameFn()
	if math.Abs(c.state.Options.PanX-.1) > .0001 || math.Abs(c.state.Options.PanY-.1) > .0001 || c.state.Options.Zoom != 0 || c.state.Options.Yaw != 0 || c.state.Options.Pitch != 0 {
		t.Fatal(c.state.Options)
	}
}
func TestAvatarPanSerializedHorizontal(t *testing.T) {
	w, _, c := avatarGestureFixture(t)
	avatarTouchEvent(w, gogui.EventTouchesBegan, 850, 300)
	avatarTwoTouch(w, gogui.EventTouchesBegan, 850, 1050, true)
	avatarTouchEvent(w, gogui.EventTouchesMoved, 914, 300)
	if c.state.Options != (avatar.Options{}) {
		t.Fatal("first contact classified early", c.state.Options)
	}
	avatarTwoTouch(w, gogui.EventTouchesMoved, 914, 1114, true)
	if math.Abs(c.state.Options.PanX-.1) > .0001 || c.state.Options.Zoom != 0 || c.state.Options.Yaw != 0 {
		t.Fatal(c.state.Options)
	}
}
