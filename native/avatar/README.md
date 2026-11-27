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

`echolocal-avatar --protocol-version` prints `5`. The controller starts the
helper with a self-contained local VRM, `--control --protocol 5`, a bounded
`--seconds` and `--viewport X,Y,SIZE`. An optional `--animation` supplies VRMA.
No microphone device, socket or network is opened by this helper.

stdin lines: `S IDLE BLINK ANIMATION MOUTH_MODE FIXED_LEVEL MIC_LEVEL YAW_DEGREES PITCH_DEGREES ZOOM PAN_X PAN_Y`.
Flags are 0/1, mouth mode is 0 closed / 1 fixed / 2 synthetic / 3 external
local mic level; levels are finite 0..1. Yaw is finite -180..180 degrees and turns the complete model around its fitted center. Pitch is finite -85..85 degrees; zoom is finite 0.5..9.99 (1 = fitted framing). Both rotate around the fitted center; zoom is orthographic. Pan X/Y are finite -5..5 preview-size fractions, applied in screen space after zoom (positive right/down). A line is at most 120 bytes; parsing
has a per-frame input budget. Invalid state lines are ignored; oversized
lines terminate control. EOF terminates the process. SIGTERM closes the surface.

Optional Talk-only line: `V X Y WIDTH HEIGHT` exposes a rectangular application
overlay through the native surface, using top-left physical panel coordinates.
All integers are nonnegative, bounded to 4096 including the far edges; zero
width/height clears the aperture. The GUI reports the actual arranged volume
card bounds, so collapsed/expanded cards on either edge share the normal
widget and its expiry. The native helper never handles slider input or volume.

stdout emits `AVATAR_READY=5` after the first rendered frame, periodic
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

Protocol 5 adds Lab-only motion snapshots: A REV FD selects an immutable
descriptor in the parent process (0 removes the Lab clip); N 0|1 pauses/plays.
No caller pathname crosses stdin. The controller keeps the descriptor alive
until the matching AVATAR_MOTION REV STATUS TRACKS acknowledgment. Rejected
motions leave the surface/model alive; success resets only the animation clock.
Talk-layout renderers ignore motion selection. Both daemon and helper must be
installed together; older scalar protocols are rejected.

## Animated image snapshots

BG01 remains the unchanged single-frame straight-alpha RGBA contract. BG02 is
six little-endian words (magic, width, height, frame count, loop count, total
milliseconds), then per-frame delay + complete straight-alpha RGBA canvas.
Loop count 0 is infinite; finite counts hold the final frame. The Go decoder
composites WebP disposal/blend rules before staging; the helper only seeks raw
frames and updates the active texture. Invalid snapshots disable that layer,
not the scene. Both ends enforce dimensions, frame count, decoded size and
timing budgets. Hidden or inactive-mode layers do no reads/uploads; visibility
starts the loop at frame zero. Helper termination releases all descriptors,
frame buffers and textures. The image path has no animation worker or audio stream. Device Go binaries/tests must use
the nodynamic tag; the Makefile enforces it for production builds.

--capture-at SECONDS selects the diagnostic PPM capture time; omitted keeps
the existing capture behavior. AVATAR_LAYER_STATS UPLOADS LAST_FRAME VISIBLE
is informational test output and contains no asset path.


## Silent MP4 background

A descriptor-backed encoded MP4 may replace BG01/BG02 for backgrounds only.
`video.hpp` uses the existing FFmpeg 9.1 build (see `../legacy/build.sh`) with
custom local AVIO, external references disabled, one H.264 stream, square pixels,
no rotation, 8-bit YUV420, <=1280×800/30 fps/120 s/64 MiB. The helper opens no
audio decoder/output. `--probe-background FD_PATH` checks metadata and the first
picture without a GPU, with 5-second parent timeout and CPU/address-space limits.

Only a visible layer has a decoding worker. It streams/paces pictures by PTS,
loops by seeking/flushing at the video track duration and drops stale pending
pictures instead of queuing the full clip. Hiding joins the worker and releases
its codec context; showing recreates it from frame zero. All GPU calls stay on
the render thread. Midstream errors disable only that layer. Foregrounds reject
MP4 independently in both Go and native paths.

`AVATAR_VIDEO_STATS UPLOADS PUBLISHED ACTIVE` and `AVATAR_VIDEO_CAPTURE PTS` are
path-free diagnostics. `make -C native/avatar OUT=... .../video-test` builds the
headless Android decoder/loop/visibility regression executable; pass a short
valid MP4 fixture as its argument. Fixtures are not bundled user media.
