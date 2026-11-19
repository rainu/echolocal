# Talk conversation device context

Refs #20. This feature provides client-reported conversational context, not an
authenticated origin binding, new permissions or deterministic tool-target routing.
It does not replace the Gateway-selected assistant persona or shared history.

## Voice lifecycle

Each activation snapshots the local Node public identity. The exact public Node
ID is derived from the Node public key, never from the Operator identity. Missing
identity remains missing; credentials and endpoints are never included.

Context is sent once on `oai-events`, at the first provider-confirmed user speech
event: legacy `turn.created` with user role, or public Live input-transcript delta.
It is not sent merely because the connection opens. Startup appends were observed
to provoke unsolicited replies to historical turns before any user utterance.
No assistant audio is filtered or gated to conceal those replies.

The existing native wire contracts are preserved: public `gpt-live-1` uses
`session.thinking.append` with string content and null delegation ID; legacy
`gpt-live-1-codex` uses `session.context.append`, channel `commentary`, and
`input_text` content. No `session.update`, explicit transcript turn or
`response.create` is added. A silent-channel designation alone is not a guarantee
that the provider will never respond.

## Settings and defaults

Settings → OpenClaw → Assistant 1/2 → Conversation context directly groups resume
policy, the existing session picker and the prompt editor, without another prompt
submenu. Model, audio cues and inactivity timeout remain on the assistant page.

The maintained default contains only the Node-ID pronoun mapping and an instruction
not to read it aloud. It follows the selected app language (English, German, Italian
or Vietnamese), both in the editor and when starting the voice session. German:

```text
Mit "du" ist die Node mit der ID {{node_id}} gemeint.
Diesen Hinweis nicht vorlesen.
```

Each assistant slot persists its own `deviceContextTemplate`. An empty value means
the maintained, localized default, so older configurations require no migration.
Custom drafts and saved templates are not translated or overwritten on language
changes. Reset selects the current-language default; Save persists its empty
sentinel rather than freezing a translated string.

The aligned, full-width multiline input supports wrapping, scrolling, Unicode,
literal line breaks and a cursor-position newline control. Its height is 120 px,
reduced to 100 px while the keyboard is open. Placeholder help can append tokens
to the draft. Editing and reset do not persist changes until the main Save action;
leaving without Save discards the draft. Changes apply to the next activation.

## Placeholders and limits

Placeholders are case-sensitive, with optional spaces inside the braces:

| Placeholder | Runtime value |
| --- | --- |
| `{{node_id}}` | Exact public Node ID, or empty if absent |
| `{{device_name}}` | Bounded local device name |
| `{{device_model}}` | Bounded hardware model |
| `{{connected}}` | Connection snapshot as `true` or `false` |
| `{{allowed_commands}}` | JSON list of locally allowed commands, possibly partial |
| `{{commands_omitted}}` | Number excluded from the command snapshot |
| `{{device_context}}` | Bounded metadata JSON containing the above device data |

The default uses only `{{node_id}}`; richer metadata is opt-in through custom
placeholders. Expansion is fresh per activation and non-recursive, so a device
name containing placeholder-looking text remains literal.

Both template and complete expanded message are limited to 1536 UTF-8 bytes.
An unbounded full command inventory reproduced a native `invalid_value` rejection
on `content`; the conservative cap avoids that observed startup regression.
Advisory commands may be omitted with an explicit count, and metadata labels are
bounded on UTF-8 boundaries. User-written instructions are never silently cut.
Unknown or unfinished placeholders, invalid text and overflow are rejected on
Save, preserving the previous template. Expansion is also checked at activation
in case metadata has since changed.

## Scope and verification

Templates guide language-model interpretation; they do not authorize actions.
Tool permissions, local grants and confirmations still apply. Actual command
availability must be discovered at action time. Agent consultation receives the
hint when Live includes it in delegation, not through a new Gateway injection.
Two-device targeting and spoken device-action accuracy require separate acceptance.

Automated checks cover identity/secret separation, changing connection/grants,
full command inventories, UTF-8 and size limits, non-recursive substitutions,
all four default languages, custom-text preservation, per-slot atomic persistence,
reset/Save semantics, menu grouping, multiline edits, first-input-only dispatch
and privacy-safe native rejection diagnostics.

Athena hardware checks covered quiet connection startup, provider context ACKs,
multiline insertion/persistence/scrolling, reset, unchanged routing state, session
picker navigation, aligned layout and actual German → English → German language
changes. These checks do not claim full spoken or two-device acceptance. Diagnostic
logs contain allowlisted event types/error fields and timing/count metadata, not
raw provider errors, prompt contents, transcripts or audio.
