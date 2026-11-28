//go:build !board_biscuit

package gui

import (
	"fmt"
	gogui "github.com/go-gui-org/go-gui/gui"
	"github.com/ygelfand/echolocal/internal/avatar"
	"github.com/ygelfand/echolocal/internal/feature/clawtalk"
	"github.com/ygelfand/libcountertop/pkg/say"
	"math"
	"strings"
	"testing"
)

func TestFullscreenTurnStatesStaticLocalizedAndActionable(t *testing.T) {
	old := say.Chosen()
	defer say.Use(old)
	phases := []string{"turn_listening", "turn_speaking", "turn_waiting", "turn_muted", "turn_connecting", "turn_disconnected"}
	for _, lang := range []string{"en", "de", "it", "vi"} {
		say.Use(lang)
		for _, size := range [][2]int{{1280, 800}, {800, 480}, {640, 480}} {
			for _, phase := range phases {
				for _, muted := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s-%d-%s-%t", lang, size[0], phase, muted), func(t *testing.T) {
						cue := clawtalk.TurnIndicator{Phase: phase, Muted: muted}
						p := &clawtalk.AvatarPage{TurnIndicator: func() clawtalk.TurnIndicator { return cue }}
						switches, ends := 0, 0
						w := gogui.NewWindow(gogui.WindowCfg{State: new(int), Width: size[0], Height: size[1]})
						root := w.TestRender(func(w *gogui.Window) gogui.View {
							return fullscreenActivityComposition(p, size[0], size[1], func() { switches++ }, func() { ends++ }, muted, true)
						})
						symbol := avatarCaptionShape(t, root, "talk-turn-symbol")
						button := avatarCaptionShape(t, root, "talk-fullscreen-switch")
						stop := avatarCaptionShape(t, root, "talk-fullscreen-stop")
						if button.Width != 72 || button.Height != 72 || stop.Width != 72 || stop.Height != 72 || button.X+36 != float32(size[0]/2-48) || button.Y+36 != float32(size[1]-56) {
							t.Fatal("changed touch aperture", button, stop)
						}
						if symbol.X < button.X || symbol.X+symbol.Width > button.X+72 || symbol.Y < button.Y || symbol.Y+symbol.Height > button.Y+72 {
							t.Fatal("symbol clipped", symbol, button)
						}
						if !strings.Contains(talkTurnLabel(cue), avatar.Text(lang, phase)) || !strings.Contains(talkTurnLabel(cue), avatar.Text(lang, "switch_split")) {
							t.Fatal("localized state/action missing")
						}
						if muted && !strings.Contains(talkTurnLabel(cue), avatar.Text(lang, "turn_muted")) {
							t.Fatal("input privacy omitted")
						}
						for _, id := range []string{"talk-fullscreen-switch", "talk-fullscreen-stop"} {
							if err := w.TestClick(avatarCaptionID(t, w, id)); err != nil {
								t.Fatal(err)
							}
						}
						if switches != 1 || ends != 1 {
							t.Fatal("state indicator changed action")
						}
						if avatarCaptionFind(root, "talk-agent-activity") == nil {
							t.Fatal("working indicator displaced")
						}
						// Same page must clear old state on its next ordinary redraw.
						cue.Phase = "turn_disconnected"
						root = w.TestRender(nil)
						if avatarCaptionFind(root, "talk-turn-symbol") == nil {
							t.Fatal("neutral status absent")
						}
					})
				}
			}
		}
	}
}

// WCAG relative luminance: white symbols retain >4.5:1 in each selected fill.
func TestTurnPaletteWhiteContrast(t *testing.T) {
	for _, rgb := range []uint32{0x145F62, 0x304F91, 0x23383F} {
		luminance := 0.0
		for i, weight := range []float64{.2126, .7152, .0722} {
			c := float64((rgb>>uint(16-i*8))&255) / 255
			if c <= .04045 {
				c /= 12.92
			} else {
				c = math.Pow((c+.055)/1.055, 2.4)
			}
			luminance += weight * c
		}
		if contrast := 1.05 / (luminance + .05); contrast < 4.5 {
			t.Fatal("low contrast", rgb, contrast)
		}
	}
}

func TestTurnSymbolRepaintsAcrossStableCanvasID(t *testing.T) {
	cue := clawtalk.TurnIndicator{Phase: "turn_waiting"}
	p := &clawtalk.AvatarPage{TurnIndicator: func() clawtalk.TurnIndicator { return cue }}
	w := gogui.NewWindow(gogui.WindowCfg{State: new(int), Width: 1280, Height: 800})
	w.TestRender(func(w *gogui.Window) gogui.View {
		return fullscreenActivityComposition(p, 1280, 800, func() {}, func() {})
	})
	w.FrameFn()
	triangles := func() int {
		n := 0
		for _, r := range w.Renderers() {
			n += len(r.Triangles)
		}
		return n
	}
	neutral := triangles()
	cue.Phase = "turn_listening"
	w.InvalidateLayout()
	w.FrameFn()
	listening := triangles()
	if listening == neutral {
		t.Fatal("listening retained cached neutral symbol")
	}
	cue.Phase = "turn_speaking"
	w.InvalidateLayout()
	w.FrameFn()
	speaking := triangles()
	if speaking == listening || speaking == neutral {
		t.Fatal("speaking retained previous symbol")
	}
	cue.Muted = true
	w.InvalidateLayout()
	w.FrameFn()
	if triangles() == speaking {
		t.Fatal("input mute did not repaint symbol")
	}
	cue = clawtalk.TurnIndicator{Phase: "turn_waiting"}
	w.InvalidateLayout()
	w.FrameFn()
	if triangles() != neutral {
		t.Fatal("neutral retained stale active symbol")
	}
}
