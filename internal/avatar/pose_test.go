package avatar

import (
	"fmt"
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
func TestPoseLibraryRoundtripAndModelIsolation(t *testing.T) {
	c := poseController(t)
	c.RotateJoint(7, 2, 35)
	if err := c.PoseAction("save", "Wave"); err != nil {
		t.Fatal(err)
	}
	c.RotateJoint(7, 2, -20)
	c.PoseAction("save", "Other")
	c.PoseAction("load", "Wave")
	c.RotateJoint(7, 2, 70)
	c.PoseAction("revert", "")
	if got := c.PoseSnapshot().Rotations["leftUpperArm"][2]; got != 35 {
		t.Fatal(got)
	}
	c.loadPoses(strings.Repeat("b", 64))
	if len(c.pose.Saved) != 0 {
		t.Fatal("cross-model poses")
	}
	c.loadPoses(strings.Repeat("a", 64))
	if len(c.pose.Saved) != 2 {
		t.Fatal("restart lost library")
	}
	c.PoseAction("load", "Wave")
	if c.pose.Rotations["leftUpperArm"][2] != 35 {
		t.Fatal(c.pose)
	}
	c.PoseAction("rest", "")
	if len(c.pose.Rotations) != 0 || c.pose.Name != "" {
		t.Fatal("rest not distinct")
	}
	if err := c.PoseAction("delete", "Other"); err != nil {
		t.Fatal(err)
	}
	c.loadPoses(strings.Repeat("a", 64))
	if len(c.pose.Saved) != 1 {
		t.Fatal("delete not persisted")
	}
	info, err := os.Stat(c.posePath())
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("private store", info, err)
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
	for i := 0; i < 32; i++ {
		if err := c.PoseAction("save", fmt.Sprint(i)); err != nil {
			t.Fatal(err)
		}
	}
	if c.PoseAction("save", "overflow") == nil {
		t.Fatal("library not bounded")
	}
}
