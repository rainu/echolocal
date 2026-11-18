# OpenClaw connection milestone (issue #7)

## Owner clarification

The first milestone is the same device-pairing and connection model as
[rainu/openclaw-pet](https://github.com/rainu/openclaw-pet), not node capability
registration. The owner explicitly clarified this after the node-protocol
preflight. Node presence, chat, session subscriptions, voice and device commands
are not implemented here. No Gateway modification is required.

## Wire contract

- WSS only, normal certificate and hostname verification, TLS 1.2 minimum.
- Protocol 4: `connect.challenge`, signed `connect`, correlated `hello-ok`.
- Stable Ed25519 identity, SHA-256 public-key device ID, v3 signature binding
  nonce, timestamp, client ID/mode, role, scopes, credential and platform metadata.
- Supported Pet client identifier `openclaw-linux`, mode `ui`, role `operator`.
  Metadata reports EchoLocal version, Android platform and tablet device family;
  the identifier is a protocol identifier, not a claim to be the Pet binary.
- Empty requested scopes. No administrator, session-read, Talk or node authority.
- Manual WSS endpoint and masked bootstrap-token entry, then explicit device
  approval in the Gateway, then explicit Retry. No invented setup-code decoder.
- Explicit private provisioning may use a short-lived operator-only, empty-scope
  bootstrap credential via auth.bootstrapToken (ConfigureBootstrap). Shared-token
  manual entry remains separate; credential kinds are never guessed from prefixes.
- Successful admission stores the returned role-scoped device token and removes
  consumed short-lived bootstrap credentials. A manually entered shared Gateway
  token remains privately stored and editable, while reconnect prefers the device token. Reconnect uses the device token and unchanged identity.

Reference: Pet `backend/gateway.go`, especially `buildConnectParams`,
`loadOrCreatePetIdentity`, `pairingStatus` and the startup retry loop.

## Truthful states

`pairing` means approval is required, not that the Gateway promises the request
is still pending. Like Pet, a non-Control-UI client cannot observe a reliable
request-bound denial/expiry decision. It stops and requires explicit Retry after
approval; it never invents `rejected` or `expired` from local timeouts, and never
automatically repeats pairing or authentication rejection.

Transport failures retry with bounded 1–30 second exponential backoff. There is
one lifecycle owner; a new action cancels and joins the previous owner before
starting another socket. Disconnect persists the disabled reconnect preference.
An enabled saved connection resumes at application startup.

## Storage and diagnostics

The separate device-local `openclaw.json` file is atomically replaced with mode
0600. It is deliberately outside general settings export/config synchronization.
At the owner’s request, the private on-device editor receives the saved endpoint
and effective credential so both fields remain editable. The endpoint is visible,
the token is masked. Never publish screenshots of a configured editor or serialize
these values into diagnostics, exports or public artifacts. Private keys are never
returned to the editor. Changing endpoint drops the old identity and tokens;
blank bootstrap input preserves a bootstrap credential only on the same endpoint.

Forget has a confirmation screen and removes local credentials, not remote
Gateway authorization. Pair again retains identity, drops the cached device token
and disables reconnect. A new bootstrap credential may then be required.

The diagnostic ring holds at most 64 fixed-class lifecycle/application events in
memory, with direction, timestamp and result. Raw messages, arbitrary Gateway
error text, event names, URLs and credentials never enter it. Payload debugging
and diagnostic export are not implemented in this connection-only milestone.

## Verification boundary

Synthetic tests cover signature/least privilege, pairing-required manual retry,
TLS trust failure, auth rejection, credential-file permissions/persistence,
forget, reconnect, explicit disconnect, malformed identity and diagnostic
redaction. These are not a real device enrollment or owner acceptance.

Device installation, screenshots, real pairing, restart and interaction results
must be reported separately against the actual tested commit. Keep the issue and
PR open until the owner accepts that implementation.

The editor uses native action-bar buttons for Save, Connect, Disconnect, Pair again,
Forget and diagnostic clearing. A changed credential invalidates cached device auth
so edits cannot silently be ignored; saving unchanged fields preserves approved auth.

OpenClaw action buttons request a 64-pixel minimum height using the optional shared
action MinHeight field (rainu/libcountertop PR #4). Other screens retain default
button sizing. Redundant action-description rows are omitted; nonempty save/error
feedback is shown without a permanent status label. The module pin uses a concrete
published library integration-preview commit to retain accepted integration features.
