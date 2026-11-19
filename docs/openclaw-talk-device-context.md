# Talk conversation device context

Refs #20. Owner narrowed this feature to conversational context, not authenticated
origin binding or new routing/authorization authority. The original issue's
server-authenticated-binding proposal is not implemented by this client change.

## Path

Before each voice activation, the Talk manager asks the separate Node client for
`ConversationDeviceContext()`. It derives the exact public Node ID by hashing the
Node public key, just like the Node connect handshake; it never uses the Operator
identity. The snapshot contains the advertised local name, hardware model,
connection state and locally permitted command names. No endpoint, token, private
key or transcript is exported. A missing identity stays missing.

After WebRTC/control readiness and before opening the room microphone, Session
sends a single silent context append on `oai-events`. It uses the installed OpenAI
provider's native contracts: legacy `session.context.append`, channel `commentary`,
with `input_text` content; public `gpt-live-1` uses `session.thinking.append` with
string content and null delegation ID. This does not replace Gateway-selected
instructions/history, create a transcript turn, trigger a response or force
consultation. No unsupported fields are added to `talk.client.create`.

The hint explains device-relative pronouns (including German), preserves the
assistant persona and explicit other-device requests, and asks Live to include
the device hint when consulting the agent about a device action. Actual commands
still use existing node tools and all normal permissions/confirmations. This is
model-mediated contextual understanding, NOT a deterministic tool-target override
or a server-verified physical source. Agent consultation receives device details
only when Live includes them in its delegation; there is no new Gateway injection.

Every new activation takes a fresh snapshot, including resumed conversations.
The hint is not explicitly appended to shared transcript/history or local logs.
The model may naturally mention the device in replies; such ordinary replies
retain the existing history behavior. Current connectivity/permissions must be
rechecked at action time; the snapshot is not a live authorization grant. No
unrelated node may be substituted when the requested device is unavailable.
Other clients and non-Talk turns receive no new context. Native provider context
compatibility is bounded to the above wire contracts; no generic-provider support
or two-device hardware acceptance is implied by unit tests.

## Verification

Unit checks cover public identity versus private state, non-Node omission, missing
identity, connectivity refresh, permission changes, both native wire variants,
and unchanged defaults for unrelated sessions. Real speech checks must distinguish
recognition, correct device interpretation and an actual tool result.

For reproducible manual acceptance: activate on A and ask which device "your
volume" refers to, then request a permitted audio status and an independent
system/display status. Verify returned targets and real results. Explicitly name
B and ensure that request is not rewritten to A. Repeat on B, alternate resumed
sessions, rename/reconnect A, and check fresh context. Disconnect/deny the target
and verify an accurate explanation without fallback. Do not interpret a spoken
success as invocation evidence. Physical audio quality and two-device cases need
separate hardware tests.
