# VRM feasibility on Echo Show 8 (Athena)

Related: [issue #34](https://github.com/rainu/echolocal/issues/34).
Test date: 2026-11-20 UTC. This is a **negative-path feasibility study**, not an
avatar integration or proof that this GPU cannot render VRM in another runtime.

## Recommendation

**No-go for directly reusing the Pet WebGL renderer in the current, unmodified
EchoLocal runtime. Conditional for a later isolated browser/GPU investigation.**
There is a hardware GLES3 renderer, but neither the Android WebView execution
environment nor WebGL in the existing Cast browser is available.

Do not enable Android framework services, change the shared Cast launch flags,
or implement a native VRM engine as an implicit part of this study. A compatible
browser GPU/driver path, or a separately justified software renderer with measured
performance, must be established first. A 2D fallback is lower-risk; it is an
option to evaluate, not an adopted product direction.

## Verified sources and device

| Item | Observed |
| --- | --- |
| Device | Echo Show 8, Athena, model AEOAT; MT8183 |
| OS | Fire OS 7 family, Android 9 / API 28, build PS75751.7809N |
| Installed EchoLocal | 0.0.15-dev.integration.a50e5677, source a50e5677, build 2026-11-20T22:31:07Z |
| Deployed library pin | rainu/libcountertop e3fd8ed21859 (v0.0.0-20261120171450-e3fd8ed21859) |
| Source branch for probes | issue/34-vrm-feasibility, based on upstream/main f63fdfa2; no daemon code changes |
| Reference Pet | rainu/openclaw-pet d02524a871a8c51062a1511f1412bbad7c9c70bc |
| Physical display | 1280x800, approximately 59.95 Hz compositor refresh |
| Device RAM | MemTotal 1,973,744 KiB; not a renderer memory measurement |

No firmware, network, connection settings, production Talk code, installed
application or browser startup flags were changed. No microphone capture, test
speech playback, or additional Talk connection was used.

### EchoLocal is not a WebView application

At the deployed revision, `internal/feature/gui/gui.go` constructs
`screens.App` with native display/GPU drivers.
`internal/hardware/display/display.go` connects to the native surface helper.
The library's panel/surface and GPU layers submit native surfaces, GLSL effects,
textures and geometry; the helper uses EGL and OpenGL ES. They do not implement
JavaScript, the DOM, WebGL, glTF skinning, VRM expressions or a VRM loader.

Athena's `internal/host/sysimg/layout.go` gates `zygote`. Live
`init.svc.zygote=stopped`; Android `activity`, `package` and
`webviewupdate` services are absent. AmazonWebView.apk exists on disk, but a
working Android WebView provider/version cannot be queried in this operating
mode. No APK version or functioning WebView is inferred from its presence.

There **is** a separate bundled Chromium for Cast:
Chrome 150.0.7871.181, V8 15.0.245.21, engine-full-1. The engine launches a
Wayland client; it is not Android WebView. Its installed `chromium.sh` supplies
`--disable-gpu`. The source engine and native display files inspected in the
library checkout were byte-compared against the deployed pinned module.

## On-device evidence

Reproduction: [probe instructions](../../scripts/vrm-feasibility/README.md).
Machine-readable browser output: [CDP result](cdp-capabilities.json).
Native output: [EGL result](egl-capabilities.txt).

| Test | Result |
| --- | --- |
| Native EGL offscreen initialization | EGL 1.4 Android META-EGL, vendor Android |
| Native GPU | ARM Mali-G72 MP3 |
| Native GLES / GLSL | OpenGL ES 3.2 / GLSL ES 3.20; driver v1.r14p0-01rel0.01e5c7ea2a98fd18e15a3ddbd079d568 |
| Limits queried | Max texture 8192; max vertex uniform vectors 1024 |
| 64x64 pbuffer pixel readback | Pass: RGBA 64,128,191,255 |
| 120 clear + glFinish operations | 50.350 ms in recorded run; 20.921 ms in earlier run; not frame-rate evidence |
| Browser WebGL2 | Context creation failed: GL_VENDOR/GL_RENDERER Disabled |
| Browser WebGL1 and experimental-webgl | Same context creation failure |
| Chromium GPU feature status | webgl/opengl disabled_off; gpu_compositing disabled_software |
| Browser Canvas2D / AudioContext constructors | Available; AudioContext playback/processing not exercised |
| Blank page requestAnimationFrame | 182 callbacks / 3002.1 ms, max gap 16.8 ms; **not avatar rendering** |
| Browser target cleanup | Disposable target closed; existing receiver target preserved |
| Clock/service after probes | Idle view topmost, daemon running and screen ready |
| Talk | Inactive/ended; no conversation started or stopped by this study |

The GLES readback proves native GPU operation, **not WebGL acceleration**.
The browser test fails before VRM rendering could start. An Android GLES driver
cannot simply be assumed usable by this Linux/Wayland Chromium build. Inspection
found no EGL/GLES/SwiftShader/Vulkan renderer library in the installed engine
payload (license directories are not renderer binaries); no browser acceleration
switch experiment, new renderer installation or driver bridge was performed.

### Explicit measurement limits

- **Representative asset:** none loaded (0 models, 0 tested avatar triangles,
  bones, textures or draw calls). No user avatar/bundle was accessed or copied.
- **VRM 0.x/1.0, idle/skeletal animation, expressions, audio-driven mouth:**
  not demonstrated on this device. They are blocked at WebGL context creation,
  not measured as model-specific incompatibilities.
- **Load time, avatar FPS/p50/p95/p99 frame times, sustained runtime, incremental
  memory, CPU and GPU utilization:** unavailable for a VRM workload. Three seconds
  of blank-page callbacks and a tiny native readback cannot substitute for them.
- **Talk/audio contention and touch responsiveness under avatar load:** not
  tested, because no avatar could run. No claims about successful concurrency,
  acoustic quality, phoneme alignment or long-run thermal behavior.
- Normal service/idle state was checked through the local control interface;
  no human physical-touch/voice acceptance or screenshot-based visual acceptance
  is claimed.

## Reference compatibility and licensing

The pinned Pet tree contains renderer code, but **no tracked .vrm, .vrma or .oca
model/bundle**. It expects user-selected portable OCA ZIP bundles containing
`avatar/model.vrm`, a manifest, optional VRMA clips and poses. There is no
repository-owned representative model whose redistribution can be assumed.

| Component at pinned Pet revision | Version / license | Reuse assessment |
| --- | --- | --- |
| Pet source | MIT | Code may be adapted with copyright/license notice; Wails bindings and desktop APIs are not drop-in EchoLocal dependencies |
| Three.js | package-lock 0.186.0 / MIT | Requires a working browser WebGL2 path for this WebGLRenderer generation |
| @pixiv/three-vrm | package-lock 3.5.5 / MIT | VRM loading, humanoid rig and expression handling; no independent native renderer |
| @pixiv/three-vrm-animation | package-lock 3.5.5 / MIT | VRMA retargeting/playback, still needs the JS/render stack |
| Vue / Vuetify | package-lock 3.5.43 / 4.2.3, MIT | Optional UI framework dependencies; unnecessary for a minimal feasibility page |
| User avatars, textures, poses, motion clips | Asset-specific; not inspected | Repository/dependency MIT licenses do **not** grant model rights |

Pet registers `VRMLoaderPlugin` with `GLTFLoader`, calls `rotateVRM0`, uses
humanoid idle poses and `AnimationMixer`/VRMA clips, and addresses legacy and
current expression names (`Blink`/`blink`, `A`/`aa`). This indicates intended
legacy VRM0 and current VRM1 handling; neither is an Echo-device compatibility
result. Pet replaces materials with texture-preserving, double-sided unlit
`MeshBasicMaterial` for its desktop WebKit compatibility path. That workaround
does not create a missing WebGL context or preserve full MToon appearance.

Its speaking expression is a smoothed scalar `mouthOpen` input, not evidence of
phoneme/viseme lip sync. A future local audio test should use permitted bundled
PCM/WAV data and amplitude/RMS-driven `aa`; accurate phonemes require a separate
time-aligned viseme source.

Before copying any avatar, inspect VRM0 `meta` or VRM1 `VRMC_vrm.meta` plus the
creator's terms: authorship, modification, commercial use, redistribution,
attribution and any third-party texture/motion rights. If permission is unclear,
use a newly authored minimal test model or a separately verified permissive
sample, retaining its notices. This study distributes **no third-party model
or renderer code**; only original capability probes and their sanitized output.

Sources:
- [Pinned Pet renderer](https://github.com/rainu/openclaw-pet/blob/d02524a871a8c51062a1511f1412bbad7c9c70bc/frontend/src/components/PetVRMRenderer.vue)
- [Pinned Pet dependency lock](https://github.com/rainu/openclaw-pet/blob/d02524a871a8c51062a1511f1412bbad7c9c70bc/frontend/package-lock.json)
- [Pinned Pet bundle code](https://github.com/rainu/openclaw-pet/blob/d02524a871a8c51062a1511f1412bbad7c9c70bc/backend/avatar_bundle.go)
- [three-vrm v3.5.5](https://github.com/pixiv/three-vrm/tree/v3.5.5)

## Gates for a follow-up (not an implementation commitment)

1. Establish an isolated browser with **actual WebGL2** and record its backend,
   driver, feature flags and license inventory. Do not change the shared Cast
   process or enable Android framework services during an unrelated feature.
2. Test both a licensed minimal VRM0 and VRM1, then a licensed representative
   Pet bundle. Record file bytes, triangles, skinned meshes, bones, morph targets,
   draw calls, texture dimensions/resident memory and dependencies.
3. Start with <=20k triangles, <=60 bones, <=10 draw calls, <=1024px textures and
   about a 512px rendering viewport at DPR 1, no shadows/postprocessing. These
   are **proposed budgets**, not observed capacity.
4. Proposed gate: first frame <=3 s; stable 30 FPS, p95 frame <=40 ms/p99 <=66 ms,
   incremental process-tree PSS <=150 MiB, average renderer demand <=one CPU
   core, and no sustained thermal throttling. Measure at least 15 minutes,
   including 5 minutes with the **existing** Talk session and interrupted speech.
   Reject if audio underruns/drops worsen or UI response exceeds 100 ms. These
   thresholds require measurement and product review.
5. Hook the existing generation-owned `clawtalk.Manager` lifecycle and
   `talk.Controller` states connecting/listening/thinking/speaking/ended/error/
   muted. Use a nonblocking, bounded latest-value amplitude tap at the actual
   `deviceAudio` playout path, accounting for queue latency, mute, interruption
   and drain. Do not consume microphone frames, duplicate playback or add another
   Talk connection. Never send transcripts/credentials to the renderer.
6. Tie a single avatar view to the current generation. On cancel/error/end,
   discard stale async loads, unsubscribe, stop animation/audio analysis, release
   model textures/geometries/materials/contexts, revoke blob URLs, close the owned
   browser target and restore the previous/clock view once. Test rapid start/end,
   reconnect, renderer crash and load failure while preserving audio ownership.

### Simpler 2D alternative

The existing native UI/GPU layer can animate a small authored sprite atlas or
simple shapes for blink/breathing/head movement, plus a small set of RMS-driven
mouth frames and listening/thinking indicators. Browser Canvas2D is also
available, but keeping native rendering avoids a second web UI/lifecycle.
Neither path needs glTF skinning, MToon or the Three/VRM stack. Asset rights still
apply. This is an architectural assessment only: no 2D production UI, quality,
resource-budget or lip-sync success is asserted.

## Reversibility and scope

Only a short-lived blank Chromium target, a loopback ADB forward and one
temporary offscreen probe executable were created. Target and forward were
removed, the executable was hash-checked against the retained build and removed.
The existing receiver page, installed daemon, settings, normal idle view and
inactive Talk state were left in place. There was no daemon update, so an
application rollback/install/reboot was neither required nor performed.

No production VRM integration should be merged under this study's results.
Acceptance is of the tested capability probes and negative-path report.

## Verification and retained artifacts

- Go probe: compiled and vetted with Go 1.27; on-device Chromium checks ran
  through the local ADB/CDP forward; non-loopback endpoint rejection checked.
- Native probe: API-28 ARMv7 NDK build with -Wall -Wextra -Werror; executed on
  Athena twice; host/device executable SHA-256 matched before cleanup.
- Tested native executable SHA-256:
  `b9dae93de89ef4802723a7cc9d4b49c3db4bfbdddd6a4891ee148fc986fa3330`.
- Sanitized outputs are committed alongside this report. Exact probe executables
  and private coordination records are retained in the development artifact
  store; they contain no copied user model or captured room audio.
- No EchoLocal daemon binary was rebuilt or installed: this change only adds
  standalone probe sources, instructions and evidence.

The review branch `issue/34-vrm-feasibility-review` carries the same six study
files on top of the existing integration branch. This avoids including unrelated
new upstream application changes in the study PR. The upstream-based feature
branch is retained for traceability; neither branch changes daemon code.
