//go:build !board_biscuit

package gui

import (
	"fmt"
	gogui "github.com/go-gui-org/go-gui/gui"
	"github.com/ygelfand/echolocal/internal/avatar"
	featureshell "github.com/ygelfand/echolocal/internal/feature/shell"
	"math"
	"strconv"
	"strings"
)

type avatarPoseController interface {
	PoseSnapshot() avatar.PoseState
	PoseMode(bool)
	SelectJoint(int)
	RotateJoint(int, int, float64)
	PoseAction(string, string) error
}

func (v *avatarView) leavePose() {
	if c, ok := v.app.(avatarPoseController); ok {
		c.PoseMode(false)
	}
	v.posePanel = false
	v.poseCamera = false
	v.dragEpoch.Add(1)
	v.touchReset.Add(1)
}
func (v *avatarView) poseHint() string {
	if v.posePanel {
		if c, ok := v.app.(avatarPoseController); ok && len(c.PoseSnapshot().Joints) == 0 {
			return avatarWords("FRAMING · no editable humanoid joints.", "AUSSCHNITT · keine bearbeitbaren Humanoid-Gelenke.")
		}
		if v.poseCamera {
			return avatarWords("CAMERA · drag to orbit; two fingers to pan/pinch.", "KAMERA · Ziehen dreht; zwei Finger verschieben/zoomen.")
		}
		return avatarWords("POSE · tap joint; drag red X / green Y / blue Z handle. Two fingers: camera.", "POSE · Gelenk antippen; roten X- / grünen Y- / blauen Z-Griff ziehen. Zwei Finger: Kamera.")
	}
	return avatarWords("One finger: rotate. Two: move. Pinch: zoom.", "Ein Finger: drehen. Zwei: bewegen. Zusammenziehen: zoomen.")
}
func poseJointName(id int) string {
	if id < 0 || id >= len(avatar.PoseBoneNames) {
		return avatarWords("Select a joint", "Gelenk auswählen")
	}
	s := avatar.PoseBoneNames[id]
	for _, r := range [][3]string{
		{"left", "Left ", "Links "}, {"right", "Right ", "Rechts "},
		{"UpperChest", "Upper chest", "Obere Brust"}, {"upperChest", "Upper chest", "Obere Brust"},
		{"UpperArm", "Upper arm", "Oberarm"}, {"LowerArm", "Forearm", "Unterarm"}, {"UpperLeg", "Thigh", "Oberschenkel"}, {"LowerLeg", "Shin", "Unterschenkel"},
		{"Shoulder", "Shoulder", "Schulter"}, {"Hand", "Hand", "Hand"}, {"Foot", "Foot", "Fuß"}, {"Toes", "Toes", "Zehen"},
		{"hips", "Hips", "Hüfte"}, {"spine", "Spine", "Wirbelsäule"}, {"chest", "Chest", "Brust"}, {"neck", "Neck", "Hals"}, {"head", "Head", "Kopf"},
		{"Thumb", "Thumb ", "Daumen "}, {"Index", "Index ", "Zeigefinger "}, {"Middle", "Middle ", "Mittelfinger "}, {"Ring", "Ring ", "Ringfinger "}, {"Little", "Little ", "Kleiner Finger "},
		{"Metacarpal", "base", "Basis"}, {"Proximal", "proximal", "proximal"}, {"Intermediate", "middle", "Mitte"}, {"Distal", "tip", "Spitze"},
	} {
		s = strings.ReplaceAll(s, r[0], avatarWords(r[1], r[2]))
	}
	return s
}

