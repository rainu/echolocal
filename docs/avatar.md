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
inside the preview choose either pinch zoom (50–999%) or screen-plane pan.
Spreading enlarges the avatar; parallel dragging moves it without rotation or
zoom. After a 12-pixel classification threshold, gesture ownership stays fixed
until lift, preventing spacing jitter from changing pan into zoom. Ambiguous
movement waits for more travel (32 pixels). Pan is bounded to ±five preview widths
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

## Custom model selection (Athena)

**Avatar Lab → Model file** selects one persistent `avatar/path` setting. The
same field is under **Display → Avatar Lab** in the owner HTTPS web UI and in
settings discovery/API, ESPHome controls, and the permission-gated Node settings
commands. This is an absolute path **on the Echo**, not on the browser computer.
Only the existing settings grant is used; no new file-reading Node command is
added. The setting is preserved across EchoLocal service restarts.

Upload a model through the existing **Files** filesystem browser into a writable
device directory, or use the existing authenticated WebDAV interface. Enter the
full destination path, including the `.vrm` filename, in the model field. Managed
Files assets can also be selected using their existing device-side path; there
is no additional uploader, public model endpoint, or network model downloader.
The existing CLI asset import remains available for the default model/motion.

Example local configuration: `echod ctl settings avatar.path /data/local/tmp/avatar.vrm`.
The owner configuration endpoint is `PUT /api/v1/settings/avatar/path` with
`{"value":"/data/local/tmp/avatar.vrm"}`; GET reads the same field. Empty `value`
or **Use default avatar** restores the installed default catalog selection and
its animation. No setting change preserves the previous behavior and catalog;
no bundled model is overwritten. Stop/Start reloads the selected model. Changes
from any settings surface apply on the next Start, not midway through a run;
the model name/credit identify the actually loaded model. The native file editor
stops the preview before opening. Reset is explicit; failures never silently
fall back. Correct the path/file, choose another model, or reset, then Start.

Custom models must be self-contained GLB: VRM 0 (`VRM.specVersion=0.0`) or VRM 1
(`VRMC_vrm.specVersion=1.0`), strictly below 64 MiB, with no buffer/image URI references (including data URIs). Paths are limited to 255 UTF-8 bytes for existing text-control parity;
relative/traversal/control-character paths and non-.vrm names are rejected.
File access shares the owner browser's canonical-root and pinned regular-file
opener; production root is the real filesystem `/`, with unchanged OS permissions.
Symlinks follow that existing policy, cannot escape a restricted test root, and
special files are rejected before any device open callback is invoked.

Before parsing, the file is copied to one 0600 snapshot that is immediately
unlinked and held open only for the renderer lifetime. Neither model replacement
nor restart accumulates model copies. Stop, renderer failure, startup failure,
and parent exit release the snapshot; a crash also releases its anonymous inode.
The renderer sees only that snapshot, never the configurable source path.
The default VRMA is reused when valid; custom selection does not import motion.
Compatible humanoid names/rigs are necessary for retargeting. Existing rendering
limitations above still apply: full MToon, spring bones and expression overrides
are not advertised, and compatibility with unrelated avatar applications is not
assumed. Scene limits include fewer than 4096 nodes, 256 meshes/materials,
500000 vertices, 128 MiB decoded texture memory, and existing morph budgets.
Model/version/resource errors are recoverable lab states, not daemon failures.

## Standalone Talk presentation and playback mouth envelope

On Athena, the standalone OpenClaw Talk activation (including assistant wake-word
routing) opens an avatar beside connection/listening/speaking/error feedback and
an explicit Stop control. It uses the **same Talk activation, selected agent,
session and audio lane**. Talk started inside a selected chat retains its normal
chat view; Home Assistant routing is unchanged.

The left side retains the existing recognized user and spoken assistant
transcripts, including provisional text updates and final replacements. It uses
the same bounded, volatile device caption cache and the same speaker-colored
cards as regular Talk. Contributions remain newest-first. Long contributions
wrap and scroll within the transcript area; status and the **History** / **End
conversation** buttons remain fixed outside it. History temporarily covers the
avatar without starting another Talk connection; returning resumes rendering.
Rendering failures keep the transcript and both actions available. No transcript
is added to diagnostic logs or persisted by this view.

