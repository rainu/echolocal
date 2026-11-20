# Audible Talk setup feedback

Refs #26.

Every accepted OpenClaw activation owns a single feedback sequence:
a 45 ms high acknowledgment is queued before UI/network setup; after 900 ms,
a repeating low double ring indicates pending setup. The connection tone is
selectable per assistant (directly after Start tone) using the same built-in/custom catalogue, preview and
explicit Use action as ready/end tones; None disables it. Custom clips drain
before the repeat delay starts, so long clips cannot accumulate in a backlog. Existing configured ready
and end tones are retained. The default ready cue rises; setup failure uses a
longer descending three-note cue. An explicitly configured silent ready tone
remains silent.

Progress uses cancellable feedback-lane PCM appended rather than mixed. Success
joins the loop, removes queued progress, waits for already submitted hardware
audio, and plays readiness only after peer/provider/audio readiness. It never
waits for a complete ring cycle. Fast setup preserves the acknowledgment.
Failure and setup timeout stop progress before the failure cue; intentional
cancellation, dismissal, mute and preemption stop progress without readiness.
Existing activation ownership consumes duplicate wakes without a second setup.

Microphone frames are discarded until readiness playback and a 250 ms acoustic
tail have completed. The native transport sends silence while its media gate is
closed. Wake detection is paused while feedback is being rendered and during
the same tail. Provider playout also waits for the ready cue. The existing
45-second setup deadline now covers microphone startup and readiness feedback.
After successful readiness playback the native provider receives one explicit
greeting request using the same speakable wire
contract as OpenClaw's triggerGreeting. A per-assistant multiline Greeting prompt
editor controls the complete request text, with explicit Save and Restore default.
Custom text is preserved verbatim across language changes and is limited to 1536
UTF-8 bytes; device placeholders are not expanded. An empty stored value follows
the localized default. Opening/editing/resetting the draft neither starts Talk nor
persists without Save. The default asks for a brief greeting and then
silence, never continuation of a historical turn. It sends neither a synthetic
user message nor replacement session instructions. The existing configurable
device prompt stays separate and silent, sent once on first user speech.
No greeting is sent on setup failure/cancellation. Actual words and response
latency remain provider-generated, not a prerecorded or deterministic cue.
No credentials, network settings or assistant targets are changed.

## Automated verification

- Feedback lifecycle: immediate acknowledgment, repeated pending progress,
  fast readiness without ring-cycle delay, failure/cancellation cleanup,
  cancellation without readiness, distinguishable default cues.
- Duplicate pending wake requests do not acquire a second activation.
- Synthetic WebRTC peer with withheld provider readiness: no microphone or
  ready cue until readiness; cancelling while withheld emits neither.
- Existing Talk transport, lifecycle, caption, ending and routing regressions.
- Athena-tagged tests and vet; Athena release build and on-device verification
  are recorded against the exact implementation commit in the issue/PR.

Automated PCM/ordering checks do not replace the owner's subjective assessment
of audibility, distinctness and timing in the actual room. Hardware verification
and any remaining limits are reported separately, not inferred from unit tests.
