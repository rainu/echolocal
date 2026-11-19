# Node tools implementation status — issue #13

Work in progress. This is NOT a complete mapping, released feature, or acceptance report.

## Confirmed foundations

- Isolated branch `issue/13-node-tools` starts from upstream main and incorporates the accepted integration branch. Shared checkouts are unchanged.
- Existing Node and operator clients have independent credentials and identities. Node currently advertises empty capabilities/commands and cannot invoke Talk.
- Gateway protocol uses `node.invoke.request` events with `id`, `nodeId`, `command`, JSON-encoded `paramsJSON`, `timeoutMs`, and optional idempotency key. Cancellation is a separate `node.invoke.cancel` event carrying `invokeId`. Do not treat invocation requests as ordinary request frames or invent a response method.
- Existing local Cobra control tree is not a suitable unrestricted Node surface: it contains profiler, networking, arbitrary file destinations, firmware and low-level hardware diagnostics.
- Existing input engine serializes gestures and defers touch release. Reuse this engine, with bounded coordinates/durations and cancellation, rather than generating independent touch injection.
- The settings registry supplies typed described fields and a Secret kind. Node access must exclude secrets by metadata and explicitly exclude authority/credential domains; an arbitrary substring filter is insufficient.
- Generic registry import is NOT permission-aware. Node import needs full field preflight before any write and must not use unrestricted Registry.Import.

## Implemented, not wired

`internal/openclaw/node_permissions.go` supplies a local-only persistent policy and bounded executor. Twelve functional groups default enabled when absent, explicit false survives reload, malformed existing policy fails closed, command advertisements are filtered and sorted, policy edits/disconnect cancel active invocations and suppress stale results. Four concurrent calls and 128 KiB arguments are the maximum. Policy edits use a private temporary file and atomic replacement. This code is not yet connected to Node sockets or settings UI.

Tests cover every group allowed/denied, persistence, upgrade defaults, corrupted policy, revocation/disconnect and invalid registration/arguments. Existing `internal/openclaw` tests pass with the configured Go toolchain. No hardware or Gateway tool tests have been performed for this issue.

## Inventory sources and remaining work

| Surface | Source | Required follow-up |
| --- | --- | --- |
| Actions | internal/feature/actions/actions.go | call, broadcast, alarm, television, call answer/hangup; inspect schemas and explicit group mapping |
| Settings/entities | component.Settings and component.Default().Entities() | enumerate actual eligible registries after component initialization; map every button, setting and sensor |
| Screen/UI | internal/feature/control/screen.go and libcountertop displaycmd/screencmd | JPEG shot, UI controls, notifications/choice; investigate supported stream transport |
| Input | libcountertop runtime/control/input.go | use shared arbitration, bounds, text lifecycle and cancellation tests |
| Audio | media, volume, playout, diag | player controls, announcement, test playback and background sounds |
| Alarms/timers | alarmcmd and feature/alarm, feature/timer | inspect complete CRUD/snooze surface |
| Calls/camera | feature/call, broadcast, vision, livecam | preserve mute/privacy and inspect finite vs streaming semantics |
| Tones | feature/tones and libcountertop audio/tones | inventory store operations and bounded upload/selection |
| Maintenance | feature/reboot, internal/update, diag | named app-only operations; no firmware/shell/network proxy |
| Diagnostics/state | component registry, logging and diag | explicit redacted projections; never arbitrary raw structs/logs |

Next: finish operation/permission matrix; wire real invocation protocol with socket lifetime and cancellation; register allowlisted existing services without import cycles; add local permission UI and advertisement reconnect; test all bypasses/invalid payloads/hardware support; compile Athena; install checksum-matched artifact with binary rollback; privately verify device and actual Gateway interactions; publish tested PR targeting rainu/integration and request owner acceptance. Do not merge or close before that acceptance.

No issue #13 binary has been built or deployed. No Gateway/network/firmware configuration has been changed.
