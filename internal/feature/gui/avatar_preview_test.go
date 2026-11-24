//go:build !board_biscuit

package gui

import (
	"fmt"
	gogui "github.com/go-gui-org/go-gui/gui"
	"github.com/ygelfand/echolocal/internal/avatar"
	"github.com/ygelfand/echolocal/internal/feature/clawtalk"
	"github.com/ygelfand/libcountertop/pkg/say"
	"testing"
)

func TestPreviewGeometryExitAndLabRestore(t *testing.T) {
	old := say.Chosen()
	defer say.Use(old)
	for _, lang := range []string{"en", "de", "it", "vi"} {
		say.Use(lang)
		for _, size := range [][2]int{{1280, 800}, {800, 480}} {
			t.Run(fmt.Sprintf("%s-%d", lang, size[0]), func(t *testing.T) {
				options := avatar.Options{Zoom: 9.99, Yaw: 33, Pitch: 20, PanX: 2, PanY: 3, Mouth: avatar.MouthMicrophone, Idle: true}
				lab := &avatarGestureController{state: avatar.State{Options: options, Running: true}}
				v := &avatarView{app: lab}
				c := avatar.New(t.TempDir(), nil, func() bool { return false }, func() {})
				c.Configure(avatar.Options{Zoom: 9.99})
				p := &avatarPreview{renderer: c, lifecycle: &clawtalk.AvatarPage{Renderer: c}, lab: v, options: options, running: true}
				w := gogui.NewWindow(gogui.WindowCfg{State: new(int), Width: size[0], Height: size[1]})
				root := w.TestRender(p.build)
				canvas := avatarCaptionShape(t, root, "talk-avatar-canvas")
				vp := talkAvatarViewport(size[0], size[1])
				if canvas.X != float32(vp.X) || canvas.Y != 0 || canvas.Width != float32(vp.Width) || canvas.Height != float32(vp.Height) {
					t.Fatal("preview differs from Talk", canvas, vp)
				}
				exit := avatarCaptionShape(t, root, "avatar-preview-exit")
				if exit.Height < 64 || exit.Y < 0 || exit.Y+exit.Height > float32(size[1]) || exit.X+exit.Width > float32(size[0]/2) {
					t.Fatal("exit not accessible", exit)
				}
				p.Close()
				if lab.state.Options != options || !v.loaded.Load() || !v.resume.Load() {
					t.Fatal("unsaved lab state lost")
				}
			})
		}
	}
}
