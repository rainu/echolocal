# Interrupted Talk recovery

Unexpected WebRTC disconnection/failure/closure, provider data-channel closure,
remote transport closure and lost control streams are distinct from a local
hang-up or end phrase. A logical Talk retains its microphone/speaker reservation
while its old voice transport is closed and a fresh one is opened.

Recovery uses at most three replacement attempts, with 1/2/4-second backoff.
Each setup also has the existing 45-second deadline. The budget is shared across
the entire logical activation, not reset by a briefly successful reconnect.
Initial activation failures are not automatically replayed.

The exact active session key and snapshotted Talk configuration are retained,
including a session chosen by transfer, model/provider, end phrase, device
context and input mute. No persistent default slot is rewritten. The initial
greeting is not repeated and queued audio/partial captions from the retired
transport are discarded. Already delivered speech is not replayed; audio spoken
during an outage is not buffered or resent. The provider may receive fresh
context/history for its replacement transport, not an invented user turn.
Recovery never resets/deletes the underlying text session or aborts its tasks.

Explicit stop, inactivity expiry, speaker preemption and the end phrase remain
final. Cancellation interrupts backoff and setup. Per-attempt callbacks are
fenced from replacement transports. Invalid contracts, rejected authorization,
unknown provider errors and session replacement are terminal. The Gateway's
specific message-only error "OpenClaw could not confirm that input. It was not
sent again; check the conversation before retrying." is a narrow exception:
EchoLocal retires that voice transport and opens a fresh one for the same target.
It does not resend the uncertain input, repeat the consult/tool call, or invent
a successful result. The last request may therefore remain unanswered; inspect
the text conversation before manually repeating it. Only the exact known error
message is recognized until the Gateway supplies a structured error code; unknown
or changed wording fails closed rather than being guessed recoverable.
The RPC decoder derives only the bounded error class before discarding the
provider payload. Raw provider messages, tokens and tool arguments still never
reach Talk subscribers, UI or persisted diagnostics. A reason claimed by the
wire itself is ignored. Regression coverage includes the raw-event privacy
boundary, not just already-decoded synthetic callbacks. Recovery stays on the
original authenticated operator binding: an edited/replaced control connection
is not silently adopted. If that binding is lost, recovery terminates safely
rather than connecting a different endpoint or identity.

Local status and the existing Node Talk status include a safe recovery object:
state (connected/reconnecting/stopped/failed), replacement attempts and a bounded
error reason. The UI uses its existing connecting state while recovering and
error on exhaustion. No end/start chime or greeting is replayed between attempts.
Normal final hang-up retains its end cue.

Native GPT-Live errors are classified at the control-channel receive path, not
all treated as rejected control requests. Known server/internal failures,
temporary rate limits and voice-session expiry may replace voice within the
same lifetime retry budget. Authentication/token failures, quota/policy errors,
invalid requests, contradictory and unknown classes remain terminal. Recovery
never refreshes credentials or resends an input/consult.

The Gateway can concurrently publish a redacted generic session.error for the
same native event. When native media is active, RPC error verdicts wait at
most one second for the structured native classification, regardless of which
channel arrives first. A following generic close cannot turn an authentication
error into a transport retry. Cancellation interrupts the wait immediately; a
missing native classification keeps the bounded RPC verdict; unknown errors remain terminal.

Recovery status preserves lastFailure and allowlisted providerErrorCode/
providerErrorType across successful reconnects. Diagnostic tokens not on the
allowlist become other (or empty when absent); provider messages, arbitrary
codes, request contents and credentials are never retained. This distinguishes
the last failure from the replacement transport's fresh counters.


For a controlled device test, local diagnostic command
echod ctl talk test-disconnect closes only the active WebRTC link without
issuing a logical stop. It is hidden from ordinary help, requires local daemon
control access, takes no target and is not exposed as a Node operation.

Tests cover real local WebRTC closure, exact-target retention, finite repeated
failure, cancellation in backoff/setup, permanent failures, end phrase and stale
callbacks. These are not proof of physical room speech reaching the original
session after recovery: verify that on the Echo, including after handoff.

The hidden local diagnostic echod ctl talk test-provider-error enqueues one
fixed synthetic native server_error into the active Talk, without a user input,
target override or arbitrary payload. It is not a Node operation. Use it to
verify native-error -> manager -> actual provider reconnection on the running
device, including after handoff. It is fault injection, not evidence that a
remote provider naturally emitted that specific error.
