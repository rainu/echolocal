//go:build !board_biscuit

package gui

import (
	"fmt"
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
	pose    avatar.PoseState
	actions []string
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
func (c *poseTestController) PoseAction(action, name string) error {
	c.actions = append(c.actions, action)
	return nil
}
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
	if len(c.actions) != 2 || c.actions[0] != "begin-edit" || c.actions[1] != "end-edit" {
		t.Fatal("drag transaction", c.actions)
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
		w, v, c := poseGUIFixture(t)
		root := w.TestRender(nil)
		texts := strings.Join(avatarCaptionTexts(root), " ")
		if !strings.Contains(texts, avatarWords("Pose editing", "Pose bearbeiten")) || !strings.Contains(texts, avatarWords("Left Upper arm", "Links Oberarm")) {
			t.Fatal("localization", texts)
		}
		for _, id := range []string{"pose-exit", "pose-camera", "pose-joint", "pose-axis-0", "pose-axis-1", "pose-axis-2", "pose-minus1", "pose-plus1", "pose-rest", "pose-revert", "pose-save", "pose-undo", "pose-redo", "pose-reset-joint"} {
			avatarCaptionShape(t, root, id)
		}
		c.pose.Joints = nil
		if !strings.Contains(v.poseHint(), avatarWords("FRAMING", "AUSSCHNITT")) {
			t.Fatal("unsupported rig mislabeled as joint editing")
		}
		texts = strings.Join(avatarCaptionTexts(w.TestRender(nil)), " ")
		if !strings.Contains(texts, avatarWords("No editable humanoid joints.", "Keine bearbeitbaren Humanoid-Gelenke.")) {
			t.Fatal(texts)
		}
		featureshell.Get().Pop()
	}
}

