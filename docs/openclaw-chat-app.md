# OpenClaw chat application

Refs #32. The home action rail and app launcher open a dedicated native app,
not a settings page. The left sidebar lists existing non-archived sessions,
using the Gateway's custom group catalog/category and order. Groups collapse
locally; session selection and drafts survive reconnects while the app is open.
The sidebar can be hidden explicitly. Home/back exits the app and stops only
Talk owned by this app. Existing connection/pairing settings are unchanged.

## Protocol and authority

Verified against the installed Gateway protocol and Control UI implementation:

- `sessions.list` with sidebar row mode and offset pagination;
  `sessions.groups.list` returns `{groups:[{name,position}],sectionOrder}`.
- `sessions.subscribe` invalidates the list; `sessions.messages.subscribe`
  binds the current conversation observer. `session.message`, `chat`, status,
  and session-change events trigger authoritative bounded history reads.
  Ten-second polling recovers missed invalidations. Closing unsubscribes the
  message observer. Reconnect uses a new operator binding, not the Node client.
- `chat.history` reads 50 messages / 128 KiB at a time. Older messages use the
  returned opaque `olderCursor`. User/assistant text blocks render as native
  Markdown (including code); tool payloads/thinking blocks are not rendered.
  Remote/local images and external math/diagram APIs are disabled.
- `chat.send` includes the selected key, existing session ID and a random
  idempotency key. No mutation is automatically replayed on reconnect. An
  unconfirmed send keeps its draft; explicit retry of identical text to the same
  session reuses the idempotency key. Changed text is a new message.
- `models.list` is scoped to the selected session. `sessions.patch` changes only
  its model with `expectedSessionId`, including reset to agent default.
- The operator adapter checks current authenticated read/write grants and fences
  each operation to its original socket. It does not request new grants,
  enable Talk, alter connection settings, or borrow Node capabilities.

Unavailable scopes/actions, lost connections, history errors and uncertain sends
are visible. Session identity replacement/removal disables actions until the user
selects a current row again. Sending, model mutation, and active/draining Talk
block session switching. Late callbacks are fenced by view and selection epochs.

## Talk

The existing local Talk manager now accepts an explicit existing session key.
It reuses assistant slot 1's voice/provider/model/ending/timeout preferences,
but does not rewrite that slot or generate a new session. The app stays visible
with the selected session identity and a Stop Talk control. Existing microphone,
privacy mute, audio ownership, captions, transcript persistence and connection
fences remain in use. Stop retains the selection fence until audio teardown has
finished. Leaving the app cancels its owned generation, not a later wake-word call.

The Talk protocol accepts `sessionKey`, not an expected session ID. External
session resets racing a live voice call are governed by the Gateway's Talk
lifecycle; the app never silently switches speech to another selected key.

## Verification

Focused controller tests cover uncertain send/retry identity, in-flight switching,
late acknowledgment after close, drafts by session, Talk stop fencing, group/history
wire shapes, session replacement, and filtering non-text blocks. Adapter tests
cover separate Node authority, scopes, socket replacement, and content-free events.
Existing Talk and native GUI tests remain required. A successful build alone does
not prove real voice/audio behavior; device evidence and remaining limits belong
in the issue/PR report for the exact tested commit.
