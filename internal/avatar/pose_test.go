package avatar

import (
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"
)

func poseController(t *testing.T) *Controller {
	t.Helper()
	c := New(t.TempDir(), nil, func() bool { return false }, func() {})
	c.state.Ready = true
	c.generation = 2
	c.loadPoses(strings.Repeat("a", 64))
	c.poseStats(2, "AVATAR_JOINT 7 120 130 1")
	c.PoseMode(true)
	c.SelectJoint(7)
	return c
}
func TestPoseSingleSlotRoundtripAndModelIsolation(t *testing.T) {
	c := poseController(t)
	c.RotateJoint(7, 2, 35)
	if err := c.PoseAction("save", "Pose"); err != nil {
		t.Fatal(err)
	}
	c.RotateJoint(7, 2, -20)
	if err := c.PoseAction("save", "Renamed"); err != nil {
		t.Fatal(err)
	}
	if len(c.pose.Saved) != 1 || c.pose.Saved[0].Name != "Renamed" {
		t.Fatal(c.pose.Saved)
	}
	c.RotateJoint(7, 2, 70)
	c.PoseAction("revert", "")
	if c.pose.Rotations["leftUpperArm"][2] != -20 {
		t.Fatal(c.pose)
	}
	c.loadPoses(strings.Repeat("b", 64))
	if len(c.pose.Saved) != 0 {
		t.Fatal("cross model")
	}
	c.loadPoses(strings.Repeat("a", 64))
	if len(c.pose.Saved) != 1 {
		t.Fatal("restart lost pose")
	}
	c.PoseAction("load", "Renamed")
	if c.pose.Rotations["leftUpperArm"][2] != -20 {
		t.Fatal("wrong persisted pose")
	}
	info, err := os.Stat(c.posePath())
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("private store", info, err)
	}
	c.PoseAction("delete", "Renamed")
	c.loadPoses(strings.Repeat("a", 64))
	if len(c.pose.Saved) != 0 {
		t.Fatal("delete not persisted")
	}
}
func TestPoseHistoryGroupingBranchAndJointReset(t *testing.T) {
	c := poseController(t)
	c.poseStats(2, "AVATAR_JOINT 8 150 130 1")
	framing := c.Snapshot().Options
	c.RotateJoint(8, 1, 15)
	c.PoseAction("begin-edit", "")
	for _, angle := range []float64{10, 20, 30} {
		c.RotateJoint(7, 2, angle)
	}
	c.PoseAction("end-edit", "")
	if len(c.pose.undo) != 2 {
		t.Fatal("drag not grouped", len(c.pose.undo))
	}
	c.PoseAction("undo", "")
	if c.pose.Rotations["leftUpperArm"][2] != 0 || c.pose.Rotations["leftLowerArm"][1] != 15 {
		t.Fatal("undo drag", c.pose)
	}
	c.PoseAction("redo", "")
	if c.pose.Rotations["leftUpperArm"][2] != 30 {
		t.Fatal("redo drag")
	}
	c.PoseAction("reset-joint", "")
	if c.pose.Rotations["leftUpperArm"] != (Rotation{}) || c.pose.Rotations["leftLowerArm"][1] != 15 {
		t.Fatal("reset affected other joint")
	}
	c.PoseAction("undo", "")
	if c.pose.Rotations["leftUpperArm"][2] != 30 {
		t.Fatal("reset undo")
	}
	c.PoseAction("redo", "")
	c.PoseAction("undo", "")
	c.RotateJoint(7, 0, 5)
	if c.PoseSnapshot().CanRedo {
		t.Fatal("new edit retained redo")
	}
	c.PoseAction("save", "Pose")
	c.PoseAction("rest", "")
	c.PoseAction("undo", "")
	if c.pose.Dirty || c.pose.Rotations["leftUpperArm"][0] != 5 {
		t.Fatal("saved baseline not restored")
	}
	c.PoseAction("redo", "")
	c.PoseAction("load", "Pose")
	c.PoseAction("undo", "")
	if len(c.pose.Rotations) != 0 {
		t.Fatal("load not undoable")
	}
	if c.Snapshot().Options != framing {
		t.Fatal("history changed framing")
	}
	for i := 0; i < 150; i++ {
		c.RotateJoint(7, 0, float64(i%80))
	}
	if len(c.pose.undo) != poseHistoryLimit {
		t.Fatal("history unbounded", len(c.pose.undo))
	}
	c.loadPoses(strings.Repeat("a", 64))
	if c.PoseSnapshot().CanUndo || c.PoseSnapshot().CanRedo {
		t.Fatal("history persisted")
	}
}
func TestPoseHistoryNoopAndLegacyPreservation(t *testing.T) {
	c := poseController(t)
	c.RotateJoint(7, 0, 10)
	n := len(c.pose.undo)
	c.RotateJoint(7, 0, 10)
	c.RotateJoint(900, 0, 10)
	if len(c.pose.undo) != n {
		t.Fatal("noop history")
	}
	c.PoseAction("undo", "")
	if !c.PoseSnapshot().CanRedo {
		t.Fatal("missing redo")
	}
	c.RotateJoint(7, 0, 0)
	if !c.PoseSnapshot().CanRedo {
		t.Fatal("noop erased redo")
	}
	legacy := poseFile{1, c.pose.Model, []SavedPose{{"First", map[string]Rotation{}}, {"Second", map[string]Rotation{"head": {1, 2, 3}}}}}
	data, _ := json.Marshal(legacy)
	if err := os.WriteFile(c.posePath(), data, 0600); err != nil {
		t.Fatal(err)
	}
	c.loadPoses(c.pose.Model)
	if err := c.PoseAction("save", "Updated"); err != nil {
		t.Fatal(err)
	}
	if len(c.pose.Saved) != 2 || c.pose.Saved[1].Name != "Second" || c.pose.Saved[1].Bones["head"] != (Rotation{1, 2, 3}) {
		t.Fatal("legacy pose discarded")
	}
}
func TestPoseWireValidationAndFramingSeparation(t *testing.T) {
	c := poseController(t)
	before := c.Snapshot().Options
	c.RotateJoint(7, 0, 999)
	c.RotateJoint(7, 1, math.NaN())
	c.RotateJoint(1, 0, 20)
	if c.pose.Rotations["leftUpperArm"][0] != 90 || len(c.pose.Rotations) != 1 {
		t.Fatal(c.pose.Rotations)
	}
	for _, line := range []string{"AVATAR_JOINT 3 nan 1 1", "AVATAR_JOINT 3 1 1 2", "AVATAR_JOINT 900 1 1 1", "AVATAR_JOINT 3 1 1 1 extra"} {
		c.poseStats(2, line)
	}
	c.poseStats(1, "AVATAR_JOINT 3 1 1 1")
	if len(c.pose.Joints) != 1 {
		t.Fatal("invalid/stale telemetry", c.pose.Joints)
	}
	rev, wire := c.poseLines()
	if rev == 0 || !strings.Contains(wire, "P 7 90.000 0.000 0.000\n") || !strings.Contains(wire, "E 1 7 1\n") {
		t.Fatal(wire)
	}
	c.PoseMode(false)
	c.RotateJoint(7, 0, 20)
	if c.pose.Rotations["leftUpperArm"][0] != 90 || c.Snapshot().Options != before {
		t.Fatal("pose/camera corruption")
	}
	s := c.PoseSnapshot()
	s.Rotations["leftUpperArm"] = Rotation{}
	s.Joints[0].X = 999
	if c.pose.Rotations["leftUpperArm"][0] != 90 || c.pose.Joints[0].X == 999 {
		t.Fatal("mutable snapshot")
	}
}
func TestPoseCorruptStoreAndLimits(t *testing.T) {
	c := poseController(t)
	raw := []byte("{\"Version\":1,\"Model\":\"wrong\",\"Poses\":[]}")
	if err := os.WriteFile(c.posePath(), raw, 0600); err != nil {
		t.Fatal(err)
	}
	c.loadPoses(strings.Repeat("a", 64))
	if c.pose.Error != "pose_read_failed" {
		t.Fatal(c.pose.Error)
	}
	c.PoseAction("rest", "")
	if c.PoseAction("save", "Do not overwrite") == nil {
		t.Fatal("overwrote corrupt library")
	}
	got, _ := os.ReadFile(c.posePath())
	if string(got) != string(raw) {
		t.Fatal("corrupt store changed")
	}
	os.Remove(c.posePath())
	c.loadPoses(strings.Repeat("a", 64))
	for _, name := range []string{"", strings.Repeat("x", 41), "x\n", " x", "x\x00"} {
		if c.PoseAction("save", name) == nil {
			t.Fatal(name)
		}
	}
	if err := c.PoseAction("save", "Valid"); err != nil {
		t.Fatal(err)
	}
}

