# Native avatar renderer

Separate **AGPL-3.0-only** executable for Athena/API 28 and OpenGL ES 3. It is
bundled with the daemon as an executable payload, not linked into the Go binary.
`LICENSE` and `THIRD_PARTY.md` ship alongside it. Models are not bundled.

## Build

From the project root: `make build-avatar NDK=/path/to/android-ndk`.
`NDK_HOST` follows the existing project convention. `AVATAR_CC`/`AVATAR_CXX`
may select a compatible host Clang when using a community ARM64-host NDK.
`make build-echod BOARD=athena` stages the helper and notices automatically.
`make test-avatar` checks the scalar protocol and Go controller/assets.

## Contract (version 4)

`echolocal-avatar --protocol-version` prints `4`. The controller starts the
helper with a self-contained local VRM, `--control --protocol 4`, a bounded
`--seconds` and `--viewport X,Y,SIZE`. An optional `--animation` supplies VRMA.
No microphone device, socket or network is opened by this helper.

stdin lines: `S IDLE BLINK ANIMATION MOUTH_MODE FIXED_LEVEL MIC_LEVEL YAW_DEGREES PITCH_DEGREES ZOOM PAN_X PAN_Y`.
Flags are 0/1, mouth mode is 0 closed / 1 fixed / 2 synthetic / 3 external
local mic level; levels are finite 0..1. Yaw is finite -180..180 degrees and turns the complete model around its fitted center. Pitch is finite -85..85 degrees; zoom is finite 0.5..9.99 (1 = fitted framing). Both rotate around the fitted center; zoom is orthographic. Pan X/Y are finite -5..5 preview-size fractions, applied in screen space after zoom (positive right/down). A line is at most 120 bytes; parsing
has a per-frame input budget. Invalid state lines are ignored; oversized
lines terminate control. EOF terminates the process. SIGTERM closes the surface.

stdout emits `AVATAR_READY=4` after the first rendered frame, periodic
`AVATAR_STATS FPS FRAMES SECONDS MOUTH WORK_MS`, and bounded structured
`AVATAR_ERROR=model_invalid|motion_invalid|gpu_failed|protocol_error` on failure.
Existing performance diagnostics are informational and ignored by the controller.
The Go side requires readiness within ten seconds and escalates a stuck stop
to process termination within two seconds.

## Supported rendering features

VRM 0/1 embedded GLB, textured GPU skinning, morph-based mouth/blink,
selected node rotation/roll/aim constraints, and VRMA humanoid rotation/hips
translation with linear/step/cubic interpolation. Bind-pose camera fitting
uses the actual skinning transforms and authored morph defaults, including props.

Full MToon lighting, spring-bone simulation, expression overrides and automatic
phoneme/viseme alignment are not advertised capabilities. Unknown materials
use the existing textured shading path. Asset budgets and external-resource
rejection remain enforced. Other board/Android surface ABIs are not enabled.

`tests/` provides pinned public sample download and deterministic fixture
creation. Samples remain outside Git; their author terms and credit are in
`THIRD_PARTY.md`. Seed's `robo_arm` is an **authored accessory**, not duplicate
geometry: its embedded author thumbnail shows it behind the shoulder.
