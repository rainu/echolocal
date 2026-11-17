# OpenClaw connection protocol preflight (issue #7)

Status: blocked before implementation. This document is not an implemented client.

## Verified contract

Reviewed the installed Gateway implementation and its bundled documentation on
2026-11-17. References below name source symbols rather than deployment paths.
No live enrollment or credential access was performed.

- Gateway protocol: challenge-based signed device identity; use the server
  challenge timestamp and nonce. Device tokens are scoped by device and role.
- A node is a capability host, not a chat/operator client. This milestone should
  declare role `node`, empty scopes, capabilities, commands and permissions.
  No shell, camera, microphone, chat or administrator authority is needed.
- Setup links use `oc-pair://<setup-code>` and a short-lived bootstrap credential.
  Generic node-host bootstrap and mobile setup handoff are different: the latter
  can also issue operator credentials. Do not silently select mobile handoff.
- Device pairing and node capability approval are distinct. Device requests have
  a five-minute expiry; capability approvals do not expire merely with time.
- Reconnect must preserve identity, serialize sockets and use bounded backoff.
  Explicit disconnect must persist the disabled reconnect preference.
- Normal certificate verification must remain enabled. A setup link certificate
  pin must not silently replace normal certificate verification for this issue.
- A node cannot perform remote revocation with its node token. Forget must be
  local-only, with Gateway-side removal instructions.

References: OpenClaw docs `gateway/protocol/auth.md`,
`gateway/protocol/handshake.md`, `gateway/pairing.md`.

## Blocking mismatch: rejection and expiry observation

The installed handshake implementation computes:

```ts
const waitForResolution = state.isControlUi && role === "operator";
```

Only that branch registers `waitForDevicePairingResolution`, responds with
`waitForResolution: true`, and sends a request/device-bound
`device.pair.resolved` event before closing the pre-auth socket.

For role `node`, the failed handshake instead calls
`failPairingHandshake`. Pending node requests carry retry guidance
(`wait_then_retry`); there is no equivalent resolution subscription for the
unadmitted node. The auth documentation explicitly says no denial history is
persisted. Reconnecting cannot reliably distinguish rejection, expiration,
supersession, or a fresh request.

Therefore a node-only Echo client cannot truthfully meet the required distinct
rejected/expired states and avoid creating new requests after rejection using
the currently verified contract. A local timeout is not proof of Gateway expiry.
Impersonating Control UI or adding operator permissions solely for this is not
an acceptable workaround.

## Decision needed

Preferred prerequisite: extend the Gateway's signed, unadmitted node handshake
to expose only its own exact pairing request's resolution, without admitting
RPC access or exposing other devices' events. Specify handling of disconnect,
supersession and reconnect, and test rejection/expiry before implementing the
Echo wizard.

Alternative scope change: explicitly accept an indeterminate approval outcome
for nodes and require manual Retry; do not label it rejected or expired.
That does not satisfy the current acceptance criteria without owner agreement.

Gateway changes are outside rainu/echolocal issue implementation scope and have
not been attempted. No Echo software, Home Assistant settings, Gateway settings,
or credentials were changed. Hardware ADB connectivity was checked successfully;
no UI, TLS handshake, pairing, reboot or reconnect acceptance tests were run.
