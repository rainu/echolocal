# OpenClaw live screen sharing and control (issue #15)

## Contract and dependencies

OpenClaw release `2026.11.6` (`06afb10`) supports cancellation-owned
`desktop.stream` invocations, a dedicated binary WebSocket carrier, and the
built-in RFB/noVNC observer. H.264 and MP4 recording output is not this protocol.
Reference upstream inspected at
[8b17822bec4f36582bf5e15743087b32fd3460b1](https://github.com/openclaw/openclaw/tree/8b17822bec4f36582bf5e15743087b32fd3460b1).

The existing Android/tablet Node identity is retained. Android has no default
`desktop.stream` admission: the operator must explicitly allow that command in
Gateway policy and approve any same-identity capability upgrade. There is no
platform spoofing, operator-identity reuse, additional listener, or mandatory
upstream protocol change established by this work.

The implementation branch includes the accepted integration dependencies (Node,
permissions and capture features) through an integration merge. Its PR diff
against the integration target contains only issue #15 work.

## Local permission and user controls

A dedicated `desktop` permission defaults **off**, both for fresh installations
and when upgrading old permission files. The old `screen` grant continues to
govern screenshots and bounded recordings only. Local administrators can enable
live sharing under OpenClaw -> Node -> Permissions. Enabling all permissions is
also an explicit opt-in. Changes reconnect the same Node identity when its declared capabilities need to
expand. Input-only reductions are applied live, cancel input work and withdraw
input tool metadata without disconnecting an ongoing screen stream. Re-enabling
previously declared input can also be live; enabling input absent from the
current handshake requires a reconnect/capability approval as usual.

Start from the OpenClaw desktop viewer. Close/release that viewer to stop.
The device permission page also has **Stop stream**; the local administrative
CLI exposes `node-desktop` status and `node-desktop stop`. Neither is exposed
through remote input or as a Node plugin tool. Revoking the local desktop
permission cancels capture and withdraws the command declaration.

The existing **Remote input** (`input`) permission also governs desktop-viewer
input; there is no additional input permission. View-only Gateway observers
filter input before delivery. In control mode, the device independently checks
the live input grant and existing sensitive-page restrictions for every event.
Sharing alone never authorizes input.

The left mouse button maps to one touch contact; click is tap, drag is swipe,
and wheel pulses become short bounded touch swipes. Coordinates are mapped from
the fixed stream size to the device view. Geometry/orientation changes disable
input until a fresh stream. A held contact has a five-second inactivity limit.
Middle/right buttons and OS-level shortcuts are not supported.
Mouse viewers advertising RFB RichCursor receive a high-contrast local arrow,
so no physical mouse or device-side cursor is needed. The cursor is not burned
into captured pixels; touch input and permission checks are unchanged.

Printable Latin-1/Unicode keys and common editor keys use the existing editor
controller; Ctrl+A selects all. Each key is an atomic editor stroke, with no
persistent OS key-down. Permission, generation and non-password focus are checked
again on the UI queue immediately before editing. Clipboard messages are still
discarded; paste/OS command execution is not added. Audio/camera are not captured.

Permission revocation, a protected page, stop, disconnect, timeout or shutdown
cancels a held touch without completing a click or gesture and clears held-key
state. Permission generations fence even rapid off/on changes. A denied held
button must be released before a fresh gesture can start. The supporting
libcountertop dependency is pinned to its concrete fork revision; its cancellation
and guarded-editor changes are tracked separately and need the same owner
acceptance before merge.

## Capture and wire implementation

- Authenticate a fresh outbound carrier using exactly the one-time ticket and
  attach path from the current authenticated Node connection.
- Preserve the enrolled Gateway origin and mount prefix. Reject arbitrary hosts,
  encoded/alternate paths, mismatched/duplicated tickets and additional query
  fields. Raw tickets and signaling never enter results or diagnostics.
- Send binary authentication metadata first, then RFB 3.8 with a fresh ephemeral
  eight-character VNC password. VNC challenge/response is checked in constant
  time. TLS provides transport protection; VNC authentication is not encryption.
- Keep every outbound/inbound WebSocket message at most 64 KiB. Process
  fragmented RFB bytes incrementally with a bounded event channel and direct
  backpressure; no accumulating video/frame queue.
- Capture the actual Android composed screen with a private `screencap` process,
  not repeated screenshot RPCs or MP4 recordings. Validate the raw Android
  12-/16-byte header, RGBA/RGBX format, pixel dimensions and total byte count.
- Scale to at most 640 pixels on the larger dimension, preserving aspect ratio.
  The capture scheduler is capped at four samples/second. Tight JPEG quality 75
  is used when the viewer advertises it with a compatible pixel format; Raw
  true-color 8/16/32-bit fallback is available.
- Incremental requests avoid resending unchanged pixels. Empty updates keep the
  normal request loop alive on static screens; an inactive client cannot retain
  capture indefinitely. Resolution is fixed for each viewer session.
- Live streaming and MP4 recording share a busy gate. Snapshot contracts are
  unchanged. The shared UI surface helper is not modified or closed by streaming.

Each capture subprocess has a two-second deadline and bounded stdout. Explicit
stop, permission changes, Node disconnect, requester cancellation, observer close
and application shutdown cancel the carrier and current capture. No persistent
encoder or surface is created. Startup reads have a 15-second deadline; writes
have a five-second deadline; inactive RFB clients have a 45-second deadline.
Only `desktop.stream` with `timeoutMs: 0` receives the cancellation-owned
unbounded invocation; other commands keep the existing 30-second cap, and a
positive explicit streaming timeout remains bounded.

The local status contains only state, delivered-frame count and output size;
errors use fixed strings. No credential, peer address, observer URL or raw
signaling is logged or returned.

## Verification and limitations

Automated checks cover default-off upgrades, unchanged screenshot permission,
strict attach-target validation, zero/positive invocation timeout semantics,
RFB authentication/initialization, Raw pixel order, Tight JPEG decoding,
fragmented carrier input, hostile pixel layouts/clipboard sizes, input grant
gating/generation fencing, coordinate/key translation, non-activating contact
cancellation and UI-queue editor guards, and teardown
for local stop, revocation, disconnect, cancellation, peer close and timeout.
Existing Node, settings and control suites remain regression gates.

Actual device, real Gateway observer and browser-viewer results must be reported
separately in the PR. Four samples/second and the 640-pixel bound are configured
limits, **not measured delivery rate or end-to-end latency guarantees**.
Raw fallback consumes considerably more bandwidth than Tight JPEG. Capture and
JPEG encoding cost CPU; there is no hardware H.264 acceleration in this RFB path.
There is no desktop audio, clipboard, OS-key injection, resize protocol extension
or change to `screen.snapshot`/`screen.record`.

A protocol client successfully receiving frames is not proof of playback in the
actual OpenClaw viewer. Final owner acceptance must name the tested commit;
keep the PR draft and issue open until all required checks and acceptance.
