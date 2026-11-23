# OpenClaw owner configuration: device / web / API parity

## Baseline reproduction

On the installed integration build `968951b932ba961b1b9f55ca8adefb560e81f47d`,
the authenticated owner device API returned 200, unauthenticated access returned
401, `device.pages` did not contain `openclaw`, and `/api/v1/openclaw`
returned 404. Source inspection confirmed that the native Connection, permission,
and assistant editors use separate stores, not the generic settings registry.
The working Gateway connection was not diagnosed as broken.

## Complete audited settings/action matrix

All API paths below are relative to `/api/v1/openclaw`. The API is mounted
through the existing owner-authenticated `webapi.Options.Pages` boundary,
never as a Node command or a control-only/screen session route.

| Native device task | Web surface | Owner API | Persistence / conditions |
| --- | --- | --- | --- |
| Shared Gateway address, protected credential, UI/Node names | Connection form | GET root; PUT /connection | Same Connections.Save; separate role keys/tokens retained; address validation and 80-character name limit |
| Both-role connect/disconnect/retry after approval | Connection actions | POST /actions connect/disconnect | Same enabled preference and identities |
| UI-only / Node-only Connect | Per-role Connect | POST /actions connect with role ui/node | No grant widening; Gateway approval remains external |
| Pair again / forget both identities | Confirmed Connection actions | POST /actions pair_again/forget with confirm=true | Same lifecycle code; Gateway-side approvals are not revoked |
| UI/Node state and diagnostic events; clear events | Connection / Diagnostics | GET root/status; POST /actions clear_events | Bounded in-memory sanitized event ring; no credentials or conversation contents |
| Requested operator scopes / current granted scopes; all/read+Talk/none | Permissions | PUT /permissions/operator scopes | Same supported closed scope set and SetOperatorScopes; reconnect; Gateway alone approves upgrades |
| Every local Node grant; all/none | Permissions | PUT /permissions/node grants | Same PermissionGroups and SetNodeGrants, cancellation and republishing |
| Assistant 1 / 2 target (Home Assistant or OpenClaw) | Separate assistant forms | GET/PUT /slots/1 and /slots/2 | Same private profile store; enabling Talk follows native authorization behavior |
| Local wake word / off | Separate assistant wake selector | GET root wake; PUT /wake/1 or /wake/2 | Installed local models; same config writer, voice slots and detector reload |
| Agent picker and display label | Assistant / Load choices | GET /choices/agents?slot=1; PUT slot | Shared agents.list parsing; metadata label is derived server-side |
| Native Live provider/model or Gateway default | Assistant / Load choices | GET /choices/models?slot=1; PUT slot | Shared configured OpenAI + WebRTC + agent-consult + voicesByModel filter, not a frozen model list |
| New context every time vs resume; existing session vs new context | Conditional Resume / Session | GET /choices/sessions?slot=1&agent=...; PUT slot | Only matching-agent session metadata; no transcript request; agent changes reset stale context |
| Device context template, supported placeholders, default reset, expanded preview | Multiline context editor | PUT slot; POST /preview cue=context | Shared parser and device expansion; 1536 UTF-8 bytes after expansion; next activation |
| Greeting prompt and localized default reset | Multiline greeting editor | PUT slot | Shared greeting validator, separate persisted text; next activation |
| Ready / connection-progress / ending tones; default, none, custom tones; preview | Three independent tone pickers | GET root tones; PUT slot; POST /preview cue=ready/progress/end | Shared catalog and PCM rendering; web audition plays locally in the browser, never on the device |
| Ending phrase, contains/start/end position, immediate/end-of-turn evaluation | Ending section | PUT slot ending | Shared EndSettings validation/defaults; canonical exact means contains; next activation |
| Conversation timeout | Assistant timeout | PUT slot timeout | Same private state; 15–900 seconds; default 60; native preset choices offered in web UI |
| Local final-caption history, delete one/all, end current conversation | Local history | GET /history; DELETE /history/{generation or 0} with confirm=true; POST /actions stop_talk | Same bounded volatile local display cache; no remote history and no persisted transcript |
| Other registered related assistant/audio/display preferences | Existing grouped Settings pages | Existing /api/v1/settings and /settings/{domain}/{name} | Existing bound validators, build/hardware registration and persisted config |

Connection, permissions, Talk and the added page are available only on Panel
boards in non-`board_biscuit` builds. Speaker-only builds do not advertise or
mount the page. Agent/session/model selection requires a connected, appropriately
authorized Controller UI; unrelated saved settings remain editable offline.
HA-target slots disable OpenClaw-only controls. Session selection appears only
for resume policy. Clearing a saved session selects a new device context on next
activation. Local wake selection does not configure HA pipelines.

