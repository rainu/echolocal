//go:build !board_biscuit

package gui

import (
	gogui "github.com/go-gui-org/go-gui/gui"
	"github.com/ygelfand/echolocal/internal/avatar"
	"github.com/ygelfand/libcountertop/pkg/say"
	"math"
	"strings"
	"testing"
)

func TestLabSpeechIntensityControlsAndTranslations(t *testing.T) {
	old := say.Chosen()
	defer say.Use(old)
	for _, lang := range []string{"en", "de", "it", "vi"} {
		t.Run(lang, func(t *testing.T) {
			say.Use(lang)
			c := &avatarGestureController{state: avatar.State{Options: avatar.Options{MouthIntensity: .5, Zoom: 1}}}
			v := &avatarView{app: c, section: 2}
			w := gogui.NewWindow(gogui.WindowCfg{State: new(int), Width: 1280, Height: 800})
			w.SetView(v.build)
			root := w.TestRender(nil)
			text := strings.Join(avatarCaptionTexts(root), " ")
			if !strings.Contains(text, avatar.Text(lang, "mouth_intensity")) || !strings.Contains(text, avatar.Text(lang, "speech_preview")) {
				t.Fatal("missing translation", text)
			}
			click := func(id string) {
				t.Helper()
				root = w.TestRender(nil)
				s := avatarCaptionShape(t, root, id)
				scroller := avatarCaptionShape(t, root, "vrm-controls-2")
				if s.Y < scroller.Y || s.Y+s.Height > scroller.Y+scroller.Height {
					if err := w.TestScroll(avatarCaptionID(t, w, "vrm-controls-2"), 0, (scroller.Y+scroller.Height/2-s.Y-s.Height/2)/gogui.CurrentTheme().ScrollMultiplier); err != nil {
						t.Fatal(err)
					}
					w.TestRender(nil)
				}
				if err := w.TestClick(avatarCaptionID(t, w, id)); err != nil {
					t.Fatal(err)
				}
				w.TestRender(nil)
			}
			click("avatar-intensity-down")
			if math.Abs(c.state.Options.MouthIntensity-.4) > .0001 {
				t.Fatal(c.state)
			}
			click("avatar-intensity-up")
			if math.Abs(c.state.Options.MouthIntensity-.5) > .0001 {
				t.Fatal(c.state)
			}
			click("avatar-mouth-reset")
			if c.state.Options.MouthIntensity != .5 {
				t.Fatal(c.state)
			}
			click("avatar-speech-preview")
			if c.state.Options.Mouth != avatar.MouthSpeechPreview || !c.state.Running {
				t.Fatal(c.state)
			}
		})
	}
}
