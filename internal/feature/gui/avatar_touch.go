//go:build !board_biscuit

package gui

import (
	gogui "github.com/go-gui-org/go-gui/gui"
	"github.com/ygelfand/echolocal/internal/avatar"
	featureshell "github.com/ygelfand/echolocal/internal/feature/shell"
	"math"
	"time"
)

// attachAvatarTouch observes raw contacts on this UI window only. The normal
// widget/mouse path still owns one-finger rotation; a second finger cancels
// that capture in go-gui. No hardware listener or global gesture is installed.
func (v *avatarView) attachAvatarTouch(w *gogui.Window) {
	if v.touchWindow == w {
		return
	}
	v.touchWindow = w
	previous := w.OnEvent
	type point struct {
		x, y   float64
		inside bool
		moved  bool
	}
	contacts := map[uint64]point{}
	var span, centerX, centerY float64
	var origin avatar.Options
	var pendingAt time.Time
	mode := 0 // 0 undecided, 1 pinch, 2 pan; owned until all fingers lift.
	var blocked bool
	epoch := v.touchReset.Load()
	w.OnEvent = func(e *gogui.Event, w *gogui.Window) {
		if previous != nil {
			previous(e, w)
		}
		if e == nil || e.IsHandled {
			return
		}
		switch e.Type {
		case gogui.EventTouchesBegan, gogui.EventTouchesMoved, gogui.EventTouchesEnded, gogui.EventTouchesCancelled:
		default:
			return
		}
		state := v.app.Snapshot()
		if epoch != v.touchReset.Load() || !featureshell.Get().Visible(v) || !state.Running || !state.Ready {
			v.touchMulti = false
			clear(contacts)
			span = 0
			blocked = false
			epoch = v.touchReset.Load()
			if !featureshell.Get().Visible(v) || !state.Running || !state.Ready {
				return
			}
		}
		width, height := w.WindowSize()
		vp := avatar.Viewport{X: width / 2, Y: 80, Size: min(width/2, height-100)}
		for i := 0; i < min(max(e.NumTouches, 0), len(e.Touches)); i++ {
			t := e.Touches[i]
			if !t.Changed {
				continue
			}
			x, y := float64(t.PosX), float64(t.PosY)
			if e.Type == gogui.EventTouchesBegan {
				if len(contacts) >= 8 {
					blocked = true
					continue
				}
				inside := !math.IsNaN(x) && !math.IsInf(x, 0) && !math.IsNaN(y) && !math.IsInf(y, 0) && x >= float64(vp.X) && x < float64(vp.X+vp.Size) && y >= float64(vp.Y) && y < float64(vp.Y+vp.Size)
				contacts[t.Identifier] = point{x: x, y: y, inside: inside}
			} else if p, ok := contacts[t.Identifier]; ok {
				p.moved = p.moved || p.x != x || p.y != y
				p.x, p.y = x, y
				contacts[t.Identifier] = p
			}
		}
		if len(contacts) >= 2 {
			v.touchMulti = true
		}
		if len(contacts) > 2 {
			blocked = true
		}
		if len(contacts) == 2 && !blocked {
			var p [2]point
			i := 0
			for _, c := range contacts {
				p[i] = c
				i++
			}
			if !p[0].inside || !p[1].inside {
				blocked = true
			} else {
				distance := math.Hypot(p[0].x-p[1].x, p[0].y-p[1].y)
				cx, cy := (p[0].x+p[1].x)/2, (p[0].y+p[1].y)/2
				if math.IsNaN(distance) || math.IsInf(distance, 0) || math.IsNaN(cx) || math.IsInf(cx, 0) || math.IsNaN(cy) || math.IsInf(cy, 0) {
					blocked = true
				} else if distance > 1 {
					if span <= 1 || e.Type == gogui.EventTouchesBegan {
						span, centerX, centerY = distance, cx, cy
						origin = state.Options
						pendingAt = time.Time{}
						for id, c := range contacts {
							c.moved = false
							contacts[id] = c
						}
						mode = 0
					} else if e.Type != gogui.EventTouchesCancelled {
						pinch := math.Abs(distance - span)
						pan := 2 * math.Hypot(cx-centerX, cy-centerY)
						if mode == 0 && (pinch >= 12 || pan >= 12) && pendingAt.IsZero() {
							pendingAt = time.Now()
						}
						// Backend emits one contact per event. Wait for its partner
						// before classifying a parallel drag; a stationary partner
						// is allowed after a short bounded recognition delay.
						paired := p[0].moved && p[1].moved
						if mode == 0 && (paired || e.Type == gogui.EventTouchesEnded || (!pendingAt.IsZero() && time.Since(pendingAt) >= 50*time.Millisecond)) {
							switch {
							case pinch >= 12 && pinch > pan*1.2:
								mode = 1
							case pan >= 12 && pan > pinch*1.2:
								mode = 2
							// One stationary finger is inherently ambiguous in an
							// isolated sample. Require more travel before pinch.
							case math.Max(pinch, pan) >= 32:
								if pinch >= pan {
									mode = 1
								} else {
									mode = 2
								}
							}
						}
						o := state.Options
						if mode == 1 {
							o.Zoom = math.Max(.5, math.Min(3, avatar.EffectiveZoom(origin.Zoom)*distance/span))
						}
						if mode == 2 {
							o.PanX = math.Max(-1, math.Min(1, origin.PanX+(cx-centerX)/float64(vp.Size)))
							o.PanY = math.Max(-1, math.Min(1, origin.PanY+(cy-centerY)/float64(vp.Size)))
						}
						if mode != 0 {
							v.app.Configure(o)
							w.InvalidateLayout()
						}
					}
				}
			}
		} else {
			span = 0
		}
		if e.Type == gogui.EventTouchesEnded || e.Type == gogui.EventTouchesCancelled {
			for i := 0; i < min(max(e.NumTouches, 0), len(e.Touches)); i++ {
				if e.Touches[i].Changed {
					delete(contacts, e.Touches[i].Identifier)
				}
			}
			span = 0
			// Do not turn a remaining finger into a fresh rotation/zoom.
			if len(contacts) > 0 {
				blocked = true
			} else {
				blocked = false
				v.touchMulti = false
			}
		}
	}
}
