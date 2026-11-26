//go:build !board_biscuit

package gui

import (
	"fmt"
	gogui "github.com/go-gui-org/go-gui/gui"
	"github.com/ygelfand/echolocal/internal/avatar"
	"github.com/ygelfand/libcountertop/pkg/say"
	"testing"
)

type motionTestController struct {
	avatarGestureController
	motions   avatar.MotionState
	refreshes int
}

func (c *motionTestController) Motions() avatar.MotionState { return c.motions }
func (c *motionTestController) RefreshMotions()             { c.refreshes++ }
func (c *motionTestController) SelectMotion(name string) {
	c.motions.Selected = name
	c.motions.Status = "motion_ready"
	c.motions.Playing = true
}
func (c *motionTestController) PlayMotion(on bool) { c.motions.Playing = on }
func TestAvatarMotionDropdownTouchAndLocalization(t *testing.T) {
	old := say.Chosen()
	defer say.Use(old)
	for _, lang := range []string{"en", "de", "it", "vi"} {
		say.Use(lang)
		for _, size := range [][2]int{{1280, 800}, {800, 480}} {
			t.Run(fmt.Sprintf("%s-%dx%d", lang, size[0], size[1]), func(t *testing.T) {
				c := &motionTestController{motions: avatar.MotionState{Entries: []avatar.MotionEntry{{File: "A turn.vrma", Name: "A turn.vrma"}, {File: "B bounce.vrma", Name: "B bounce.vrma"}}}}
				c.state = avatar.State{Running: true, Ready: true, Options: avatar.Options{Zoom: 2, PanY: .5}}
				v := &avatarView{app: c}
				w := gogui.NewWindow(gogui.WindowCfg{State: new(int), Width: size[0], Height: size[1]})
				w.SetView(v.build)
				root := w.TestRender(nil)
				if c.refreshes != 1 {
					t.Fatal("opening did not refresh")
				}
				if poseTextShape(root, avatar.Text(lang, "motion_label")) == nil {
					t.Fatal("untranslated heading")
				}
				field := avatarCaptionShape(t, root, "avatar-motion")
				if field.Height < 48 || field.X+field.Width > float32(size[0]/2) {
					t.Fatal("dropdown clipped", field)
				}
				if e := w.TestClick(avatarCaptionID(t, w, "avatar-motion")); e != nil {
					t.Fatal(e)
				}
				root = w.TestRender(nil)
				target := poseTextShape(root, "B bounce.vrma")
				if target == nil {
					t.Fatal("missing option")
				}
				if target.Y < 0 || target.Y+target.Height > float32(size[1]) {
					ids := w.ResolveID("avatar-app:avatar-workspace:vrm-controls-0:avatar-motion:dropdown")
					if len(ids) != 1 {
						t.Fatal(ids)
					}
					w.TestScroll(ids[0], 0, -10000)
					root = w.TestRender(nil)
					target = poseTextShape(root, "B bounce.vrma")
				}
				before := c.state.Options
				avatarSwipe(w, target.X+target.Width/2, target.Y+target.Height/2, target.X+target.Width/2, target.Y+target.Height/2)
				if c.motions.Selected != "B bounce.vrma" || c.state.Options != before {
					t.Fatal("selection failed or changed framing", c.motions)
				}
				w.TestRender(nil)
				if c.refreshes != 1 {
					t.Fatal("repeated scan")
				}
				v.Close()
				v.build(w)
				if c.refreshes != 2 {
					t.Fatal("reopen did not scan")
				}
			})
		}
	}
}
