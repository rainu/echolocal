# Shared OpenClaw connection settings

OpenClaw → Connection owns one WSS address and one masked shared Gateway-token
field, plus two editable advertised device names, two independent status rows,
Node permissions, and separate role diagnostics. The root menu retains only
Connection, the two assistant slots and conversation history.

Defaults use the hardware name without an EchoLocal prefix and distinguish UI/Node.
Exact former generated defaults are upgraded; custom names are preserved. A name is a display
label, never a device identity or authority. Blank names restore defaults; names
are limited to 80 Unicode characters with no control characters.

Private common state lives in openclaw-connection.json (atomic 0600 writes).
Existing UI and Node identity files remain separate and retain their private
keys, Gateway-issued device tokens, role scopes and enabled preferences.
The WSS address and an explicitly entered shared Gateway credential are the
common source. Role-scoped device tokens are never copied to the shared form,
or to the other role. Short-lived per-role bootstrap credentials are likewise
not promoted to a shared Gateway token. The single form accepts a shared Gateway
credential; a one-role bootstrap code is not interchangeable with it.

Migration picks the existing UI endpoint, otherwise the Node endpoint. Existing
role credentials are retained only for the same Gateway. Existing ordinary
Gateway bootstrap credentials may migrate, but cached device tokens are never
used as the shared form's fallback. Already paired connections need no fresh
token entry. No role credentials/configuration are logged by migration.

Save validates the form before stopping either connection, keeps both role
operation locks while applying changes, and commits common state last. Failed
saves restore the role snapshots; storage failures fail closed. Unchanged saves
do not reconnect. Name-only edits retain keys, device tokens and scopes and
reconnect previously enabled roles. Replacing a nonempty shared Gateway token
or changing the endpoint invalidates both cached device tokens. Clearing the
shared token removes the shared bearer but preserves already paired device
tokens. Enabled preferences are retained, not silently set on for disabled roles.

Connect/disconnect controls apply to both roles. Re-pair keeps their distinct
keys. Forget explicitly confirms removal of both local identities plus common
configuration and does not revoke Gateway-side approvals. Permission grants
remain Node-local and separate from operator/Talk authority.

Connection status rows refresh on state changes. Local root diagnostics:
`ctl connection status` returns only states, names and configured booleans,
never addresses, tokens, keys or conversation content. It is not a Node command.

Verification covers fresh configuration, legacy migration, role isolation,
names/defaults, reconnect with stable identity, failed-save rollback, malformed
storage, localized UI structure and one masked credential field. Device install,
visual inspection and actual Gateway reconnect are separate acceptance stages.

The connection form omits technical token/name/save explanations. Permissions
use a plain device-functions heading; each diagnostics page repeats its distinct
Controller UI / Node entry title for orientation.

## Accepted validation scope (2026-11-19)

Owner accepted the installed Node tools and shared connection UI for integration.
Five targeted Go packages passed tests and vet; Athena build and official
no-reboot installation succeeded. Installed binary matched the build artifact;
both roles remained paired and connected with the shorter hardware-based names.
Legacy-default migration preserves custom names and role identities.
The documented streaming, recording/privacy and adapter limitations remain
explicitly outside the implemented command surface.

## Startup and retry lifecycle

Enabled is a per-role persisted user choice, not a cached connection state.
Startup resumes only enabled roles; lifecycle Close never disables them, whereas
Disconnect does. Start is idempotent: repeated lifecycle starts do not replace
a live connection or repeat registration.

Both role clients wait for Wi-Fi/address readiness and a valid device clock
before attempting a signed TLS handshake. Node setup is installed before its
client starts. A network or clock wait is cancellable, uses the existing bounded
1/2/4/8/16/30-second backoff and does not block the app boot indefinitely.
Boot readiness evaluates **both** configured roles. The eight-second startup
summary deadline does not stop background retries.

Transport loss, timeouts and service-restart closes before either handshake
message are retryable, not malformed protocol responses. A complete malformed
JSON frame, protocol violation, authentication refusal or pairing requirement
remains terminal until explicit user action. Certificate date validity is
retried to allow clock synchronization; all attempts still perform full TLS
verification. Untrusted issuers and hostname mismatches remain terminal.
Backoff retains the sanitized reason instead of replacing it with "network".
No endpoint, server-provided message, credential or identity enters diagnostics.

Regression coverage includes interrupted challenge/hello for both roles,
certificate-date correction without changing the process clock, delayed
prerequisite readiness with saved identities, duplicate Start suppression,
persistent explicit disconnect, cancellation, malformed-frame rejection and
manual retry. Simulated startup faults are not proof of a physical power-cycle
test; real-device reboot/restart checks and owner cold-start acceptance are
reported separately.
