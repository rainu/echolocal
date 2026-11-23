# Live screen streaming: integration investigation (issue #15)

Status: **design/investigation only; no live-stream implementation or hardware
acceptance is claimed.** Existing snapshot and bounded MP4 contracts are unchanged.

## Verified reference points

Investigated on 2026-11-23:

- OpenClaw release `2026.11.6`, revision `06afb10`: installed desktop command
  policy, stream broker, node desktop source, observer bridge and bundled viewer.
- Current upstream OpenClaw source pinned at
  [8b17822bec4f36582bf5e15743087b32fd3460b1](https://github.com/openclaw/openclaw/tree/8b17822bec4f36582bf5e15743087b32fd3460b1).
  Upstream observations must not be mistaken for deployed runtime behavior.
- EchoLocal accepted integration `2d93f969d43e536573f6a21002de1c80c471f115`:
  `internal/openclaw/node.go`, `node_invoke.go`, `node_permissions.go`,
  `internal/feature/control/node_screen.go` and existing Node command docs.
- This documentation branch starts from the existing clean local main baseline
  `f6b596d4b966f8124710fb2bd677427e5d4614a5`. It does not silently import the
  integration branch's other features. Future implementation must resolve the
  existing Node feature dependencies explicitly.

## Actual OpenClaw contract

`desktop.stream` exists. It is a cancellable Node invocation controlling a
separate **RFB/VNC byte stream**, not a command returning video or a video URL.

1. An authenticated viewer requests `desktop.observe` for the selected Node.
   The Gateway verifies current pairing generation, effective approved commands
   and requester authority.
2. The Gateway invokes `desktop.stream` with precisely `ticket` and
   `attachPath`. The ticket is a one-time, short-lived bearer capability bound
   to that Node connection and pairing generation. Neither value belongs in
   normal diagnostics or command results.
3. The Node opens a dedicated outbound WebSocket to the connected Gateway.
   The attach path must remain on the enrolled Gateway origin, preserve its
   reverse-proxy mount path and match the expected desktop attach endpoint.
   A caller-selected destination, public listener or new pairing identity is
   not part of this contract.
4. The first WebSocket message is binary JSON authentication metadata:
   `auth: "vnc-password"` or `auth: "ard-account"`, optionally
   `vncPassword`. Subsequent messages carry the ordered RFB byte stream.
   The installed broker accepts at most **64 KiB per attach frame**, including
   metadata. Backpressure and stream teardown must be bounded.
5. The Gateway authenticates the RFB server before presenting its no-auth RFB
   handshake to the browser. Unauthenticated local RFB is not accepted by the
   built-in desktop Node implementation.
6. The viewer receives an RFB observer descriptor, then opens its separate
   authenticated observer WebSocket. The Control UI uses the bundled noVNC
   viewer. Framebuffer bytes do not travel through `node.invoke.result`.
7. Viewer release, disconnect, revoked authority, changed pairing generation
   and command cancellation tear down the carrier. Unused tickets expire;
   reconnect establishes a fresh session rather than replaying an old ticket.

Reference implementation:

- [Node desktop command](https://github.com/openclaw/openclaw/blob/8b17822bec4f36582bf5e15743087b32fd3460b1/src/node-host/desktop-stream-command.ts)
- [Binary Node stream transport](https://github.com/openclaw/openclaw/blob/8b17822bec4f36582bf5e15743087b32fd3460b1/src/node-host/node-stream-transport.ts)
- [Wire constants](https://github.com/openclaw/openclaw/blob/8b17822bec4f36582bf5e15743087b32fd3460b1/src/shared/node-desktop-stream.ts)
- [Gateway command policy](https://github.com/openclaw/openclaw/blob/8b17822bec4f36582bf5e15743087b32fd3460b1/src/gateway/node-command-policy.ts)
- [Desktop Node source](https://github.com/openclaw/openclaw/blob/8b17822bec4f36582bf5e15743087b32fd3460b1/src/gateway/desktop/node-source.ts)
- [Attach broker](https://github.com/openclaw/openclaw/blob/8b17822bec4f36582bf5e15743087b32fd3460b1/src/gateway/desktop/node-stream-broker.ts)

## Android admission and scope boundary

EchoLocal declares the existing separately paired Node as Android/tablet.
The built-in desktop host enables desktop sharing for macOS, Linux and Windows,
not Android. In the installed Gateway command policy:

- Android does not receive `desktop.stream` by platform default.
- Automatic admission of paired desktop host commands is desktop-platform-only.
- An explicit Gateway command allow entry can admit the command on Android,
  subject to an explicit deny, declaration and pairing approval.
- The environment summary derives desktop availability from the effective
  allowed `desktop.stream` command; that summary has no independent Android
  exclusion.

Consequently **no mandatory upstream protocol change has been established**:
an owner-authorized explicit allow plus an approved Android capability upgrade
may be sufficient. This remains a source finding, not a successful live test.

Runtime Gateway policy inspection was denied because the scheduled worker lacks
owner authority. It was not retried through another access path. Effective
runtime admission is therefore **unknown**, not proven absent. Gateway policy
changes and capability approval require a separate owner-authorized step.
Changing platform metadata to Linux, borrowing the operator connection or
issuing a new identity would not be a valid workaround.

## Capture/codec integration gap

The existing device snapshot adapter obtains a composed image. The existing
Android recording helper emits H.264 at 15 fps and the recording adapter muxes
it into bounded fragmented MP4. Neither is an RFB server.

Sending H.264 access units, repeated MP4 recordings or base64 snapshots to the
desktop attach socket would violate the verified RFB contract. Merely adding
`desktop.stream` to the Node catalog would expose a nonworking capability.

Two implementation paths remain to be evaluated:

1. Implement a bounded, **view-only RFB producer** backed by composed display
   capture and compatible authentication, then use the supported Node carrier.
   This can use an in-process duplex rather than expose a network listener.
   It must consume but never execute input/clipboard messages even if a viewer
   asks for control. Capture quality, overlays, update encoding and CPU costs
   need actual device measurement; repeated encoded still images returned as
   RPC results are not a substitute.
2. If direct hardware H.264 transport is required, extend OpenClaw's supported
   desktop transport and receiving viewer first. This would be a separately
   scoped upstream dependency, not an EchoLocal-only private command.

The Node lifecycle currently imposes a 30-second maximum invocation timeout,
including incoming `timeoutMs: 0`. The desktop contract uses zero for an
unbounded invocation owned by cancellation. A narrowly scoped streaming
lifecycle change is needed; existing bounded commands must keep their limits.

## Proposed permission and lifecycle requirements

These are requirements for future code, not implemented behavior:

- Keep the existing Node identity, credentials and operator separation.
- Gate declaration and every start through a dedicated local sharing decision;
  specify upgrade defaults explicitly rather than silently enabling continuous
  capture through an old screenshot grant.
- Honor local denial/revocation even when the Gateway requests control.
- Only start capture for the current authorized carrier; one bounded active
  session initially, with explicit busy/status behavior.
- Stop and release capture, authentication state, buffers and sockets on local
  stop, revocation, Node disconnect, invocation cancellation, startup timeout,
  failed authentication, viewer disconnect and application shutdown.
- Enforce startup/idle/write deadlines and bounded queues. A slow viewer must
  not accumulate arbitrarily old frames.
- Return only sanitized lifecycle outcomes. Do not log tickets, observer URLs,
  authentication metadata, connection credentials or raw signaling.

## Required verification before implementation acceptance

| Area | Required evidence | Current status |
| --- | --- | --- |
| Protocol | RFB handshake/authentication, binary metadata ordering, 64 KiB framing, hostile/fragmented messages, backpressure | Source investigation only |
| Admission | Same Android identity, explicit effective allow, approved capability, actual Desktop picker | Blocked on owner-authorized Gateway inspection/approval |
| Viewer | Real continuously changing display viewed through the actual OpenClaw viewer | Not performed |
| Local policy | Denied start, revocation during capture, explicit local stop | Not implemented |
| Cleanup | Repeated start/stop, disconnect/reconnect, cancellation, timeout, shutdown, no residual capture/encoder resources | Not implemented |
| Performance | Sustained device CPU/RSS, frame delivery rate, resolution, end-to-end latency and memory/resource stability | Not measured |
| Regression | Existing PNG/JPEG snapshot and bounded 15 fps MP4, separate paired connections | Existing contracts inspected; no new runtime regression test |

Do not advertise a supported streaming resolution, frame rate or latency until
measured on hardware through the actual viewer. The recording helper's 15 fps
is not evidence for live-stream performance.

No application build, deployment, capture session or device-setting change was
performed for this investigation. Issue #15 remains incomplete and must remain
open until a concrete implementation is tested and accepted by the owner.
