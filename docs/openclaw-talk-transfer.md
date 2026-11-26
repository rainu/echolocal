# Device-local Talk transfer

The dedicated Node command is `echolocal.talk.transfer` (published tool `echolocal_talk_transfer`), with one required `sessionId`: the exact key of an existing accessible session. Resolve spoken names with the existing session catalogue and clarify ambiguous matches before invoking it.

## Semantics

This is a device command: it operates on the **currently active Talk on this Echo**, regardless of the invoking text session or voice activation. A delayed call is a new command against the then-current Talk. It does not authenticate or infer the origin of an invocation. This scope intentionally replaces the original origin-binding requirement in issue #73.

1. Reserve the local transition and validate the target using the current Talk Control connection. Invalid/inaccessible, unchanged and busy targets leave the current voice connection untouched.
2. Return an acceptance receipt. Only a positive Gateway acknowledgment of the Node result permits the deferred transition. Failed, cancelled or uncertain delivery does not close the current Talk.
3. Wait for the current consultation and audio to settle, then request a short spoken confirmation through the existing provider data channel. Require new audible PCM, a completed assistant response and drained speaker audio. Missing/rejected confirmation times out without ending the current Talk.
4. Close the old voice activation, retaining the local reservation throughout, then create a new voice activation for the selected existing session. This is a close/create handoff with a reconnect gap, not an atomic protocol switch.
5. If target setup fails, try the previous session on the same connection and announce recovery there. If recovery also fails, leave Talk stopped/error. Never claim the target opened when only recovery succeeded.

`echolocal.talk.status` reports `transfer` and `transferReason`. States include validating, accepted, confirming, opening, recovering, completed, recovered, failed and cancelled. The initial tool receipt means **accepted**, not completed. Finish the originating consultation promptly: do not poll/wait for completion inside it, since confirmation waits for consultation completion. Check status later or from another session for the final outcome.

Stop cancels a pending transfer, including the close/create gap. Repeated requests are rejected while reserved. Public wake/UI/HA/Node starts share the same exclusion. Local generations fence work after acceptance, so callbacks from an accepted transfer cannot retire a replacement activation.

The command uses the existing local `talk` permission group; it neither widens Gateway privileges nor changes the existing public Node stop ownership rule. Node disconnect and grant cleanup cancel pending transitions. No text-session reset/delete, task abort or persistent assistant-default update is issued.

## Verification boundaries

Unit/wire tests cover acknowledgment fencing, cancellation, local exclusion, confirmation audio/turn/drain requirements, correct target selection and target-failure recovery. A real device must additionally verify provider announcement/reconnect behavior. Spoken request, room audibility and subsequent speech arriving in the new session require an actual voice interaction; unit tests and an installed build alone do not establish those properties.