// Values are stable catalog IDs, independent of translated labels or rig order.
func poseJointDropdown(c avatarPoseController, p avatar.PoseState) gogui.View {
	present := make(map[int]bool, len(p.Joints))
	for _, j := range p.Joints {
		if j.ID >= 0 && j.ID < len(avatar.PoseBoneNames) {
			present[j.ID] = true
		}
	}
	options := make([]gogui.SelectOption, 0, len(present))
	for id := range avatar.PoseBoneNames {
		if present[id] {
			options = append(options, gogui.NewSelectOption(poseJointName(id), strconv.Itoa(id)))
		}
	}
	var selected []string
	if present[p.Selected] {
		selected = []string{strconv.Itoa(p.Selected)}
	}
	st := gogui.CurrentTheme().Cfg.TextStyleDef
	// Select uses text height for option hit targets; keep rows finger-sized.
	st.Size = 36
	st.Color = gogui.Hex(0xEEF3F4)
	return gogui.Select(gogui.SelectCfg{
		ID: "pose-joint", A11YCfg: gogui.A11YCfg{A11YLabel: avatarWords("Joint", "Gelenk")},
		Placeholder: avatarWords("Select a joint", "Gelenk auswählen"),
		Options:     options, Selected: selected, Sizing: gogui.FillFit,
		TextStyle: st, PlaceholderStyle: st, Padding: gogui.PadVH(6, 12),
		Color: gogui.Hex(0x283C42), ColorSelectSubtle: gogui.Hex(0x3E706D),
		FloatZIndex: 100,
		OnSelect: func(values []string, e gogui.EventCtx) {
			if len(values) != 1 {
				return
			}
			id, err := strconv.Atoi(values[0])
			if err != nil || !present[id] {
				return
			}
			c.SelectJoint(id)
			e.Window.InvalidateLayout()
		},
	})
}
func (v *avatarView) poseControls() []gogui.View {
	c, ok := v.app.(avatarPoseController)
	if !ok {
		return nil
	}
	p := c.PoseSnapshot()
	action := func(a, name string) {
		v.poseNotice = ""
		if err := c.PoseAction(a, name); err != nil {
			v.poseNotice = err.Error()
		}
		v.poseDelete = ""
	}
	body := []gogui.View{
		avatarHeading(avatarWords("Pose editing · Lab only", "Pose bearbeiten · nur Labor")),
		avatarRow(avatarButton("pose-exit", avatarWords("Exit pose mode", "Posenmodus verlassen"), false, false, v.leavePose)),
	}
	if !v.app.Snapshot().Ready || len(p.Joints) == 0 {
		return append(body, avatarText(avatarWords("No editable humanoid joints. Start a compatible VRM; framing remains available.", "Keine bearbeitbaren Humanoid-Gelenke. Kompatibles VRM starten; Ausschnitt bleibt bedienbar.")))
	}
	body = append(body, avatarButton("pose-camera", avatarWords("Camera navigation", "Kamera bedienen"), v.poseCamera, false, func() {
		v.poseCamera = !v.poseCamera
		c.PoseMode(!v.poseCamera)
		v.dragEpoch.Add(1)
		v.touchReset.Add(1)
	}), avatarText(v.poseHint()), poseJointDropdown(c, p))
	body = append(body,
		avatarRow(avatarButton("pose-undo", avatarWords("Undo", "Rückgängig"), false, !p.CanUndo, func() { action("undo", "") }), avatarButton("pose-redo", avatarWords("Redo", "Wiederholen"), false, !p.CanRedo, func() { action("redo", "") })),
		avatarButton("pose-reset-joint", avatarWords("Reset selected joint", "Gewähltes Gelenk zurücksetzen"), false, p.Selected < 0 || v.poseCamera, func() { action("reset-joint", "") }))
	axes := []gogui.View{}
	for i, name := range []string{"X · " + avatarWords("red", "rot"), "Y · " + avatarWords("green", "grün"), "Z · " + avatarWords("blue", "blau")} {
		axes = append(axes, avatarButton(fmt.Sprintf("pose-axis-%d", i), name, v.poseAxis == i, false, func() { v.poseAxis = i }))
	}
	body = append(body, avatarRow(axes...))
	angle := 0.
	if p.Selected >= 0 {
		angle = p.Rotations[avatar.PoseBoneNames[p.Selected]][v.poseAxis]
	}
	adjust := func(delta float64) { c.RotateJoint(p.Selected, v.poseAxis, angle+delta) }
	body = append(body, avatarText(fmt.Sprintf("%s · %.1f°  (−90…90°)", []string{"X", "Y", "Z"}[v.poseAxis], angle)),
		avatarRow(avatarButton("pose-minus5", "−5°", false, p.Selected < 0 || v.poseCamera, func() { adjust(-5) }), avatarButton("pose-minus1", "−1°", false, p.Selected < 0 || v.poseCamera, func() { adjust(-1) }), avatarButton("pose-plus1", "+1°", false, p.Selected < 0 || v.poseCamera, func() { adjust(1) }), avatarButton("pose-plus5", "+5°", false, p.Selected < 0 || v.poseCamera, func() { adjust(5) })),
		avatarRow(avatarButton("pose-rest", avatarWords("Rest pose", "Ruhepose"), false, false, func() { action("rest", "") }), avatarButton("pose-revert", avatarWords("Revert edits", "Änderungen verwerfen"), false, false, func() { action("revert", "") })))
	name := "Pose"
	if len(p.Saved) > 0 {
		name = p.Saved[0].Name
	}
	body = append(body, avatarButton("pose-save", avatarWords("Save pose", "Pose speichern"), true, false, func() { action("save", name) }))
	if p.Dirty {
		body = append(body, avatarText(avatarWords("Unsaved pose changes", "Ungespeicherte Posenänderungen")))
	}
	if len(p.Saved) == 0 {
		body = append(body, avatarText(avatarWords("No saved pose for this VRM.", "Keine gespeicherte Pose für dieses VRM.")))
	} else {
		body = append(body, avatarButton("pose-load", avatarWords("Load saved pose", "Gespeicherte Pose laden"), false, false, func() { action("load", name) }))
		label := avatarWords("Delete saved pose", "Gespeicherte Pose löschen")
		if v.poseDelete == name {
			label = avatarWords("Confirm deletion", "Löschen bestätigen")
		}
		body = append(body, avatarButton("pose-delete", label, false, false, func() {
			if v.poseDelete != name {
				v.poseDelete = name
				return
			}
			action("delete", name)
		}))
	}
	errCode := v.poseNotice
	if errCode == "" {
		errCode = p.Error
	}
	if errCode != "" {
		messages := map[string][2]string{
			"pose_name_invalid": {"Use a unique name of 1–40 characters without control characters.", "Namen mit 1–40 Zeichen ohne Steuerzeichen verwenden."},
			"pose_read_failed":  {"Pose library could not be read. It has not been overwritten.", "Posenbibliothek nicht lesbar. Sie wurde nicht überschrieben."},
			"pose_save_failed":  {"Could not save pose. Check device storage and retry.", "Pose nicht gespeichert. Gerätespeicher prüfen und erneut versuchen."},
			"pose_missing":      {"Pose not found.", "Pose nicht gefunden."},
		}
		m, ok := messages[errCode]
		if !ok {
			m = [2]string{"Pose action failed.", "Posenaktion fehlgeschlagen."}
		}
		body = append(body, avatarText(avatarWords(m[0], m[1])))
	}
	return body
}

