# Native avatars and Avatar Lab

Native avatar rendering is a regular Athena feature. The feasibility phase is
complete. The renderer and application are normal project code, built, shipped
and checked through the standard build and installation paths.

## Components

- `internal/avatar`: owned renderer lifecycle, bounded scalar IPC, readiness and
  error handling, microphone-level adapter, validated local asset selection.
- `native/avatar`: separate AGPL-3.0 native GLES3 executable; its license and
  third-party notices remain separate from the Go application.
- `internal/feature/gui/avatar.go`: Avatar Lab under **Settings → Avatar Lab**
  (**Einstellungen → Avatar-Labor**) and **Apps → Avatar Lab**.
- `internal/parts/avatar.go`: Athena payload installation and license delivery.
- `echolocal avatar import`: local model/motion provisioning through the host CLI.

No debug artifact directory, hardcoded Seed filename, manually copied renderer,
background shell launcher or main-service pause is required by the feature.
Other boards are not enabled until their native surface ABI is implemented.

## Build and install

`make build-echod BOARD=athena NDK=/path/to/ndk` builds and embeds the avatar
helper automatically. `make build-avatar` builds it independently.
`make test-avatar` runs the native scalar-protocol and Go controller/asset tests.
The existing installer and `parts.Ensure` install/update it at
`/system/app/echod/echolocal-avatar`, together with `.LICENSE` and
`.THIRD_PARTY.md`. No new service, port, firmware, or framework component is added.

The host CLI is built by `make build-echolocal`. Import assets with:

```sh
echolocal avatar import --serial DEVICE --model avatar.vrm --animation motion.vrma
```

`--animation` is optional. Import never downloads remote resources or copies
third-party assets into Git. Use models/motions you are entitled to use.
Embedded model name and authors are displayed in the app; Seed is credited to
VirtualCast, Inc. Model data remains local to the selected device.

The selection lives in `/data/misc/echolocal/avatar/current.json`. Files are named
by SHA-256; the manifest contains hashes, not arbitrary paths. Import verifies
self-contained GLB/VRM/VRMA, rejects external resource URIs, checks transfer
hashes and replaces the selection atomically only after all files are present.
`previous.json` retains the previous selection. Restore it atomically to
`current.json` for an asset-selection rollback; content-addressed files remain
available. A running renderer keeps its loaded model until Stop/Start.

## Operation

The normal EchoLocal capture/service stays running. Avatar Lab provides live
idle, blink and VRMA controls, 15-degree left/right rotation with front-view reset, plus closed/manual/synthetic/microphone mouth
sources. The local shared microphone stream is calibrated for one quiet second;
hardware mute is respected, and stale levels close the mouth after 200 ms.
Neither raw audio nor recordings leave the capture process through this feature.
Only bounded scalar state is sent to the helper; no shell or network listener
is involved. The helper does not open ALSA independently.

Each lab run is bounded to five minutes. Stop, Back, hiding the app, parent exit
or stdin EOF ends its owned renderer. Readiness must arrive within ten seconds;
a stuck stop is escalated after two seconds. Layout uses the current physical
preview viewport, not a fixed 1280×800 assumption. Model, motion, graphics and
protocol errors have bounded user-facing states.

Metrics report FPS, rendered frames, elapsed time, mouth level, RMS, microphone
blocks and >100-ms delivery gaps. Delivery gaps are **not ALSA XRUN counters**.
This is an engineering/test application built on the production renderer, not
a permanently enabled assistant avatar. Talk/TTS phoneme alignment is not
implicitly enabled by opening it.

## Rendering contract

Supported: VRM 0/1 embedded GLB, textured GPU skinning, expression morphs for
mouth/blink, selected node constraints and VRMA rotation/hips translation with
linear/step/cubic interpolation. Input and GPU-memory budgets are enforced;
model selection verifies file integrity before starting. Camera fitting uses
skinned bind-pose geometry including morph defaults and authored accessories.
Full MToon, spring-bone simulation, expression overrides and phoneme/viseme
alignment are outside the advertised feature set, not blockers to feasibility.

**Correction to the earlier test report:** Seed's additional mechanical arm is
an authored `robo_arm` mesh. The embedded author thumbnail shows the same
accessory behind the shoulder. It must not be removed as duplicate geometry.
The earlier interpretation as a renderer geometry defect was not justified.
Camera clipping is addressed by skinning-aware camera fitting.

The scalar control contract is version 4; daemon and bundled helper are updated together. Rotation is live, wraps at ±180 degrees and uses a fixed pivot. Camera framing includes the horizontal rotation radius to keep authored props in view.

In Avatar Lab only, one-finger dragging inside the visible running preview
rotates horizontally and tilts vertically (180 degrees per preview width;
pitch limited to ±85°). Diagonal drags combine both. Two fingers starting
inside the preview choose either pinch zoom (50–300%) or screen-plane pan.
Spreading enlarges the avatar; parallel dragging moves it without rotation or
zoom. After a 12-pixel classification threshold, gesture ownership stays fixed
until lift, preventing spacing jitter from changing pan into zoom. Ambiguous
movement waits for more travel (32 pixels). Pan is bounded to ±one preview width
per axis and is independent of zoom and model orientation.
The second finger cancels one-finger rotation. Lifting either finger ends
pinch ownership until all fingers lift; outside-origin or three-finger gestures
cannot zoom or pan. Leaving/stopping the lab or resetting the view invalidates gestures.
Arrow buttons remain; **Front** resets yaw, pitch, zoom and position together.

Pinch uses a view-gated raw-contact observer chained on the UI window, preserving
other handlers. One-finger input retains widget-local press/absolute capture
conversion, slop, final release coordinates and cancellation. No global hardware
listener is added. Daemon and bundled helper use scalar protocol 4 and must be
updated together. Pitch rotates the whole fitted model around its center;
orthographic zoom scales screen-plane coordinates without modifying depth.

IPC versioning is independent of the GLSL language version: both shaders remain
`#version 300 es` for the Athena GLES3 GPU. Native host protocol tests do not
compile GPU shaders; verify a rendered frame on the target after shader changes.
