# Native VRM follow-up: measured results

Related: [issue #34](https://github.com/rainu/echolocal/issues/34).
Test date: 2026-11-21 UTC. Owner-authorized extension of the original study:
try a native engine rather than treating unavailable browser WebGL as the
device's rendering limit.

## Verdict

**Native VRM is feasible on this hardware. Conditional go for a production
integration project, not acceptance of a completed Talk/avatar product.**
The previous browser-path no-go in [PR #35](https://github.com/rainu/echolocal/pull/35)
remains correct for the existing Chromium flags/runtime. It does not apply to
the independent native Mali/GLES executable demonstrated here.

The prototype loads real VRM 0/1 files, renders their textured/skinned meshes,
animates a humanoid from an external VRMA clip, applies selected face morphs and
drives mouth opening from a local audio envelope. Both GPU readback and a
composed screen capture were visually inspected. This is not a blank context
or synthetic triangle benchmark.

## Target and exact binary

- Echo Show 8, Athena, MT8183; Fire OS 7 family / Android 9 / API 28.
- ARM Mali-G72 MP3, OpenGL ES 3.2, vendor driver
  `v1.r14p0-01rel0.01e5c7ea2a98fd18e15a3ddbd079d568`.
- Physical display: 1280x800; viewer: 640x640 at `(640,80)`, 30 FPS cap.
- Installed EchoLocal remains `a50e5677`; no daemon rebuild/install/restart,
  Android framework startup, Cast flag change or new Talk connection.
- Native binary SHA256:
  `6b893cc25d2a3912db4958bbf51f085089f0c3b781f7dbcd9d350fe4b20ca391`.
  Host and device copies match. A clean invocation of the committed build
  script produced exactly the same binary hash.
- All source, dependency/license inventory, authored fixture generator and
  reproduction instructions are in this directory. Test models/binaries,
  raw display captures and private coordination data are not committed.

## Assets and short device checks

Counts describe this viewer's actual uploads/draws, including per-primitive
vertex duplication. Texture numbers are **base RGBA bytes**, excluding
mipmaps, temporary decoder allocations and driver memory.

| Model | Version | File bytes | Uploaded vertices | Triangles | Draw parts | Base texture bytes |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| VRM1 Constraint Twist Sample, pixiv Inc. | VRM1 | 10,776,032 | 24,951 | 36,470 | 13 | 72,613,888 |
| Seed-san, VirtualCast, Inc. | VRM1 | 10,917,800 | 34,059 | 45,058 | 21 | 43,253,760 |
| Alicia Solid, (c) DWANGO Co., Ltd. | VRM0 | 7,878,712 | 52,849 | 31,798 | 20 | 39,845,888 |

Asset hashes, source revisions and exact usage terms are in the downloader and
[license inventory](THIRD_PARTY.md). These samples are not user avatars and are
not a claim of compatibility with a particular Pet OCA bundle.

Observed short-run results on the final executable:

| Check | Evidence |
| --- | --- |
| Native transform math | On-device `--self-test` passes inverse/multiplication, quaternion interpolation and aim/opposite-direction rotations |
| VRM1 + LINEAR VRMA | 90 frames / 3.000 s; first completed frame 691.351 ms; frame-start interval p95 33.376 ms / p99 33.432 ms; average CPU 0.108 cores |
| VRM1 + STEP VRMA | 90 frames / 3.000 s; first frame 688.510 ms; interval p95 33.419 / p99 33.635 ms; CPU 0.104 cores |
| VRM1 + CUBICSPLINE VRMA | 90 frames / 3.000 s; first frame 681.887 ms; interval p95 33.458 / p99 33.870 ms; CPU 0.103 cores |
| VRM0 + procedural idle | 90 frames / 3.000 s; first frame 651.652 ms; interval p95 33.386 / p99 33.598 ms; CPU 0.119 cores |
| Native display layer | Composed screenshot shows textured avatar beside the still-running clock; no browser or UI replacement |
| Constraint behavior | VRM1 shoulder/sleeve deformation visually corrected by aim/roll/rotation solver; VRM0 arm pose orientation corrected and readback inspected |
| Mouth morph | Static neutral vs aa=1 Seed-san images differ at 155 pixels, bounded to mouth rectangle `(214,141)..(232,157)` in the 640px frame |
| Local audio | Authored PCM16 tone with known silent/active intervals accepted via `--wav`; render readback inspected; no playback, microphone or provider connection |

The short VRM0 screenshot run had a maximum frame-start interval of 72.757 ms,
including a PPM readback/write. Thus an average 30 FPS result alone must not be
described as perfect presentation or absence of stalls.

## Sustained display run

Completed with **exit 0**: Seed-san on the actual display, external two-second
LINEAR VRMA loop, blinking and explicit continuous synthetic mouth morphs.
One composed screenshot was taken during the run.

| Metric | Observed |
| --- | ---: |
| Completed frames / duration | 27,000 / 900.000 s |
| Average rendered-loop FPS (30 cap) | 30.000 |
| First completed GPU frame | 709.812 ms |
| Frame-start interval p95 / p99 / maximum | 33.539 / 34.703 / 46.849 ms |
| Update/draw/glFinish work p50 / p95 / p99 | 6.718 / 8.610 / 10.440 ms |
| Average process CPU demand | 0.138 CPU cores |
| Peak process RSS, including startup | 134,764 KiB |
| Post-startup PSS samples | 57,050–57,478 KiB |
| Post-startup RSS samples | 62,740–63,168 KiB |
| Resource samples | 30, at approximately 30-second spacing |
| All sampled thermal-zone readings | 25.129–40.300 °C |

Post-startup ranges discard the first sample taken before loading completed.
PSS changed by less than 0.5 MiB over the sampled steady interval; this is not
an unlimited-duration leak proof. Thermal zones are not attributed to specific
silicon here; these readings do not prove the absence of throttling. Driver/GPU
resident memory and GPU utilization remain unavailable.

### Final-device verification

All **13 final checks passed** on the same executable hash: native math,
VRM0+VRMA, neutral/open/WAV mouth frames, malformed GLB, rejected external buffer
and image URIs, truncated WAV, NaN mouth value, invalid duration, graceful SIGTERM
and forced SIGKILL. External-resource fixtures were rejected before loading the
resource. Expected error exits are passes for those rejection tests.

On the final binary the open-mouth comparison again changes **155 pixels**;
the PCM-envelope comparison changes **133 pixels**, both confined to the same
mouth rectangle `(214,141)..(232,157)`. This demonstrates the reference-audio
mouth path without claiming phonemes or real playback alignment. Final VRM0
VRMA loading also passed; comprehensive retargeting accuracy remains untested.

SIGTERM exits 0; SIGKILL yields the expected 137. In both cases the owned display
layer is removed without restarting EchoLocal. After all checks, the normal
clock was captured and visually inspected again.

## Interpretation and unverified gates

- The standalone engine path works. It does **not** implement complete VRM:
  no spring-bone/collider simulation, full MToon/outline pipeline, complete
  expression overrides, lookAt, OCA bundle import or general motion-library
  conformance. See the feature matrix in [README](README.md).
- Basic mouth opening is an amplitude/RMS-driven `aa`/`a` morph, not phoneme
  lip synchronization. The WAV is a reference tone and is not played. This
  establishes an audio-envelope-to-morph path, not acoustic alignment.
- The 15-minute workload uses a continuous synthetic mouth scalar, not live
  Talk audio. The existing Talk manager was inactive/ended and was not started
  or stopped. No five-minute real Talk concurrency, audio-underrun regression,
  room speech, mute/interruption or end-to-end speaker synchronization result
  is claimed.
- Process CPU is measured; GPU utilization is not available from the inspected
  usual sysfs counters. PSS/RSS exclude unknown driver/device memory and do not
  establish total incremental system memory. Thermal samples and frequencies
  are observational, not a complete no-throttling proof.
- The clock/UI and Node read operations remain available while the separate
  layer renders. Physical touch/voice and sub-100-ms UI latency are not measured.
- User-selected models and arbitrary commercial animation packs have not been
  tested. Retargeting is demonstrated with newly authored head/hips tracks;
  neither full VRMA conformance nor exact appearance parity is established.
- The SurfaceComposer bridge is vendor/API-28/ARMv7-specific and derives from
  AGPL libcountertop. The viewer is separately AGPL-3.0-only; copying it under
  the parent repository's MIT label would be incorrect.

## Next integration gate

Use the existing manager generation/context and actual voice-lane submission
point described in [README](README.md#future-talk-integration-outline-not-implemented-here).
Before enabling this in the product, measure a representative permitted user
avatar with real Talk active, drain/interrupt/mute tests and renderer-failure
recovery. Keep the renderer optional and supervised; audio ownership, current
connection settings and the normal clock must survive any renderer failure.

## Cleanup

Completed: all **37** experiment-owned temporary device files were pulled to
the private artifact store, SHA256-compared individually, and only those exact
files were removed. The now-empty fixture and experiment directories were
removed. No broad device cleanup or deletion of pre-existing rollbacks.

- SurfaceFlinger lists **zero** layers named for this experiment.
- Original clock/idle view is topmost and visibly restored.
- EchoLocal remains running, screen ready; installed binary SHA256 remains
  `8ef33a4d4b098cce7221bdcd6d103a53f5f215df5586d19c970171cb5e0320be`.
- The normal screen and Talk-state Node calls succeed; Talk is inactive/ended,
  as before. No conversation was started or stopped.
- No settings, grants, framework services, browser flags, daemon installation,
  network, firmware or Gateway configuration changed.

The original browser study remains separate. This PR changes only the isolated
experiment and its documentation; it must not be described as a deployed Talk
avatar feature. Merge and issue closure remain subject to owner acceptance.
