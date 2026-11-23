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
