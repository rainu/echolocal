//go:build !board_biscuit

package gui

import (
	"fmt"
	"math"
	"strings"
	"testing"

	gogui "github.com/go-gui-org/go-gui/gui"
	"github.com/ygelfand/echolocal/internal/avatar"
	"github.com/ygelfand/libcountertop/pkg/say"
)

// Exercise the actual GUI layout and event path: every section must remain
// usable on the smallest supported panel, with actions outside the scroller.
func TestAvatarWorkspaceSectionsAndPinnedActions(t *testing.T) {
	old := say.Chosen()
	defer say.Use(old)
	for _, lang := range []string{"en", "de"} {
		say.Use(lang)
		for _, size := range [][2]int{{1280, 800}, {800, 480}, {640, 480}} {
			t.Run(fmt.Sprintf("%s-%dx%d", lang, size[0], size[1]), func(t *testing.T) {
				c := &avatarGestureController{state: avatar.State{Notice: "ready", Options: avatar.Options{Zoom: 1, MouthIntensity: .3}}}
				v := &avatarView{app: c}
				w := gogui.NewWindow(gogui.WindowCfg{State: new(int), Width: size[0], Height: size[1]})
				w.SetView(v.build)
				root := w.TestRender(nil)
				initial := avatarCaptionShape(t, root, "avatar-save")
				saveY := initial.Y
				expected := [][]string{
					{"avatar-file", "avatar-background", "avatar-presentation"},
					{"avatar-rotate-left", "avatar-rotate-reset", "avatar-rotate-right"},
					{"avatar-intensity-down", "avatar-intensity-up", "avatar-mouth-reset", "avatar-speech-preview", "vrm-idle", "vrm-blink", "vrm-animation"},
					{"vrm-mouth-0", "vrm-mouth-1", "vrm-mouth-2", "vrm-mouth-3", "vrm-fixed-down", "vrm-fixed-up"},
				}
				for section, ids := range expected {
					if err := w.TestClick(avatarCaptionID(t, w, fmt.Sprintf("avatar-section-%d", section))); err != nil {
						t.Fatal(err)
					}
					root = w.TestRender(nil)
					if v.section != section {
						t.Fatal("section did not change")
					}
					scrollID := fmt.Sprintf("vrm-controls-%d", section)
					scroller := avatarCaptionShape(t, root, scrollID)
					if !scroller.Scrollable || scroller.Height < 90 || scroller.X+scroller.Width > float32(size[0]/2) {
						t.Fatal("unusable section", scroller)
					}
					for _, id := range ids {
						root = w.TestRender(nil)
						shape := avatarCaptionShape(t, root, id)
						scroller = avatarCaptionShape(t, root, scrollID)
						if shape.Y < scroller.Y || shape.Y+shape.Height > scroller.Y+scroller.Height {
							delta := (scroller.Y + scroller.Height/2 - shape.Y - shape.Height/2) / gogui.CurrentTheme().ScrollMultiplier
							if err := w.TestScroll(avatarCaptionID(t, w, scrollID), 0, delta); err != nil {
								t.Fatal(err)
							}
						}
						root = w.TestRender(nil)
						shape = avatarCaptionShape(t, root, id)
						scroller = avatarCaptionShape(t, root, scrollID)
						if shape.X < scroller.X-.5 || shape.X+shape.Width > scroller.X+scroller.Width+.5 || shape.Y < scroller.Y-.5 || shape.Y+shape.Height > scroller.Y+scroller.Height+.5 || shape.Height < 48 {
							t.Fatalf("%s is clipped: %+v inside %+v", id, shape, scroller)
						}
					}
					for _, id := range []string{"vrm-back", "avatar-section-0", "avatar-section-1", "avatar-section-2", "avatar-section-3", "vrm-start", "vrm-stop", "avatar-save", "avatar-fullscreen"} {
						shape := avatarCaptionShape(t, root, id)
						if shape.Y < 0 || shape.Y+shape.Height > float32(size[1]) || shape.X < 0 || shape.X+shape.Width > float32(size[0]/2) || shape.Height < 48 {
							t.Fatal("inaccessible pinned action", id, shape)
						}
					}
					if math.Abs(float64(avatarCaptionShape(t, root, "avatar-save").Y-saveY)) > .5 {
						t.Fatalf("save moved with section %d: %v != %v", section, avatarCaptionShape(t, root, "avatar-save").Y, saveY)
					}
					info := avatarCaptionShape(t, root, "avatar-preview-info")
					if info.Y < 0 || info.Y+info.Height > 80.5 || !info.Scrollable {
						t.Fatal("credits/gesture text overlap the native viewport", info)
					}
					preview := avatarCaptionShape(t, root, "avatar-preview-touch")
					if preview.X+preview.Width > float32(size[0])+.5 || preview.X < float32(size[0]/2)-16.5 {
						t.Fatal("preview layout changed", preview)
					}
					if c.state.Options.MouthIntensity != .3 || c.state.Options.Zoom != 1 {
						t.Fatal("navigation changed configuration")
					}
				}
				if err := w.TestClick(avatarCaptionID(t, w, "vrm-start")); err != nil {
					t.Fatal(err)
				}
				root = w.TestRender(nil)
				if !c.state.Running || !strings.Contains(strings.Join(avatarCaptionTexts(root), " "), avatarWords("Preview running", "Vorschau läuft")) {
					t.Fatal("missing live state")
				}
				if err := w.TestClick(avatarCaptionID(t, w, "vrm-stop")); err != nil {
					t.Fatal(err)
				}
				w.TestRender(nil)
				if c.state.Running {
					t.Fatal("stop no longer works")
				}
			})
		}
	}
}
