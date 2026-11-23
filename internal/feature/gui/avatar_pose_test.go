//go:build !board_biscuit

package gui

import (
	gogui "github.com/go-gui-org/go-gui/gui"
	"github.com/ygelfand/echolocal/internal/avatar"
	featureshell "github.com/ygelfand/echolocal/internal/feature/shell"
	"github.com/ygelfand/libcountertop/pkg/say"
	"math"
	"strings"
	"testing"
)

type poseTestController struct {
	avatarGestureController
	pose avatar.PoseState
}

func (c *poseTestController) PoseSnapshot() avatar.PoseState { return c.pose }
func (c *poseTestController) PoseMode(on bool)               { c.pose.Enabled = on }
func (c *poseTestController) SelectJoint(id int)             { c.pose.Selected = id }
func (c *poseTestController) RotateJoint(id, axis int, value float64) {
	if c.pose.Enabled {
		r := c.pose.Rotations[avatar.PoseBoneNames[id]]
		r[axis] = value
		c.pose.Rotations[avatar.PoseBoneNames[id]] = r
	}
}
func (*poseTestController) PoseAction(string, string) error { return nil }
func poseGUIFixture(t *testing.T) (*gogui.Window, *avatarView, *poseTestController) {
	c := &poseTestController{avatarGestureController: avatarGestureController{state: avatar.State{Running: true, Ready: true, Options: avatar.Options{Zoom: 1}}}, pose: avatar.PoseState{Enabled: true, Selected: 7, Joints: []avatar.Joint{{ID: 7, X: 260, Y: 220, Visible: true}, {ID: 8, X: 150, Y: 220, Visible: true}}, Rotations: map[string]avatar.Rotation{}}}
	v := &avatarView{app: c, section: 1, posePanel: true}
	featureshell.Get().Push(v)
	w := gogui.NewWindow(gogui.WindowCfg{State: new(int), Width: 1280, Height: 800})
	w.SetView(v.build)
	w.FrameFn()
	t.Cleanup(func() {
		w.MouseCancel()
		if featureshell.Get().Top() == v {
			featureshell.Get().Pop()
		}
	})
	return w, v, c
}
func TestPoseHandleTouchVersusCamera(t *testing.T) {
	w, v, c := poseGUIFixture(t)
	before := c.state.Options
	avatarSwipe(w, 964, 300, 1004, 300)
	if math.Abs(c.pose.Rotations["leftUpperArm"][0]-20) > .01 || c.state.Options != before {
		t.Fatal("handle moved framing or wrong joint", c.pose, c.state.Options)
	}
	avatarSwipe(w, 1100, 550, 1200, 550)
	if c.state.Options != before {
		t.Fatal("pose empty-space drag changed camera")
	}
	avatarSwipe(w, 790, 300, 790, 300)
	if c.pose.Selected != 8 {
		t.Fatal("visible joint selection", c.pose.Selected)
	}
	v.poseCamera = true
	c.PoseMode(false)
	avatarSwipe(w, 900, 400, 1060, 400)
	if math.Abs(c.state.Options.Yaw-45) > .01 || c.pose.Rotations["leftUpperArm"][0] != 20 {
		t.Fatal("camera changed joint", c.state.Options, c.pose)
	}
	v.leavePose()
	avatarSwipe(w, 900, 400, 1060, 400)
	if math.Abs(c.state.Options.Yaw-90) > .01 {
		t.Fatal("exit did not restore framing")
	}
}
func TestPosePanelLocalizationAndUnavailableRig(t *testing.T) {
	old := say.Chosen()
	defer say.Use(old)
	for _, lang := range []string{"en", "de"} {
		say.Use(lang)
		w, _, c := poseGUIFixture(t)
		root := w.TestRender(nil)
		texts := strings.Join(avatarCaptionTexts(root), " ")
		if !strings.Contains(texts, avatarWords("Pose editing", "Pose bearbeiten")) || !strings.Contains(texts, avatarWords("Left Upper arm", "Links Oberarm")) {
			t.Fatal("localization", texts)
		}
		for _, id := range []string{"pose-exit", "pose-camera", "pose-joint-prev", "pose-joint-next", "pose-axis-0", "pose-axis-1", "pose-axis-2", "pose-minus1", "pose-plus1", "pose-rest", "pose-revert", "pose-save"} {
			avatarCaptionShape(t, root, id)
		}
		c.pose.Joints = nil
		texts = strings.Join(avatarCaptionTexts(w.TestRender(nil)), " ")
		if !strings.Contains(texts, avatarWords("No editable humanoid joints.", "Keine bearbeitbaren Humanoid-Gelenke.")) {
			t.Fatal(texts)
		}
		featureshell.Get().Pop()
	}
}
