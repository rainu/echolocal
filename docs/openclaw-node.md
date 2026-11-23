# EchoLocal Node registration

EchoLocal keeps two independent WSS connections: the existing operator/Talk
connection and a Node connection. Settings → OpenClaw → OpenClaw Node edits only
the latter. Its credential file is separate, atomically saved with mode 0600;
no operator token or identity is copied. Existing settings and assistant routing
are not migrated or modified.

## Pairing

Issue a short-lived Gateway bootstrap credential restricted to role `node`
with empty scopes. Enter the WSS endpoint and this credential on the private
Node settings page, Save, then Connect. Do not enter an operator/Talk token or
a shared administrator credential. Fresh Node editor credentials are explicitly
sent as `auth.bootstrapToken`, not guessed from their spelling.

The protocol-4 connection uses `openclaw-android`, mode `node`, role `node`,
a persistent Ed25519 identity and the nonce-bound v3 device signature. The
Gateway controls device approval and Node capability approval. Approve only
the exact EchoLocal identity and its empty command surface. If approval is
required, the page reports it and waits for an explicit Connect retry.
Successful enrollment stores the role-specific device token and removes the
consumed bootstrap credential. Unexpected hello roles/scopes are rejected.

The Node advertises Android, the detected board's model/name, and the exact
EchoLocal build version. Both `caps` and `commands` are empty: no shell, audio,
camera or arbitrary action is implemented or advertised by this connection.
Talk remains exclusively on the separately authorized operator connection.
No permission toggles or Gateway configuration changes are required.

## Lifecycle and privacy

Transport loss retries with bounded exponential backoff; app restarts retain
the same identity and reconnect preference. Disconnect persists disabled state.
Pair again retains identity but clears the cached role token; Forget clears only
local Node state and does not revoke Gateway approval. Service shutdown closes
both sockets without changing either reconnect preference. Node failure does
not control operator/Talk startup, assistant selection or Home Assistant.

The Node page reports the actual socket state, not pairing history as online
status. Connection diagnostics retain at most 64 whitelisted events, never raw
Gateway errors, endpoints, tokens, audio or conversation contents. The private
editor displays the endpoint and masks credentials; do not publish screenshots
of it.

## Verification

Synthetic TLS-WebSocket tests cover the Node signature and handshake, empty
authority surface, unexpected hello authority, separate identities, approval,
transport reconnect, persisted role token, restart, disconnect, forget and
diagnostic redaction. Existing operator/Talk suites remain regression checks.
Mock tests are not proof of live Gateway enrollment or real audio behavior;
hardware results and the exact tested commit are recorded separately at issue #11.