func TestPoseHistorySnapshotCounts(t *testing.T) {
	c := poseController(t)
	check := func(undo, redo int) {
		t.Helper()
		s := c.PoseSnapshot()
		if s.UndoCount != undo || s.RedoCount != redo || s.CanUndo != (undo > 0) || s.CanRedo != (redo > 0) {
			t.Fatalf("history counts: got %d/%d, want %d/%d", s.UndoCount, s.RedoCount, undo, redo)
		}
	}
	check(0, 0)
	c.PoseAction("begin-edit", "")
	c.RotateJoint(7, 2, 10)
	c.RotateJoint(7, 2, 20)
	check(1, 0)
	c.PoseAction("end-edit", "")
	c.RotateJoint(7, 2, 25)
	check(2, 0)
	c.PoseAction("undo", "")
	check(1, 1)
	c.PoseAction("undo", "")
	check(0, 2)
	c.PoseAction("undo", "")
	check(0, 2)
	c.PoseAction("redo", "")
	check(1, 1)
	c.RotateJoint(7, 2, 30)
	check(2, 0)
	c.PoseAction("reset-joint", "")
	check(3, 0)
	c.PoseAction("reset-joint", "")
	check(3, 0)
	for i := 0; i < 150; i++ {
		c.RotateJoint(7, 0, float64(i%80))
	}
	check(poseHistoryLimit, 0)
	c.loadPoses(c.pose.Model)
	check(0, 0)
}
