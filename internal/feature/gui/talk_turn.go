//go:build !board_biscuit

package gui

import (
	gogui "github.com/go-gui-org/go-gui/gui"
	"github.com/ygelfand/echolocal/internal/avatar"
	"github.com/ygelfand/echolocal/internal/feature/clawtalk"
	"github.com/ygelfand/libcountertop/pkg/say"
)

func talkTurnColor(cue clawtalk.TurnIndicator) gogui.Color {
	switch cue.Phase {
	case "turn_listening":
		return gogui.Hex(0x145F62)
	case "turn_speaking":
		return gogui.Hex(0x304F91)
	default:
		return gogui.Hex(0x23383F)
	}
}

func talkTurnLabel(cue clawtalk.TurnIndicator) string {
	label := avatar.Text(say.Chosen(), cue.Phase)
	if cue.Muted && cue.Phase != "turn_muted" {
		label += " · " + avatar.Text(say.Chosen(), "turn_muted")
	}
	return label + " · " + avatar.Text(say.Chosen(), "switch_split")
}

// Same 72px touch target and split-view action. Static mic/speaker/dash shapes
// distinguish states without color or motion, separate from the working dots.
func talkTurnButton(cue clawtalk.TurnIndicator, do func()) gogui.View {
	st := gogui.CurrentTheme().TextStyleIconLarge
	st.Size, st.Color = 26, gogui.White
	return gogui.Button(gogui.ButtonCfg{
		ID: "talk-fullscreen-switch", A11YCfg: gogui.A11YCfg{A11YLabel: talkTurnLabel(cue)},
		Width: 72, Height: 72, MinWidth: 72, MinHeight: 72, Sizing: gogui.FixedFixed,
		Radius: gogui.RadiusPx(36), Color: talkTurnColor(cue), Padding: gogui.NoPadding,
		Content: []gogui.View{gogui.Row(gogui.ContainerCfg{Sizing: gogui.FitFit, Padding: gogui.NoPadding, Spacing: gogui.NoSpacing, Content: []gogui.View{
			gogui.Text(gogui.TextCfg{Text: gogui.IconColumns, TextStyle: st}),
			gogui.DrawCanvas(gogui.DrawCanvasCfg{ID: "talk-turn-symbol", Width: 30, Height: 28, VersionFn: func() uint64 { return talkTurnVersion(cue) }, OnDraw: func(dc *gogui.DrawContext) {
				switch cue.Phase {
				case "turn_speaking":
					dc.Polyline([]float32{2, 11, 7, 11, 13, 5, 13, 23, 7, 17, 2, 17, 2, 11}, gogui.White, 2)
					dc.Polyline([]float32{16, 9, 19, 12, 19, 16, 16, 19}, gogui.White, 2)
				case "turn_listening", "turn_muted":
					dc.RoundedRect(8, 2, 7, 13, 3, gogui.White, 2)
					dc.Polyline([]float32{4, 10, 4, 14, 7, 19, 15, 19, 19, 14, 19, 10}, gogui.White, 2)
					dc.Line(11, 19, 11, 24, gogui.White, 2)
					dc.Line(6, 25, 16, 25, gogui.White, 2)
				default:
					dc.Line(5, 14, 18, 14, gogui.White, 2)
				}
				if cue.Muted && cue.Phase == "turn_speaking" {
					// Do not slash the speaker: only input is muted.
					dc.RoundedRect(23, 8, 4, 9, 2, gogui.White, 1)
					dc.Polyline([]float32{20, 13, 20, 17, 23, 20, 27, 20, 29, 17, 29, 13}, gogui.White, 1)
					dc.Line(25, 20, 25, 24, gogui.White, 1)
					dc.Line(22, 25, 28, 25, gogui.White, 1)
					dc.Line(20, 6, 30, 26, talkTurnColor(cue), 3)
					dc.Line(20, 6, 30, 26, gogui.White, 1)
				} else if cue.Muted {
					dc.Line(2, 2, 21, 26, talkTurnColor(cue), 5)
					dc.Line(2, 2, 21, 26, gogui.White, 2)
				}
			}}),
		}})},
		OnClick: func(e gogui.EventCtx) {
			if do != nil {
				do()
			}
			e.Window.InvalidateLayout()
		},
	})
}

// DrawCanvas caches by stable ID and version, not by closure identity. Both
// phase and input-mute changes must invalidate its recorded geometry.
func talkTurnVersion(cue clawtalk.TurnIndicator) uint64 {
	version := uint64(0)
	for i, phase := range []string{"turn_waiting", "turn_listening", "turn_speaking", "turn_muted", "turn_connecting", "turn_disconnected"} {
		if phase == cue.Phase {
			version = uint64(i + 1)
			break
		}
	}
	if cue.Muted {
		version |= 1 << 8
	}
	return version
}
