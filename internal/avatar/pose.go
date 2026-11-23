package avatar

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// PoseBoneNames is a fixed wire catalog, not a claim that any particular rig has every bone.
var PoseBoneNames = []string{"hips", "spine", "chest", "upperChest", "neck", "head", "leftShoulder", "leftUpperArm", "leftLowerArm", "leftHand", "rightShoulder", "rightUpperArm", "rightLowerArm", "rightHand", "leftUpperLeg", "leftLowerLeg", "leftFoot", "leftToes", "rightUpperLeg", "rightLowerLeg", "rightFoot", "rightToes", "leftThumbMetacarpal", "leftThumbProximal", "leftThumbDistal", "leftIndexProximal", "leftIndexIntermediate", "leftIndexDistal", "leftMiddleProximal", "leftMiddleIntermediate", "leftMiddleDistal", "leftRingProximal", "leftRingIntermediate", "leftRingDistal", "leftLittleProximal", "leftLittleIntermediate", "leftLittleDistal", "rightThumbMetacarpal", "rightThumbProximal", "rightThumbDistal", "rightIndexProximal", "rightIndexIntermediate", "rightIndexDistal", "rightMiddleProximal", "rightMiddleIntermediate", "rightMiddleDistal", "rightRingProximal", "rightRingIntermediate", "rightRingDistal", "rightLittleProximal", "rightLittleIntermediate", "rightLittleDistal", "leftThumbIntermediate", "rightThumbIntermediate"}

type Rotation [3]float64
type Joint struct {
	ID      int
	X, Y    float64
	Visible bool
}
type SavedPose struct {
	Name  string
	Bones map[string]Rotation
}
type PoseState struct {
	Model                  string
	Enabled, Active, Dirty bool
	Selected               int
	Joints                 []Joint
	Rotations              map[string]Rotation
	Saved                  []SavedPose
	Name, Error            string
	CanUndo, CanRedo       bool
	undo, redo             []poseEdit
	grouping, grouped      bool
	readFailed             bool
	revision               uint64
}

// History is transient and bounded; saved poses and framing are not undo state.
type poseEdit struct {
	Bones  map[string]Rotation
	Name   string
	Active bool
}

const poseHistoryLimit = 100

