# Avatar Lab pose editor

Experimental Athena Avatar Lab only; poses do not automatically apply to Talk.

Start a compatible VRM, open **Framing → Edit pose…** (**Ausschnitt → Pose
bearbeiten…**). The heading and preview hint identify POSE versus CAMERA mode.
Tap a visible yellow joint marker, then drag a red X, green Y or blue Z handle.
These fixed screen-direction handles edit **local joint Euler rotation
components**, relative to the authored rest rotation, in X/Y/Z multiplication
order. They are not projected world axes or an inverse-kinematics solver.
One degree requires two physical pixels of travel; each component is bounded
to ±90°. Fine controls provide ±1° and ±5° adjustments. They are conservative
limits, not a model-specific anatomical collision or joint-limit system.

Only active, TRS humanoid nodes actually present in the loaded VRM are offered.
Head, neck, torso, shoulders, arms, hands, hips, legs, feet, toes and finger
segments are supported; VRM 0 thumb intermediate and VRM 1 thumb metacarpal
names are included. Eye/jaw expression controls, matrix-only joints and rig
creation are outside this editor. An empty/incompatible rig explains the
limitation while ordinary framing remains available. A valid default VRMA with
no matching target bones becomes a no-op rather than preventing model display. Dense or overlapping
markers can be disambiguated with the previous/next joint controls.

Two fingers still pan/pinch the camera. **Camera navigation** temporarily
restores one-finger orbit without changing the pose; switch it off to edit
handles again. **Exit pose mode** restores normal framing controls. Camera
framing and skeleton rotations use separate state. While a manual pose is
displayed, idle/VRMA and node constraints are suspended so they cannot overwrite
joint edits; blink/mouth controls remain independent. The pose remains displayed
after leaving manipulation until Stop/Start or an explicit rest/load action.

**Undo / Redo** (**Rückgängig / Wiederholen**) step through this Lab session's
pose edits; one handle drag is one step, each fine adjustment is one step.
Rest/load/revert and selected-joint resets are also undoable. A new edit after
Undo discards the redo branch; no-op edits, camera navigation and saving do not.
History retains the latest 100 steps and is cleared when the renderer/model
session restarts; it is not persisted.

**Reset selected joint** clears all three local rotation components of only the
selected joint to its authored rest rotation. Other joints and framing stay
unchanged. **Rest pose** resets the whole skeleton. **Revert edits** reloads the
saved pose (or rest when none is saved).

There is one visible saved pose per model: **Save pose** updates it,
**Load saved pose** restores it, and deletion requires confirmation.
No pose naming or multiple-pose library is exposed yet. Older multi-entry files
remain readable and extra legacy entries are preserved when saving; the UI
shows the first entry. Saving/deleting is persistent and is not undone by
Undo/Redo, which only changes the in-session pose draft.

The library uses a versioned, bounded JSON file per loaded VRM SHA-256 digest,
not its display name or path. Identical model bytes share a library; replacing a
file with different contents selects a different library. A new Lab renderer
starts at its normal presentation; the saved pose is available to load again.
Libraries survive service/device restart. Writes use mode 0600, fsync and atomic
rename plus directory sync. Corrupt/incompatible libraries fail closed and cannot
be overwritten by Save/Delete in that run. No existing settings migration or
Talk pose activation is introduced.

The existing scalar control protocol gains bounded opt-in lines:
P ID X Y Z sets a catalog bone's local delta in degrees, and
E EDIT SELECTED ACTIVE toggles handles/manual display. Existing S/T controls
are unchanged. The renderer emits bounded AVATAR_JOINT ID X Y VISIBLE
telemetry at 10 Hz in Lab only. Coordinates are viewport-local physical pixels,
top-left origin, projected using the same matrix as the skinned mesh. Both
sides reject invalid IDs, extra fields, non-finite numbers and out-of-range
values. K HIDDEN makes the Lab surface transparent while the full-width native keyboard
is open, without restarting rendering or losing the draft pose. Talk never uses this command. The native overlay owns no input or external resources.