// Hit targets match native overlay geometry, in physical viewport pixels.
// Axes are local joint Euler components; the screen handles are deliberately
// fixed-direction controls, not misleading projected world-axis rotations.
func (v *avatarView) beginPoseDrag(e gogui.EventCtx, vp avatar.Viewport, x, y float64) {
	c, ok := v.app.(avatarPoseController)
	if !ok {
		return
	}
	p := c.PoseSnapshot()
	axis := -1
	id := p.Selected
	for _, j := range p.Joints {
		if j.ID != id || !j.Visible {
			continue
		}
		for a, d := range [][2]float64{{64, 0}, {0, -64}, {-45, 45}} {
			if math.Hypot(x-float64(vp.X)-j.X-d[0], y-float64(vp.Y)-j.Y-d[1]) <= 26 {
				axis = a
				break
			}
		}
	}
	e.Consume()
	if axis < 0 {
		best := 27.
		for _, j := range p.Joints {
			if j.Visible {
				distance := math.Hypot(x-float64(vp.X)-j.X, y-float64(vp.Y)-j.Y)
				if distance < best {
					best = distance
					id = j.ID
				}
			}
		}
		if best < 27 {
			c.SelectJoint(id)
			e.Window.InvalidateLayout()
		}
		return
	}
	v.poseAxis = axis
	epoch := v.dragEpoch.Add(1)
	decided := false
	origin := p.Rotations[avatar.PoseBoneNames[id]][axis]
	_ = c.PoseAction("begin-edit", "")
	end := func() {
		v.dragEpoch.CompareAndSwap(epoch, epoch+1)
		_ = c.PoseAction("end-edit", "")
	}
	move := func(m gogui.EventCtx) {
		if m.Event == nil || v.touchMulti || v.dragEpoch.Load() != epoch || !featureshell.Get().Visible(v) {
			return
		}
		dx, dy := float64(m.Event.MouseX)-x, float64(m.Event.MouseY)-y
		if !decided {
			if math.Hypot(dx, dy) < 6 {
				return
			}
			decided = true
		}
		if !finitePoseDrag(dx, dy) {
			return
		}
		// 2 physical pixels per degree; bounded by shared model-safe limits.
		c.RotateJoint(id, axis, origin+(dx-dy)*.5)
		m.Window.InvalidateLayout()
	}
	e.Window.MouseLock(gogui.MouseLockCfg{MouseMove: move, MouseUp: func(m gogui.EventCtx) { move(m); m.Window.MouseUnlock(); end() }, Cancel: func(*gogui.Window) { end() }})
}
func finitePoseDrag(x, y float64) bool {
	return !math.IsNaN(x) && !math.IsNaN(y) && !math.IsInf(x, 0) && !math.IsInf(y, 0)
}