Avatar Lab's **Save presentation** persists the preview framing and the Idle
movement, Blink and VRMA animation switches. **Front** resets and saves all five
transforms, without resetting the selected model or motion choices. A
**Presentation** editor with numeric fields and motion toggles is available on the device; the owner web settings and
existing settings API expose the same fields in domain `avatar`:

| Field | Unit and accepted range | Default and interpretation |
| --- | --- | --- |
| mouth_intensity | integer percent, 0–100 | 50: restrained speech; 0 = closed, 100 = original strength |
| zoom | integer percent, 50–999 | 100: fitted model; larger enlarges |
| yaw | integer degrees, −180–180 | 0: front; horizontal model-center rotation |
| pitch | integer degrees, −85–85 | 0: upright; vertical model-center rotation |
| x | integer percent, −500–500 | 0: centered; positive moves right |
| y | integer percent, −500–500 | 0: centered; positive moves down |
| idle | on/off | on: idle movement enabled |
| blink | on/off | on: automatic blinking enabled |
| animation | on/off | on: installed VRMA motion playing |

All three motion switches are saved with **Save presentation** and restored in
both Lab and standalone Talk, including after restart. Native settings, web and
API/Node edits save immediately. **Front** only resets framing, preserving the
motion choices. Legacy storage without motion flags keeps all three enabled;
explicitly saved off states remain off. Storage uses inverse disabled flags to
distinguish these states without changing legacy defaults. Mouth mode/microphone
preview is not saved or inherited by Talk: its mouth still follows agent playback.

Position is relative to the **rendering viewport width**, including vertical
translation, independent of model orientation and zoom. Lab keeps its square
preview; Talk uses the entire right half from the top to the bottom edge, with
no title/footer reservation. The model name is in the left status header.
Aspect-correct projection preserves width-relative model scale and translation
without stretching the model; the rectangular Talk surface shows additional
vertical space. Legacy square renderer viewports remain supported.
Gestures preview continuously; Save rounds to integer degrees/percent. Native numeric edits and API/web writes persist immediately.
The settings grant also covers these fields for existing Node configuration
commands. Out-of-range, fractional and nonfinite writes are rejected. Legacy
storage without zoom uses 100%; malformed manually edited storage is bounded
when read for rendering. Restart retains settings. Changes are loaded on the next
lab entry or Talk activation, not halfway through a running conversation.

The mouth uses a local amplitude envelope of **StreamVoice periods successfully
submitted to ALSA**, after stream mute and output gain. Scalar levels are scheduled
against ALSA playback delay, after the existing jitter buffer and speaker queue;
received text/packets do not drive animation. The Talk renderer never subscribes
to microphone audio. Feedback sounds and media do not animate it. Silence,
stale playback, stream drain/interruption and conversation teardown reset the
envelope; a generation counter rejects output periods racing a drain. The bounded
queue holds no PCM or recordings. Hardware/DSP and frame scheduling introduce
approximately one audio/render period of timing uncertainty. This is amplitude
animation using the model's existing compatible mouth expression, **not phoneme
or viseme alignment**; models lacking that expression cannot show it.

Renderer loading happens asynchronously and does not gate audio readiness.
Model/graphics/renderer failure displays a localized recovery hint while Talk
remains usable. Stop Talk, correct the model/presentation in Avatar Lab, then
activate again. No silent model fallback or second Talk connection is created.
Conversation completion/failure removes its temporary page, stops the owned
renderer and releases the model snapshot, revealing the previous display.
Existing acknowledgment/progress/readiness/error/end sounds and end phrase
behavior use their unchanged audio paths.

### Extended close-up zoom contract

The shared range is **50–999%**, inclusive, with unchanged 100% default.
Lab gestures, native numeric settings, owner web/API and Node settings all use
this range. The native control accepts scale 0.5–9.99; no consumer may clamp to
3. Save rounds preview scale to integer percent. A saved face close-up is loaded
unchanged on the next Lab entry and standalone Talk activation.

