//go:build !board_biscuit

package gui

import (
	"fmt"
	gogui "github.com/go-gui-org/go-gui/gui"
	"github.com/ygelfand/echolocal/internal/avatar"
	"github.com/ygelfand/echolocal/internal/config"
	"github.com/ygelfand/echolocal/internal/feature/clawtalk"
	featureshell "github.com/ygelfand/echolocal/internal/feature/shell"
	"github.com/ygelfand/libcountertop/pkg/say"
	"image"
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func previewEditFixture(t *testing.T, wid, hei int) (*gogui.Window, *avatarPreview) {
	t.Helper()
	config.Use(filepath.Join(t.TempDir(), "state.json"))
	lab := &avatarGestureController{state: avatar.State{Running: true, Ready: true}}
	v := &avatarView{app: lab, fullscreen: true}
	c := avatar.New(t.TempDir(), nil, func() bool { return false }, func() {})
	p := &avatarPreview{lab: v, renderer: c, lifecycle: &clawtalk.AvatarPage{Renderer: c}, fullscreen: true, controls: true, options: avatar.Options{Zoom: 1, PanX: .1, Mouth: avatar.MouthMicrophone}}
	p.initEditor()
	featureshell.Get().Push(v)
	featureshell.Get().Push(p)
	lab.Start() // keep the host-only renderer from launching a device surface
	w := gogui.NewWindow(gogui.WindowCfg{State: new(int), Width: wid, Height: hei})
	w.SetView(p.build)
	w.FrameFn()
	t.Cleanup(func() {
		w.MouseCancel()
		if featureshell.Get().Top() == p {
			featureshell.Get().Pop()
		}
		if featureshell.Get().Top() == v {
			featureshell.Get().Pop()
		}
	})
	return w, p
}
func TestFullscreenEditorControlsLanguagesAndAperture(t *testing.T) {
	old := say.Chosen()
	defer say.Use(old)
	for _, lang := range []string{"en", "de", "it", "vi"} {
		say.Use(lang)
		for _, size := range [][2]int{{1280, 800}, {800, 480}, {640, 480}} {
			t.Run(fmt.Sprintf("%s-%d", lang, size[0]), func(t *testing.T) {
				w, p := previewEditFixture(t, size[0], size[1])
				root := w.TestRender(nil)
				bar := avatarCaptionShape(t, root, "avatar-preview-editor")
				if bar.Y != float32(size[1]-previewEditorHeight) || bar.Width != float32(size[0]) {
					t.Fatal("aperture geometry", bar)
				}
				for _, id := range []string{"save", "reset", "split", "exit", "zoom-in", "zoom-out", "left", "right", "hide"} {
					s := avatarCaptionShape(t, root, "avatar-preview-"+id)
					if s.Height < 48 || s.Y < bar.Y || s.Y+s.Height > float32(size[1]) || s.X < 0 || s.X+s.Width > float32(size[0]) {
						t.Fatal("unreachable control", id, s)
					}
				}
				hint := avatarCaptionShape(t, root, "avatar-preview-help")
				if hint.Y+hint.Height > float32(size[1]) {
					t.Fatal("help clipped", hint)
				}
				texts := strings.Join(avatarCaptionTexts(root), " ")
				if !strings.Contains(texts, avatar.Text(lang, "placement_pending")) || !strings.Contains(texts, avatar.Text(lang, "placement_help")) {
					t.Fatal("missing localized help", texts)
				}
				if err := w.TestClick(avatarCaptionID(t, w, "avatar-preview-hide")); err != nil {
					t.Fatal(err)
				}
				root = w.TestRender(nil)
				for i, id := range []string{"avatar-preview-show", "avatar-preview-exit"} {
					s := avatarCaptionShape(t, root, id)
					if s.X+36 != float32(size[0]/2-48+i*96) || s.Y+36 != float32(size[1]-56) {
						t.Fatal("native hidden aperture mismatch", s)
					}
				}
				if *p.bounds.Load() != (image.Rectangle{}) {
					t.Fatal("hidden chrome still cuts canvas")
				}
				if err := w.TestClick(avatarCaptionID(t, w, "avatar-preview-show")); err != nil {
					t.Fatal(err)
				}
				if !p.controls {
					t.Fatal("cannot rediscover controls")
				}
			})
		}
	}
}

func TestFullscreenEditorSharedDraftSaveResetAndExit(t *testing.T) {
	w, p := previewEditFixture(t, 1280, 800)
	initial := config.AvatarSection.Read()
	o := p.options
	o.Zoom = 2.25
	o.PanX = .2
	o.PanY = -.15
	o.Yaw = 30
	p.Configure(o)
	p.switchMode()
	p.switchMode()
	if p.options != o {
		t.Fatal("mode switch lost draft", p.options)
	}
	p.Close()
	if p.lab.app.Snapshot().Options != o || !p.lab.fullscreen {
		t.Fatal("Lab did not receive unsaved edit")
	}
	if config.AvatarSection.Read() != initial {
		t.Fatal("navigation saved implicitly")
	}
	// Save via the real control, then reset via its control: reset must stay a draft.
	w.TestRender(nil)
	if err := w.TestClick(avatarCaptionID(t, w, "avatar-preview-save")); err != nil {
		t.Fatal(err)
	}
	got := config.AvatarSection.Read()
	if got.FullscreenZoom != 225 || got.FullscreenX != 20 || got.FullscreenY != -15 || got.FullscreenYaw != 30 || got.Zoom != initial.Zoom {
		t.Fatal("wrong persisted composition", got)
	}
	if p.renderer.Snapshot().Options.Mouth != avatar.MouthClosed || p.renderer.Snapshot().Running {
		t.Fatal("preview started audio/renderer in unit test")
	}
	w.TestRender(nil)
	if err := w.TestClick(avatarCaptionID(t, w, "avatar-preview-reset")); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(config.AvatarSection.Read(), got) || p.options.Zoom != 1 || p.options.PanX != 0 || p.options.PanY != 0 || p.options.Yaw != 0 {
		t.Fatal("reset saved or failed", p.options, config.AvatarSection.Read(), got)
	}
	p.Close()
	if p.lab.app.Snapshot().Options != p.options {
		t.Fatal("reset draft lost on exit")
	}
	p.lab.selectComposition(false)
	p.lab.selectComposition(true)
	if p.lab.app.Snapshot().Options != p.options {
		t.Fatal("Lab mode switch lost draft")
	}
}
func TestFullscreenEditorRealEventPathAndControlExclusion(t *testing.T) {
	w, p := previewEditFixture(t, 1280, 800)
	// The real renderer is never launched in host tests. Substitute only its
	// ready/running state while retaining the actual editor and widget callbacks.
	c := &avatarGestureController{state: avatar.State{Running: true, Ready: true, Options: avatar.Options{Zoom: 1}}}
	p.editor.app = c
	avatarPairXY(w, gogui.EventTouchesBegan, 900, 300, 1100, 300)
	avatarPairXY(w, gogui.EventTouchesMoved, 980, 340, 1180, 340)
	if math.Abs(c.state.Options.PanX-.1) > .0001 || math.Abs(c.state.Options.PanY-.05) > .0001 || c.state.Options.Zoom != 1 {
		t.Fatal("fullscreen pan uses wrong reference", c.state.Options)
	}
	avatarPairXY(w, gogui.EventTouchesEnded, 980, 340, 1180, 340)
	avatarPairXY(w, gogui.EventTouchesBegan, 900, 300, 1100, 300)
	avatarPairXY(w, gogui.EventTouchesMoved, 800, 300, 1200, 300)
	if c.state.Options.Zoom != 2 {
		t.Fatal("fullscreen pinch", c.state.Options)
	}
	avatarPairXY(w, gogui.EventTouchesEnded, 800, 300, 1200, 300)
	avatarSwipe(w, 900, 200, 1100, 200)
	if math.Abs(c.state.Options.Yaw-45) > .01 {
		t.Fatal("right-hand canvas cannot rotate", c.state.Options)
	}
	before := c.state.Options
	// A gesture which starts on the overlay may never become a canvas gesture.
	avatarPairXY(w, gogui.EventTouchesBegan, 900, 600, 1100, 600)
	avatarPairXY(w, gogui.EventTouchesMoved, 980, 400, 1180, 400)
	if c.state.Options != before {
		t.Fatal("overlay gesture changed placement")
	}
	avatarPairXY(w, gogui.EventTouchesCancelled, 980, 400, 1180, 400)
	p.controls = false
	p.editor.touchReset.Add(1)
	w.FrameFn()
	before = c.state.Options
	avatarPairXY(w, gogui.EventTouchesBegan, 600, 750, 700, 750)
	avatarPairXY(w, gogui.EventTouchesMoved, 650, 700, 750, 700)
	if c.state.Options != before {
		t.Fatal("hidden exit controls became pan")
	}
}