func (c *Controller) poseEdit() poseEdit {
	return poseEdit{copyRotations(c.pose.Rotations), c.pose.Name, c.pose.Active}
}
func pushPoseEdit(stack []poseEdit, edit poseEdit) []poseEdit {
	if len(stack) == poseHistoryLimit {
		stack = stack[1:]
	}
	return append(stack, edit)
}
func (c *Controller) recordPoseEdit(before poseEdit) {
	if maps.Equal(before.Bones, c.pose.Rotations) && before.Name == c.pose.Name && before.Active == c.pose.Active {
		return
	}
	if !c.pose.grouping || !c.pose.grouped {
		c.pose.undo = pushPoseEdit(c.pose.undo, before)
		c.pose.redo = nil
		c.pose.grouped = c.pose.grouping
	}
}
func (c *Controller) restorePoseEdit(edit poseEdit) {
	c.pose.Rotations = copyRotations(edit.Bones)
	c.pose.Name, c.pose.Active = edit.Name, edit.Active
	c.pose.Dirty = len(c.pose.Rotations) != 0
	if len(c.pose.Saved) > 0 {
		c.pose.Dirty = !maps.Equal(c.pose.Saved[0].Bones, c.pose.Rotations)
	}
	for _, saved := range c.pose.Saved {
		if saved.Name == c.pose.Name {
			c.pose.Dirty = !maps.Equal(saved.Bones, c.pose.Rotations)
			break
		}
	}
}
func copyRotations(in map[string]Rotation) map[string]Rotation {
	out := map[string]Rotation{}
	for k, v := range in {
		out[k] = v
	}
	return out
}
func (c *Controller) PoseSnapshot() PoseState {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.pose
	s.CanUndo, s.CanRedo = len(s.undo) > 0, len(s.redo) > 0
	s.undo, s.redo = nil, nil
	s.Joints = append([]Joint(nil), s.Joints...)
	s.Rotations = copyRotations(s.Rotations)
	s.Saved = append([]SavedPose(nil), s.Saved...)
	for i := range s.Saved {
		s.Saved[i].Bones = copyRotations(s.Saved[i].Bones)
	}
	return s
}
func validPoseName(s string) bool {
	if s == "" || s != strings.TrimSpace(s) || utf8.RuneCountInString(s) > 40 || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func validRotations(b map[string]Rotation) bool {
	if len(b) > len(PoseBoneNames) {
		return false
	}
	for name, r := range b {
		found := false
		for _, n := range PoseBoneNames {
			if n == name {
				found = true
				break
			}
		}
		if !found {
			return false
		}
		for _, a := range r {
			if !finite(a) || math.Abs(a) > 90 {
				return false
			}
		}
	}
	return true
}
func (c *Controller) posePath() string { return filepath.Join(c.dir, "poses-"+c.pose.Model+".json") }

type poseFile struct {
	Version int
	Model   string
	Poses   []SavedPose
}

func (c *Controller) loadPoses(model string) {
	c.pose = PoseState{Model: model, Selected: -1, Rotations: map[string]Rotation{}, revision: c.pose.revision + 1}
	if !validDigest(model) {
		return
	}
	f, e := os.Open(c.posePath())
	if os.IsNotExist(e) {
		return
	}
	if e != nil {
		c.pose.Error = "pose_read_failed"
		c.pose.readFailed = true
		return
	}
	defer f.Close()
	var file poseFile
	d := json.NewDecoder(io.LimitReader(f, 256<<10))
	d.DisallowUnknownFields()
	var extra any
	if d.Decode(&file) != nil || d.Decode(&extra) != io.EOF || file.Version != 1 || file.Model != model || len(file.Poses) > 32 {
		c.pose.Error = "pose_read_failed"
		c.pose.readFailed = true
		return
	}
	seen := map[string]bool{}
	for _, s := range file.Poses {
		if !validPoseName(s.Name) || !validRotations(s.Bones) || seen[s.Name] {
			c.pose.Error = "pose_read_failed"
			c.pose.readFailed = true
			return
		}
		seen[s.Name] = true
	}
	c.pose.Saved = file.Poses
}
func (c *Controller) writePoses() error {
	if !validDigest(c.pose.Model) {
		return errors.New("pose_save_failed")
	}
	data, e := json.Marshal(poseFile{1, c.pose.Model, c.pose.Saved})
	if e != nil {
		return e
	}
	if e = os.MkdirAll(c.dir, 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(c.dir, ".poses-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(data); e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e == nil {
		e = closeErr
	}
	if e != nil {
		return e
	}
	if e = os.Rename(f.Name(), c.posePath()); e != nil {
		return e
	}
	// Persist the rename as well as file contents before reporting success.
	dir, e := os.Open(c.dir)
	if e != nil {
		return e
	}
	defer dir.Close()
	return dir.Sync()
}
func (c *Controller) PoseMode(on bool) {
	c.mu.Lock()
	if !on || (c.state.Ready && len(c.pose.Joints) > 0) {
		c.pose.Enabled = on
		if on {
			c.pose.Active = true
		}
		c.pose.revision++
	}
	c.mu.Unlock()
	c.redraw()
}
func (c *Controller) SelectJoint(id int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, j := range c.pose.Joints {
		if j.ID == id {
			c.pose.Selected = id
			c.pose.revision++
			return
		}
	}
}
func (c *Controller) RotateJoint(id, axis int, value float64) {
	if id < 0 || id >= len(PoseBoneNames) || axis < 0 || axis > 2 || !finite(value) {
		return
	}
	c.mu.Lock()
	if c.pose.Enabled {
		for _, j := range c.pose.Joints {
			if j.ID == id {
				before := c.poseEdit()
				r := c.pose.Rotations[PoseBoneNames[id]]
				r[axis] = math.Max(-90, math.Min(90, value))
				if r == c.pose.Rotations[PoseBoneNames[id]] {
					break
				}
				if r == (Rotation{}) {
					delete(c.pose.Rotations, PoseBoneNames[id])
				} else {
					c.pose.Rotations[PoseBoneNames[id]] = r
				}
				c.recordPoseEdit(before)
				c.pose.Dirty = true
				c.pose.Active = true
				c.pose.revision++
				break
			}
		}
	}
	c.mu.Unlock()
	c.redraw()
}

// PoseAction changes only Lab state/library, never Talk or model/framing settings.
// Save updates the single current slot. Legacy additional entries are preserved.
func (c *Controller) PoseAction(action, name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	fail := func(code string) error { c.pose.Error = code; return errors.New(code) }
	if c.pose.readFailed && (action == "save" || action == "delete") {
		return errors.New("pose_read_failed")
	}
	c.pose.Error = ""
	before := c.poseEdit()
	if action != "begin-edit" && action != "end-edit" {
		c.pose.grouping, c.pose.grouped = false, false
	}
	switch action {
	case "begin-edit":
		c.pose.grouping, c.pose.grouped = true, false
		return nil
	case "end-edit":
		c.pose.grouping, c.pose.grouped = false, false
		return nil
	case "undo", "redo":
		from, to := &c.pose.undo, &c.pose.redo
		if action == "redo" {
			from, to = to, from
		}
		if len(*from) == 0 {
			return nil
		}
		edit := (*from)[len(*from)-1]
		*from = (*from)[:len(*from)-1]
		*to = pushPoseEdit(*to, before)
		c.restorePoseEdit(edit)
	case "reset-joint":
		if c.pose.Selected < 0 || c.pose.Selected >= len(PoseBoneNames) {
			return nil
		}
		found := false
		for _, j := range c.pose.Joints {
			if j.ID == c.pose.Selected {
				found = true
				break
			}
		}
		if !found {
			return nil
		}
		delete(c.pose.Rotations, PoseBoneNames[c.pose.Selected])
		c.pose.Dirty, c.pose.Active = true, true
	case "rest":
		c.pose.Rotations = map[string]Rotation{}
		c.pose.Name = ""
		c.pose.Dirty = true
		c.pose.Active = true
	case "load", "revert":
		if action == "revert" {
			name = ""
			if len(c.pose.Saved) > 0 {
				name = c.pose.Saved[0].Name
			}
		}
		found := false
		for _, s := range c.pose.Saved {
			if s.Name == name {
				c.pose.Rotations = copyRotations(s.Bones)
				c.pose.Name = name
				found = true
				break
			}
		}
		if !found {
			if name != "" {
				return fail("pose_missing")
			}
			c.pose.Rotations = map[string]Rotation{}
			c.pose.Name = ""
		}
		c.pose.Dirty = false
		c.pose.Active = true
	case "save", "delete":
		if !validPoseName(name) {
			return fail("pose_name_invalid")
		}
		old := append([]SavedPose(nil), c.pose.Saved...)
		index := -1
		for i, s := range c.pose.Saved {
			if s.Name == name {
				index = i
				break
			}
		}
		if action == "delete" {
			if index < 0 {
				return fail("pose_missing")
			}
			c.pose.Saved = append(c.pose.Saved[:index:index], c.pose.Saved[index+1:]...)
		} else {
			if index < 0 {
				if len(c.pose.Saved) == 0 {
					c.pose.Saved = append(c.pose.Saved, SavedPose{})
				}
				index = 0
			}
			c.pose.Saved[index] = SavedPose{name, copyRotations(c.pose.Rotations)}
		}
		if e := c.writePoses(); e != nil {
			c.pose.Saved = old
			return fail("pose_save_failed")
		}
		if action == "save" {
			c.pose.Name = name
			c.pose.Dirty = false
		} else if c.pose.Name == name {
			c.pose.Name = ""
			c.pose.Dirty = true
		}
	default:
		return fail("pose_invalid")
	}
	if action == "rest" || action == "reset-joint" || action == "load" || action == "revert" {
		c.recordPoseEdit(before)
	}
	c.pose.revision++
	return nil
}
func (c *Controller) poseLines() (uint64, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var b strings.Builder
	for i, name := range PoseBoneNames {
		r := c.pose.Rotations[name]
		fmt.Fprintf(&b, "P %d %.3f %.3f %.3f\n", i, r[0], r[1], r[2])
	}
	fmt.Fprintf(&b, "E %d %d %d\n", bit(c.pose.Enabled), c.pose.Selected, bit(c.pose.Active))
	return c.pose.revision, b.String()
}
func (c *Controller) poseStats(gen uint64, line string) bool {
	if !strings.HasPrefix(line, "AVATAR_JOINT ") {
		return false
	}
	var j Joint
	var visible int
	var extra string
	n, e := fmt.Sscanf(line, "AVATAR_JOINT %d %f %f %d %s", &j.ID, &j.X, &j.Y, &visible, &extra)
	if n != 4 || (e != nil && e != io.EOF) || j.ID < 0 || j.ID >= len(PoseBoneNames) || !finite(j.X) || !finite(j.Y) || math.Abs(j.X) > 1e6 || math.Abs(j.Y) > 1e6 || visible < 0 || visible > 1 {
		return true
	}
	j.Visible = visible == 1
	c.mu.Lock()
	defer c.mu.Unlock()
	if gen != c.generation {
		return true
	}
	for i, v := range c.pose.Joints {
		if v.ID == j.ID {
			c.pose.Joints[i] = j
			return true
		}
	}
	c.pose.Joints = append(c.pose.Joints, j)
	return true
}
