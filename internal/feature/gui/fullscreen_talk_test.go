//go:build !board_biscuit

package gui

import (
	"fmt"
	gogui "github.com/go-gui-org/go-gui/gui"
	"github.com/ygelfand/echolocal/internal/avatar"
	"github.com/ygelfand/echolocal/internal/config"
	"github.com/ygelfand/echolocal/internal/feature/clawtalk"
	"github.com/ygelfand/libcountertop/pkg/say"
	"path/filepath"
	"testing"
)

func TestFullscreenExactlyTwoControlsAndGeometry(t *testing.T) {
	old := say.Chosen()
	defer say.Use(old)
	for _, lang := range []string{"en", "de", "it", "vi"} {
		say.Use(lang)
		for _, sz := range [][2]int{{1280, 800}, {800, 480}, {640, 480}} {
			t.Run(fmt.Sprintf("%s-%d", lang, sz[0]), func(t *testing.T) {
				switches, ends := 0, 0
				w := gogui.NewWindow(gogui.WindowCfg{State: new(int), Width: sz[0], Height: sz[1]})
				root := w.TestRender(func(w *gogui.Window) gogui.View {
					return fullscreenComposition(sz[0], sz[1], func() { switches++ }, func() { ends++ })
				})
				for i, id := range []string{"talk-fullscreen-switch", "talk-fullscreen-stop"} {
					shape := avatarCaptionShape(t, root, id)
					center := float32(sz[0]/2 - 48 + i*96)
					if shape.Width != 72 || shape.Height != 72 || shape.X+36 != center || shape.Y+36 != float32(sz[1]-56) {
						t.Fatalf("native aperture parity: %+v expected center %v", shape, center)
					}
					if err := w.TestClick(avatarCaptionID(t, w, id)); err != nil {
						t.Fatal(err)
					}
				}
				if switches != 1 || ends != 1 {
					t.Fatal("actions", switches, ends)
				}
				if texts := avatarCaptionTexts(root); len(texts) != 2 {
					t.Fatal("fullscreen chrome", texts)
				}
			})
		}
	}
}

func TestFullscreenPreviewSwitchKeepsUnsavedFramingAndNoCapture(t *testing.T) {
	config.Use(filepath.Join(t.TempDir(), "state.json"))
	if err := config.AvatarSection.Update(func(a *config.Avatar) { a.Zoom = 150; a.FullscreenZoom = 225 }); err != nil {
		t.Fatal(err)
	}
	c := avatar.New(t.TempDir(), nil, func() bool { return false }, func() {})
	lab := &avatarGestureController{state: avatar.State{Running: true}}
	v := &avatarView{app: lab, fullscreen: true}
	o := avatar.Options{Zoom: 2.75, Mouth: avatar.MouthMicrophone}
	p := &avatarPreview{renderer: c, lifecycle: &clawtalk.AvatarPage{Renderer: c}, lab: v, options: o, fullscreen: true, controls: true}
	w := gogui.NewWindow(gogui.WindowCfg{State: new(int), Width: 1280, Height: 800})
	w.TestRender(p.build)
	if err := w.TestClick(avatarCaptionID(t, w, "avatar-preview-split")); err != nil {
		t.Fatal(err)
	}
	w.TestRender(nil)
	if p.fullscreen || c.Snapshot().Options.Zoom != 1.5 || c.Snapshot().Options.Mouth != avatar.MouthClosed {
		t.Fatal("split preview", c.Snapshot())
	}
	if err := w.TestClick(avatarCaptionID(t, w, "avatar-preview-fullscreen")); err != nil {
		t.Fatal(err)
	}
	w.TestRender(nil)
	if !p.fullscreen || c.Snapshot().Options.Zoom != 2.75 || c.Snapshot().Options.Mouth != avatar.MouthClosed || c.Snapshot().Running {
		t.Fatal("unsaved preview or capture", c.Snapshot())
	}
}

func TestFullscreenMutedBadgeKeepsBothActionsAndTheirApertures(t *testing.T) {
	old := say.Chosen()
	defer say.Use(old)
	for _, lang := range []string{"en", "de", "it", "vi"} {
		say.Use(lang)
		for _, size := range [][2]int{{1280, 800}, {800, 480}} {
			w := gogui.NewWindow(gogui.WindowCfg{State: new(int), Width: size[0], Height: size[1]})
			switches, ends := 0, 0
			root := w.TestRender(func(w *gogui.Window) gogui.View {
				return fullscreenComposition(size[0], size[1], func() { switches++ }, func() { ends++ }, true)
			})
			badge := avatarCaptionShape(t, root, "talk-muted-badge")
			button := avatarCaptionShape(t, root, "talk-fullscreen-switch")
			if badge.Width != 22 || badge.Height != 28 || badge.X < button.X || badge.X+badge.Width > button.X+72 || badge.Y < button.Y || badge.Y+badge.Height > button.Y+72 {
				t.Fatal("badge outside visible native aperture", lang, badge, button)
			}
			for _, id := range []string{"talk-fullscreen-switch", "talk-fullscreen-stop"} {
				if err := w.TestClick(avatarCaptionID(t, w, id)); err != nil {
					t.Fatal(err)
				}
			}
			if switches != 1 || ends != 1 {
				t.Fatal("muted badge intercepted actions")
			}
		}
	}
}
