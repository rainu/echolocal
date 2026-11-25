//go:build !board_biscuit

package gui

import (
	"image"
	"math"
	"sync/atomic"

	gogui "github.com/go-gui-org/go-gui/gui"
	"github.com/ygelfand/echolocal/internal/config"
	featureshell "github.com/ygelfand/echolocal/internal/feature/shell"
	"github.com/ygelfand/echolocal/internal/feature/volume"
	"github.com/ygelfand/libcountertop/pkg/display/screens"
	sharedshell "github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

var volumeCardBounds atomic.Pointer[image.Rectangle]

func talkVolumeBounds() image.Rectangle {
	v := volume.Get()
	if _, selected := v.Selected(); selected && featureshell.Get().Visible(v.Card()) {
		if r := volumeCardBounds.Load(); r != nil {
			return *r
		}
	}
	return image.Rectangle{}
}

// Reuse the normal widget and all its interactions. Capture its arranged bounds
// instead of duplicating theme-, locale-, side- or expansion-dependent geometry.
func extendTalkVolume(ui *widgets.UI, o *screens.Options, _ func() *screens.App) {
	o.Views = append(o.Views, func(v sharedshell.View) *screens.Screen {
		vol := o.Volume
		if v != vol.Card() {
			return nil
		}
		return &screens.Screen{View: v, Build: func(w *gogui.Window) gogui.View {
			stream, open := vol.Picked()
			var choices []widgets.VolumeChoice
			for _, s := range o.Streams {
				choices = append(choices, widgets.VolumeChoice{Key: string(s), Label: s.Label(), Level: vol.Level(s)})
			}
			card := ui.Toolkit.VolumeCard(w, widgets.VolumeOptions{
				Current: widgets.VolumeChoice{Key: string(stream), Label: stream.Label(), Level: vol.Level(stream)},
				Choices: choices, Open: open, Left: o.Settings().Screen.Volume == schema.EdgeLeft,
				Set:    func(s string, level int) { vol.Set(config.Stream(s), level) },
				Linger: vol.Linger, Expand: vol.Expand, Dismiss: vol.Dismiss, Grip: ui.Grip,
			})
			return volumeMeasuredView{View: card, report: func(r image.Rectangle) { volumeCardBounds.Store(&r) }}
		}}
	})
}

type volumeMeasuredView struct {
	gogui.View
	report func(image.Rectangle)
}

func (v volumeMeasuredView) GenerateLayout(w *gogui.Window) gogui.Layout {
	return gogui.GenerateViewLayout(gogui.Column(gogui.ContainerCfg{
		Sizing: gogui.FillFill, Padding: gogui.NoPadding, Spacing: gogui.NoSpacing,
		SizeBorder: gogui.NoBorder, Radius: gogui.NoRadius, Content: []gogui.View{v.View},
		AmendLayout: func(e gogui.EventCtx) {
			var visit func(*gogui.Layout)
			visit = func(l *gogui.Layout) {
				if s := l.Shape; s != nil && s.ID == "volume" {
					v.report(image.Rect(int(math.Floor(float64(s.X))), int(math.Floor(float64(s.Y))),
						int(math.Ceil(float64(s.X+s.Width))), int(math.Ceil(float64(s.Y+s.Height)))))
					return
				}
				for i := range l.Children {
					visit(&l.Children[i])
				}
			}
			visit(e.Layout)
		},
	}), w)
}