func TestPoseSingleSlotAndHistoryControls(t *testing.T) {
	w, _, c := poseGUIFixture(t)
	c.pose.CanUndo, c.pose.CanRedo = true, true
	c.pose.Saved = []avatar.SavedPose{{Name: "Existing"}}
	w.InvalidateLayout()
	root := w.TestRender(nil)
	for _, id := range []string{"pose-undo", "pose-redo", "pose-reset-joint", "pose-load", "pose-save"} {
		root = w.TestRender(nil)
		field := avatarCaptionShape(t, root, id)
		scroller := avatarCaptionShape(t, root, "vrm-controls-1")
		w.TestScroll(avatarCaptionID(t, w, "vrm-controls-1"), 0, (scroller.Y+scroller.Height/2-field.Y-field.Height/2)/gogui.CurrentTheme().ScrollMultiplier)
		w.TestRender(nil)
		if err := w.TestClick(avatarCaptionID(t, w, id)); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Join(c.actions, ",") != "undo,redo,reset-joint,load,save" {
		t.Fatal(c.actions)
	}
	if strings.Contains(strings.Join(avatarCaptionTexts(root), " "), "Save named pose") {
		t.Fatal("old library UI")
	}
}

func poseTextShape(root *gogui.Layout, text string) *gogui.Shape {
	if root.Shape != nil && root.Shape.TC != nil && root.Shape.TC.Text == text {
		return root.Shape
	}
	for i := range root.Children {
		if s := poseTextShape(&root.Children[i], text); s != nil {
			return s
		}
	}
	return nil
}
func TestPoseJointDropdownSelectionAndModelSync(t *testing.T) {
	old := say.Chosen()
	defer say.Use(old)
	for _, lang := range []string{"en", "de"} {
		say.Use(lang)
		w, _, c := poseGUIFixture(t)
		// Unordered, sparse rig IDs must not become option indices.
		c.pose.Joints = []avatar.Joint{{ID: 8, X: 150, Y: 220, Visible: true}, {ID: 5, X: 100, Y: 100, Visible: true}, {ID: 7, X: 260, Y: 220, Visible: true}}
		c.pose.Selected = -1
		w.InvalidateLayout()
		root := w.TestRender(nil)
		if !strings.Contains(strings.Join(avatarCaptionTexts(root), " "), avatarWords("Select a joint", "Gelenk auswählen")) {
			t.Fatal("missing placeholder")
		}
		field := avatarCaptionShape(t, root, "pose-joint")
		if field.Height < 48 || field.X+field.Width > 640 {
			t.Fatal("dropdown field bounds", field)
		}
		if err := w.TestClick(avatarCaptionID(t, w, "pose-joint")); err != nil {
			t.Fatal(err)
		}
		root = w.TestRender(nil)
		for _, id := range []int{5, 7, 8} {
			if poseTextShape(root, poseJointName(id)) == nil {
				t.Fatal("missing rig option", id)
			}
		}
		if poseTextShape(root, poseJointName(9)) != nil {
			t.Fatal("missing bone offered")
		}
		target := poseTextShape(root, poseJointName(8))
		before := c.state.Options
		avatarSwipe(w, target.X+target.Width/2, target.Y+target.Height/2, target.X+target.Width/2, target.Y+target.Height/2)
		if c.pose.Selected != 8 || c.state.Options != before || len(c.actions) != 0 {
			t.Fatal("selection changed pose/framing/history", c.pose.Selected, c.actions)
		}
		root = w.TestRender(nil)
		if poseTextShape(root, poseJointName(8)) == nil {
			t.Fatal("selected field out of sync")
		}
		// A tap on the model closes an open dropdown and updates its field.
		if err := w.TestClick(avatarCaptionID(t, w, "pose-joint")); err != nil {
			t.Fatal(err)
		}
		w.TestRender(nil)
		avatarSwipe(w, 740, 180, 740, 180)
		for range 3 {
			w.FrameFn()
		}
		if ids := w.ResolveID("avatar-app:avatar-workspace:vrm-controls-1:pose-joint:dropdown"); len(ids) != 0 {
			t.Fatal("model tap left dropdown open", ids)
		}
		if c.pose.Selected != 5 {
			t.Fatal("on-model head not selected", c.pose.Selected)
		}
		root = w.TestRender(nil)
		if poseTextShape(root, poseJointName(5)) == nil || poseTextShape(root, poseJointName(8)) != nil {
			t.Fatal("external selection stale")
		}
		featureshell.Get().Pop()
	}
}
func TestPoseJointDropdownScrollsFullRig(t *testing.T) {
	w, _, c := poseGUIFixture(t)
	c.pose.Joints = nil
	for id := range avatar.PoseBoneNames {
		c.pose.Joints = append(c.pose.Joints, avatar.Joint{ID: id})
	}
	c.pose.Selected = -1
	w.InvalidateLayout()
	w.TestRender(nil)
	if err := w.TestClick(avatarCaptionID(t, w, "pose-joint")); err != nil {
		t.Fatal(err)
	}
	w.TestRender(nil)
	ids := w.ResolveID("avatar-app:avatar-workspace:vrm-controls-1:pose-joint:dropdown")
	if len(ids) != 1 {
		t.Fatal("missing dropdown scroller", ids)
	}
	w.TestScroll(ids[0], 0, -10000)
	root := w.TestRender(nil)
	target := poseTextShape(root, poseJointName(len(avatar.PoseBoneNames)-1))
	if target == nil {
		t.Fatal("last bone missing")
	}
	if target.Y < 0 || target.Y+target.Height > 800 {
		t.Fatal("last bone not scrolled into view", target.Y)
	}
	avatarSwipe(w, target.X+target.Width/2, target.Y+target.Height/2, target.X+target.Width/2, target.Y+target.Height/2)
	if c.pose.Selected != len(avatar.PoseBoneNames)-1 {
		t.Fatal("last bone not selectable", c.pose.Selected)
	}
}

func TestPoseHistoryCountLabels(t *testing.T) {
	old := say.Chosen()
	defer say.Use(old)
	for _, lang := range []string{"en", "de"} {
		say.Use(lang)
		w, _, c := poseGUIFixture(t)
		for _, counts := range [][2]int{{0, 0}, {3, 0}, {2, 1}, {0, 3}, {100, 0}} {
			c.pose.UndoCount, c.pose.RedoCount = counts[0], counts[1]
			c.pose.CanUndo, c.pose.CanRedo = counts[0] > 0, counts[1] > 0
			w.InvalidateLayout()
			root := w.TestRender(nil)
			for i, label := range []string{avatarWords("Undo", "Rückgängig"), avatarWords("Redo", "Wiederholen")} {
				if poseTextShape(root, fmt.Sprintf("%s (%d)", label, counts[i])) == nil {
					t.Fatalf("%s: missing updated count %s (%d)", lang, label, counts[i])
				}
			}
		}
		featureshell.Get().Pop()
	}
}
