//go:build !board_biscuit

package gui

import (
	"fmt"
	"image"
	"math"
	"testing"

	gogui "github.com/go-gui-org/go-gui/gui"
	"github.com/ygelfand/libcountertop/pkg/display/interaction"
	"github.com/ygelfand/libcountertop/pkg/display/style"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
	"github.com/ygelfand/libcountertop/pkg/say"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

// The aperture must follow the real widget, including locale/side/expansion,
// rather than a hard-coded width that can hide labels or leak over controls.
func TestTalkVolumeApertureTracksArrangedCard(t *testing.T) {
	old := say.Chosen()
	defer say.Use(old)
	for _, lang := range []string{"en", "de", "it", "vi"} {
		say.Use(lang)
		for _, size := range [][2]int{{1280, 800}, {800, 480}, {640, 480}} {
			for _, left := range []bool{false, true} {
				for _, open := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/%dx%d/left=%v/open=%v", lang, size[0], size[1], left, open), func(t *testing.T) {
						presses := interaction.New()
						gestures := widgets.Gestures{}
						kit := widgets.Toolkit{Kit: func() style.Kit { return style.ByName("standard").Kit }, Pressable: presses.Pressable}
						choices := []widgets.VolumeChoice{
							{Key: "main", Label: schema.StreamMain.Label(), Level: 40},
							{Key: "media", Label: schema.StreamMedia.Label(), Level: 50},
							{Key: "voice", Label: schema.StreamVoice.Label(), Level: 60},
							{Key: "alerts", Label: schema.StreamAlerts.Label(), Level: 70},
							{Key: "feedback", Label: schema.StreamFeedback.Label(), Level: 80},
						}
						w := gogui.NewWindow(gogui.WindowCfg{State: new(int), Width: size[0], Height: size[1]})
						var got image.Rectangle
						root := w.TestRender(func(w *gogui.Window) gogui.View {
							return volumeMeasuredView{View: kit.VolumeCard(w, widgets.VolumeOptions{
								Current: choices[2], Choices: choices, Left: left, Open: open, Grip: gestures.Grip,
								Set: func(string, int) {}, Linger: func() {}, Expand: func() {}, Dismiss: func() {},
							}), report: func(r image.Rectangle) { got = r }}
						})
						s := avatarCaptionShape(t, root, "volume")
						want := image.Rect(int(math.Floor(float64(s.X))), int(math.Floor(float64(s.Y))), int(math.Ceil(float64(s.X+s.Width))), int(math.Ceil(float64(s.Y+s.Height))))
						if got.Empty() || got != want || !got.In(image.Rect(0, 0, size[0], size[1])) {
							t.Fatalf("aperture %v, card %v", got, want)
						}
						slider := avatarCaptionShape(t, root, "volume-slider-voice")
						if slider.X < float32(got.Min.X) || slider.X+slider.Width > float32(got.Max.X) {
							t.Fatal("voice slider outside aperture")
						}
						// A collapsed card on either edge leaves the central fullscreen controls exposed.
						if !open && got.Overlaps(image.Rect(size[0]/2-84, size[1]-92, size[0]/2+84, size[1]-20)) {
							t.Fatal("aperture reaches fullscreen controls")
						}
					})
				}
			}
		}
	}
}