Interactive chat application navigation, sending messages and runtime per-session
model palettes are not device settings; this work does not claim a web clone of
the chat application. Owner configuration does not change Gateway configuration.

## Wire contract and secrets

- GET root returns labels in the device language, defaults, canonical catalog
  values, connection state, editable address/names, `tokenConfigured`, permissions,
  two profiles, local wake metadata, and diagnostic events.
- GET /status returns only current connection states, granted scopes and
  sanitized diagnostic events for polling without overwriting browser drafts.
- PUT /connection accepts `{url, uiName, nodeName, token?}`.
  Omit token (or leave the web password input blank) to retain it; send a nonempty
  value to replace it; send an explicit empty string to clear the shared
  credential. No read or write response includes it. Role private keys, bootstrap
  tokens and device tokens are never returned. Configuration exports do not add
  these separate private stores.
- PUT /slots/{1|2} accepts a complete Slot profile. `agentLabel` and
  `contextLabel` are derived from shared catalog metadata and are not trusted
  as presentation content from the request. Changes to agent/model/session are
  checked against the live picker catalogs; unchanged selections remain valid
  for unrelated offline edits. A session must belong to its selected agent.
- PUT /permissions/operator accepts `{scopes: [...]}`;
  PUT /permissions/node accepts `{grants: {group: boolean}}`.
  An empty operator scope list revokes the local request, not a previously stored
  Gateway-side authorization. Check current granted scopes after reconnect.
- POST /actions accepts `{action, role?: both|ui|node, confirm?: boolean}`.
  Pair-again/forget require confirmation and apply to both roles; a single-role
  destructive request is rejected. Role selection applies to connect, disconnect
  and clearing diagnostic events.
- POST /preview accepts `{cue, choice?}` for tone audition, or
  `{cue: "context", template}` for a private owner-only expansion preview.
- PUT /wake/{1|2} accepts `{value: localModelId}` (empty means off).
- Unknown fields, oversized/trailing JSON and invalid slots are rejected.
  Validation errors use 422, invalid bodies 400, unavailable catalogs 503, busy
  unavailable selection 503, and unauthenticated requests 401. Errors never echo request values
  or raw Gateway/provider/storage payloads. All responses are no-store.
- Refresh reloads the same source of truth; no separate web settings file exists.
  Profile prompt validation runs before Talk authorization changes or persistence.
- The existing admin login/session/Basic-auth model and TLS behavior are unchanged.
  Restricted Node `echolocal.settings.*` remains governed by its reviewed
  allowlist/grants and cannot access these endpoints, credentials or permissions.

## Maintained coverage

[openclaw-owner-coverage.json](openclaw-owner-coverage.json) enumerates every
separately persisted field and supported lifecycle/configuration operation.
`TestOwnerCoverageManifest` fails when a Slot, EndSettings or
ConnectionSettings field is added without a surface/dependency decision.
The web contract tests cover all page groups; the browser regression runs the
actual built frontend against explicitly synthetic API responses. Backend HTTP
tests exercise the real shared owner authentication and actual persistence.

Run the app tests with the matching board tag, and the library tests with:

```sh
go test -tags board_athena ./internal/feature/openclawconfig ./internal/feature/settings ./internal/feature/clawtalk ./internal/feature/frontend ./internal/feature/control ./internal/openclaw/...
npm --prefix web ci
npm --prefix web run build
npm --prefix web test
npx --prefix web playwright install chromium
npm --prefix web run test:e2e
```

Synthetic browser tests, live device API tests, native rendering and physical
owner acceptance are distinct; only report checks actually performed.

## Temporary experimental exception

Avatar Lab may remain device/CLI-only while experimental. Its runtime renderer
and import support do not broaden this exception. Before promoting its settings
to a supported feature, reassess and implement owner web/API parity, shared
validation/persistence and tests. No other settings gain an exception by analogy.

### Browser tone audition

`POST /openclaw/preview` with a validated `ready`, `progress`, or `end` cue
and catalog `choice` returns `{ "audio": "<base64 PCM16 WAV>" }` (HTTP 200).
The web interface plays this audio in its local browser with native controls;
there is no device speaker output, microphone capture or Talk interruption.
The empty choice uses the same cue defaults as the native UI; `none` returns
short silence. Uploaded tones use the same stored clip and level as device
feedback. Prompt-context previews retain their existing JSON contract.