Future fullscreen/background consumers (#47) must load
`avatarlab.Options(config.AvatarSection.Read())` and use the same native
zoom/pan/projection path. Do not add editor-only or 300% limits. Width-relative
framing remains consistent; a wider viewport naturally scales the model with
its width and a taller viewport reveals additional vertical area.

At high zoom, model-centered scaling can move the head beyond the former ±100%
translation reach. Position range is therefore extended additively to ±500%;
units, signs, screen-space ordering and previously valid saved values are
unchanged. The native scalar bounds are ±5. This lets a full-body model's face
remain reachable at 999% instead of merely enlarging its torso.

### Saved background and fullscreen composition

The shared native/web/owner API field is avatar.background. Use Files or
WebDAV to upload a PNG or JPEG, then enter its full **device path**, not a path
on the browser computer. Empty restores the existing default background.
Changing or clearing the image never resets model selection, zoom, rotation,
position or motion toggles. Changes apply on the next Lab Start, fullscreen
preview or standalone conversation activation, and survive restart.

Images use **centered cover** within the avatar panel: preserve aspect ratio,
fill the panel and crop surplus edges symmetrically. Transparent PNG pixels
show the normal default clear color. The avatar is rendered in front. The
fullscreen preview and conversation share both panel geometry and the native
background/projection path; neither adds a separate camera or transform.

The Lab fullscreen preview uses the current (including unsaved) placement,
with exactly the standalone conversation's right-half, full-height viewport.
Its left half contains an explicit touch exit and visual-only notice rather
than conversation controls. It never starts Talk, opens a microphone, sends a
message or places a call. Lab mouth-source options cannot enable preview
capture. Exit restores the previous lab options and running/stopped state.
Use Save presentation to persist placement for the next actual conversation.
The session-specific chat Live Talk view is unchanged.

Limits: 8 MiB encoded, 4096 pixels per side and 4 megapixels decoded. Only PNG
and JPEG are accepted. File access uses the existing pinned-regular-file policy;
no permissions or roots are expanded. A bounded immutable encoded snapshot is
header-checked before full decode, then converted to a texture of at most 2048
pixels per side. One temporary RGBA snapshot is unlinked immediately and its
descriptor released on stop; no permanent imported copies accumulate. Invalid
selection is rejected without losing the previous setting. Files subsequently
removed, made unreadable or corrupted fall back to the default background with
a localized recovery message, without blocking Talk.

### Speech mouth intensity

`avatar.mouth_intensity` is the same integer setting in on-device Presentation,
owner web settings, settings API and existing Node settings commands. Invalid,
fractional and nonfinite writes fail without changing storage. Old installations
without the field use **50%**. Explicit **0%** survives save/restart and keeps the
speech mouth at rest; **100%** restores the old strength. Reset presentation or
the Lab mouth reset returns to 50%. Manual hand-edited storage is bounded.

The Lab has live ±10% controls, a 50% reset and **Speech preview (silent)**. The
preview replays an amplitude envelope derived from an offline synthesized speech
recording, including pauses, without playing audio, capturing the microphone or
connecting to an agent. Start/Stop and the other mouth-source buttons remain
available. Preview/source choice is temporary; **Save presentation** persists the
intensity for the next standalone wake-word Talk activation. Fullscreen remains
a silent, closed-mouth composition preview, never a Talk session.

Intensity multiplies the final bounded playback expression, after the existing
speaker level calculation and before renderer smoothing. It never changes PCM,
speaker volume, microphone gain, VAD or audio sent to an agent. The range cannot
amplify above the old maximum or introduce extra clipping; rest remains zero.
Existing interruption, stale-period and teardown resets are unchanged. Both VRM
0/1 mouth bindings use their existing authored weights, clamped by the renderer;
models without a compatible A/aa expression stay unchanged rather than failing.
Different authored mouth shapes can still warrant a different saved percentage.

### Avatar Lab editing workspace

The left side is divided into four workflow sections; the right side keeps
the same live avatar viewport and gestures:

- **Assets**: model file, background and saved presentation settings.
- **Framing**: current rotation, zoom/position readout and front-view reset.
- **Expression**: speech mouth intensity, silent speech preview and motion toggles.
- **Advanced**: mouth-source tests, fixed mouth value and renderer/microphone diagnostics.

Only the selected section scrolls. Section navigation, Back, Start/Stop,
Fullscreen and Save stay visible outside the scrolling region. The selected
section has a filled/marked button and a full heading. Long localized button
labels wrap and grow rather than being clipped. Each section has independent
scroll state. Credits and the existing one-/two-finger gestures are retained.

Save is the existing Save presentation operation (including mouth intensity);
it does not persist the temporary mouth source. Front retains its existing
reset-and-save behavior. Asset selection, presentation settings and fullscreen
preview still use the existing pages/lifecycle. No rendering, configuration or
microphone behavior is changed by selecting a section.


## Wake-word Talk presentation modes

Standalone wake-word Talk on Athena has **Split-screen** and **Fullscreen**
compositions. Session-specific Live Talk in Chat is unchanged. The split-screen
keeps its transcript, status, history and end controls; its expand icon switches
presentation. Fullscreen contains only the avatar/background and two circular
bottom controls: split-screen and end conversation. Their accessible names are
localized. Controls remain available if the model, image or graphics fail.

Both modes use the same Talk connection, playback lane and microphone state.
One full-panel native surface is retained; mode changes select a rendering region,
background and saved transform without restarting the renderer or its animation
clock. Both validated background snapshots are loaded once for the conversation,
then reused across switches and released with the renderer. Invalid/missing
backgrounds use the normal dark background. An image change made during Talk
applies to the next conversation.

### Settings contract

All fields are in the existing `avatar` domain, shared by device settings,
owner-authenticated web settings, configuration API and permission-gated Node
settings. API paths remain `/api/v1/settings/avatar/FIELD`.

| Field | Values / units | Default / reset value |
| --- | --- | --- |
| `talk_mode` | `split_screen`, `fullscreen` | `split_screen` |
| `fullscreen_background` | Validated absolute device image path | Empty = dark background |
| `fullscreen_zoom` | Integer 50–999 percent | 100 (fitted framing) |
| `fullscreen_yaw` | Integer −180–180 degrees | 0 |
| `fullscreen_pitch` | Integer −85–85 degrees | 0 |
| `fullscreen_x`, `fullscreen_y` | Integer −500–500 percent of viewport width | 0 |

Positive X moves right, positive Y moves down. Image fitting uses centered cover
cropping against the actual mode's viewport. Fullscreen fills the complete panel;
split-screen uses the right half at full panel height. Old/missing/invalid stored
mode values resolve to split-screen. Missing/zero stored zoom resolves to 100.
API writes reject invalid choices, out-of-range numbers and invalid image paths.

In Avatar Lab the heading identifies the composition being edited. **Assets →
Edit other mode** selects its independent saved framing; save edits before
changing mode. Background and presentation settings route to that composition.
**Default Talk view** selects the next conversation's mode, independently of the
current preview or current Talk mode. **Reset fullscreen presentation** resets
only its background and framing to the table above, not the default mode,
split-screen composition, model or shared expression settings. The same reset
values can be written through web/API fields individually.

Save and Front/reset write framing only to the selected composition. Model,
idle/blink/animation and mouth intensity remain shared. Fullscreen image and
framing never overwrite the split-screen image and framing. The pinned preview
button opens the selected composition with the real panel geometry, cover crop,
transform and bottom controls. Its mode button switches the preview and its end
button returns to the editor. Preview never starts Talk or opens a microphone;
its renderer uses a closed mouth and a zero scalar audio source.

The optional scalar protocol-4 extension `T 0|1` selects split/full presentation
only for helpers launched with `--talk-layout`. The surface dimensions remain
fixed; neither the frame clock nor the model/motion resources are recreated.
The GUI and native helper share 72-pixel controls, a 24-pixel gap and a 20-pixel
bottom inset. Transparent circular apertures in the upper native layer reveal
normal accessible, focusable GUI controls below it. Input stays in the GUI.
