# Device-local Talk transfer

## Invocation surfaces

The native Home Assistant action is `esphome.YOUR_DEVICE_openclaw_talk_transfer`,
with required string `session_id`. Use `response_variable`: this operation
requires a correlated answering request, not fire-and-forget execution.

The owner-authenticated HTTPS API exposes the same action through the established
`POST /api/v1/actions/openclaw_talk_transfer` endpoint:

```json
{"args":{"session_id":"agent:example:existing-session"}}
```

Use the normal owner session cookie, login bearer token or owner Basic
authentication. An accepted HTTP 200 response wraps the receipt in `answer`,
for example `{"answer":{"accepted":true,"state":"accepted","message":"..."}}`.
Errors use the existing action endpoint's HTTP 422 error response; unauthorized
requests receive 401 before action execution or discovery. No new public route
or OpenClaw permission is introduced.

Discover targets with `openclaw_sessions` through HA or
`POST /api/v1/actions/openclaw_sessions` with `{"args":{}}`.
Read final state with `openclaw_talk_status` through either action surface.
Both expose the same privacy-bounded state projection as Node Talk status,
without prompts, history, target identifiers or connection diagnostics.

The supported HA route is the built-in ESPHome integration, including when the
EchoLocal HACS dashboard is installed. HACS does not currently supply Talk
services; its separately deferred convenience-wrapper PR is not required and
is not changed here. Use the native actions rather than an undocumented wrapper.

The dedicated Node command is `echolocal.talk.transfer` (published tool `echolocal_talk_transfer`), with one required `sessionId`: the exact key of an existing accessible session. Resolve spoken names with the existing session catalogue and clarify ambiguous matches before invoking it.

## Semantics

This is a device command: it operates on the **currently active Talk on this Echo**, regardless of the invoking text session or voice activation. A delayed call is a new command against the then-current Talk. It does not authenticate or infer the origin of an invocation. This scope intentionally replaces the original origin-binding requirement in issue #73.

1. Reserve the local transition and validate the target using the current Talk Control connection. Invalid/inaccessible, unchanged and busy targets leave the current voice connection untouched.
2. Return an acceptance receipt. Node retains its positive Gateway acknowledgment
   fence. Native HA requires a correlated response requesting its answer and a
   successful response write; HTTP requires a successful response write/flush.
   These transports do not provide an application-level peer acknowledgment:
   successful writing is not proof that the caller consumed the receipt.
   Failed writes, missing response correlation, cancellation and uncertain
   delivery before that boundary do not close the current Talk. Do not blindly
   retry a timed-out call: inspect status first.
3. Wait for the current consultation and audio to settle, then request a short spoken confirmation through the existing provider data channel. Require new audible PCM, a completed assistant response and drained speaker audio. Missing/rejected confirmation times out without ending the current Talk.
4. Close the old voice activation, retaining the local reservation throughout, then create a new voice activation for the selected existing session. This is a close/create handoff with a reconnect gap, not an atomic protocol switch.
5. If target setup fails, try the previous session on the same connection and announce recovery there. If recovery also fails, leave Talk stopped/error. Never claim the target opened when only recovery succeeded.

`echolocal.talk.status` reports `transfer` and `transferReason`. States include validating, accepted, confirming, opening, recovering, completed, recovered, failed and cancelled. The initial tool receipt means **accepted**, not completed. Finish the originating consultation promptly: do not poll/wait for completion inside it, since confirmation waits for consultation completion. Check status later or from another session for the final outcome.

Stop cancels a pending transfer, including the close/create gap. Repeated requests are rejected while reserved. Public wake/UI/HA/Node starts share the same exclusion. Local generations fence work after acceptance, so callbacks from an accepted transfer cannot retire a replacement activation.

The command uses the existing local `talk` permission group; it neither widens Gateway privileges nor changes the existing public Node stop ownership rule. Node disconnect and grant cleanup cancel pending transitions. No text-session reset/delete, task abort or persistent assistant-default update is issued.

## Verification boundaries

Unit/wire tests cover acknowledgment fencing, cancellation, local exclusion, confirmation audio/turn/drain requirements, correct target selection and target-failure recovery. A real device must additionally verify provider announcement/reconnect behavior. Spoken request, room audibility and subsequent speech arriving in the new session require an actual voice interaction; unit tests and an installed build alone do not establish those properties.

## Automation example

```yaml
- action: esphome.YOUR_DEVICE_openclaw_talk_transfer
  data:
    session_id: "agent:example:existing-session"
  response_variable: handoff
# handoff.accepted means accepted, NOT completed.
# Inspect separately after the originating consultation has finished:
- action: esphome.YOUR_DEVICE_openclaw_talk_status
  response_variable: talk_state
```

`completed` means the target transport/audio opened; `recovered` means the
previous session was restored instead. `failed` and `cancelled` are not
successful handoffs. Busy, invalid/inaccessible, absent and unchanged targets
return actionable errors without ending the existing voice activation.

### Spoken confirmation

EchoLocal supplies the audible handoff announcement. The Node tool description
explicitly tells the agent not to speak an additional confirmation before the
call or repeat it after acceptance; rejected requests should explain their actual
error. This targeted tool guidance is not a hard guarantee of model behavior.
A real spoken test must verify one confirmation and subsequent speech routing;
synthetic tests cannot establish either acoustic property.
