# Operator permissions

Refs #16.

The connection menu separates Operator permissions (authority to act through the
Gateway) from Node permissions (local device-function grants). Neither setting
changes the other role, identity or Gateway Node-command policy.

## Protocol and defaults

The supported closed set was verified against Gateway 2026.11.4,
src/gateway/operator-scopes.ts (packaged operator-scopes module):

- operator.admin
- operator.read
- operator.write
- operator.sessions.read
- operator.sessions.write
- operator.approvals
- operator.questions
- operator.pairing
- operator.talk
- operator.talk.secrets

All permissions explicitly requests all ten scopes. Admin is not substituted
for the remaining scopes. The signature covers the exact selected list.
The per-operator identity file stores operatorScopes as an explicit array.
A missing preference means All (fresh installation or migration); an empty
array means no scopes. Explicit saved selections survive upgrades. Enabling
local Talk does not widen this selection. Node handshakes still request no
operator scopes.

## Changes and authorization

Changing the selection stops the existing socket before persisting and
reauthenticating. It preserves the device key and cached token. A previously
disabled connection stays disabled. The operator client first uses its cached device token. A rejected scope
expansion reconnects the same identity with no scopes, then uses the supported
owner-bound device.scopes.requestUpgrade / device.scopes.waitUpgrade methods.
Pending upgrades cannot use Talk. Approval supplies a new device token, which
is stored privately before a fresh signed handshake requests the selected scopes.
Rejection and expiry disconnect and show explicit sanitized status.
No shared credential, identity reset or automatic approval is required. Node authentication is
unchanged. Without a suitable credential an expansion can be rejected before
pairing: edit the private Connection credential and retry. Never reset the
device identity to obtain an upgrade.

The UI shows requested scopes independently from scopes reported by the active
hello response. Disconnected, pending or failed authentication has no active
grants. Approval-required remains disconnected until the user approves at the
Gateway and retries Connect. The application never approves requests itself.
A rejected upgrade remains an authentication/pairing failure, never a claim of
granted authority. Raw Gateway error text, tokens and identity values do not
enter these diagnostics.

Reducing scopes replaces the active connection. It does not revoke the broader
authorization stored at the Gateway: revoke that separately if necessary.
Local Node grants still apply independently to every incoming command.

## Validation

Unit tests cover missing-preference migration, explicit empty/restricted
persistence, signed handshake derivation, role separation, identity continuity,
pending approval and retry, live reconnect grants and grant clearing.
Device and voice validation results belong in the issue; unit tests alone are
not proof of an approved Talk-to-Node round trip.

## Gateway upgrade schema compatibility

Gateway 2026.11.4 recognizes ten scopes but its requestUpgrade schema accepts at
most eight entries. Its verified shared/operator-scope-compat rules imply session
read/write through operator.write or operator.admin (session read also through
operator.read). Only when exceeding that schema limit, the approval request omits
already implied session entries. The saved selection and signed handshake still
explicitly request every selected scope, including all ten for All permissions.
No authority is added beyond the user's selection.
