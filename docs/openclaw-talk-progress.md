# Continuous OpenClaw Talk

Implementation for https://github.com/rainu/echolocal/issues/9. Application PR: https://github.com/rainu/echolocal/pull/10; library dependency PR: https://github.com/rainu/libcountertop/pull/5. Neither is to be merged before owner acceptance of the concrete tested revisions.

## Routing and settings

Each existing assistant slot retains Home Assistant by default. OpenClaw slots also have a voice-model picker populated from the current Gateway native Live catalogue, with Gateway default as the unchanged default. A concrete choice stores provider/model per slot and passes the same selection to creation and capability lookup. New Gateway-advertised models do not require a client model-name allowlist. Native event variants are observed by event type rather than a fixed list of model names; unsupported tool-call protocols still fail clearly instead of silently switching models. The slot submenu adds the conversation target. OpenClaw selection exposes agent, resume/new-context policy, context picker and inactivity timeout; HA selection retains the original pipeline row. OpenClaw can select an already installed wake-word model without depending on HA actions. Connection settings and credentials remain exclusively in the existing OpenClaw connection menu.

Agent and context identifiers remain in a separate private backend store (mode 0600). Menus use human names or localized numbered labels, never raw IDs. Agent selection clears the old context. Resume creates/reuses a dedicated context or continues the explicitly selected existing context. Disabling resume explicitly creates a new context each activation. Corrupt or mismatched routing fails closed, never silently selects HA or another agent.

The satellite routes before starting HA. Queued and arbitrating HA activations are visible to the alternate route, preventing overlap during the handoff. Repeated activations while Talk owns the slot are consumed without another session. Action/stop-word controls can end Talk.

## Backend, transport and audio

Talk opt-in requests exactly operator.read and operator.talk; Gateway authorization is separate. The existing paired identity is reused. Each activation has a fresh voice-session ID and a binding to one authenticated socket. Disconnects/endpoint changes cannot transfer RPCs, offer authorization or session cleanup to a replacement connection. Uncertain requests are not replayed.

Startup first creates an authorized native session, which initializes a new conversation context, then checks that context's ready realtime/WebRTC/agent-consult catalogue. Only explicitly retryable catalogue-warming reads are retried; creation is never replayed. The bounded SDP offer uses the paired same-origin endpoint without redirects or proxies.

The client follows the deployed Control UI's native Live media contract: mode realtime, transport webrtc, brain agent-consult, and voice-transcript/voice-selection capabilities. The RPC term realtime also names native Live Talk; it is not a request for an unrelated STT/chat/TTS pipeline. The Gateway builds initial instructions from the selected agent's identity/profile files and shared conversation context. The client does not replace those instructions or force every utterance through another agent roundtrip.

The bounded oai-events channel observes native transcript, lifecycle and delegation events. The Gateway sideband still executes actual agent consultation. Direct Live answers, acknowledgements during work and natural follow-ups are played and persisted regardless of whether a new delegation occurred. There is no client-side semantic output gate, suppressed-answer classification or mandatory-delegation watchdog. Unsupported protocols and real transport/provider failures are reported normally.

Connected peer, provider and control-channel readiness precede microphone acquisition and the ready tone; there is no additional policy-append acknowledgement roundtrip. Before readiness only synthetic silence advances the timeline; the room microphone is not acquired. Existing Echo microphone capture/AEC is reused: 16 kHz mono, 320-sample/20-ms frames, pure-Go wideband Opus, 48-kHz RTP clock. Received Opus uses the existing 48-kHz stereo speaker path. A bounded 120-ms initial jitter reserve feeds a 100-ms speaker lane, capped at 300 ms of application reserve. Short tails drain. Provider-confirmed interruptions clear stale queued audio once, but never block subsequent Live audio. Input queues remain bounded; one speaker claim covers the activation.

The visible page provides Connecting, Listening, Thinking, Speaking, Ended, Error and Muted states and a large end button. Configured inactivity applies while listening, not during in-progress agent work or actual playback. Older playback completion cannot overwrite newer response activity. Exact localized end phrases terminate the session; physical mute is checked on every captured frame and via the existing mute-change hook. Stop, failure, disconnect and shutdown close capture, peer, logical session and speaker resources idempotently without reopening the microphone.

The provider's native interruption/cancellation path is retained; user transcription and cancelled-turn events clear queued output. Effective echo rejection and audible barge-in still require the real hardware/acoustic acceptance test, not just transport or codec tests.

## Privacy and diagnostics

No local recording or content-level diagnostic mode is added. Audio, transcripts, tool arguments/results, credentials, endpoints, SDP and ICE remain out of normal diagnostics. Final captions are used transiently for end-phrase matching and forwarded in order, with bounded queues and stable deduplication IDs, to the chosen Gateway conversation history. They are not recorded locally. Native legacy client media requires this explicit caption forwarding; the public Live path retains Gateway-normalized transcript handling.

First-response timing measures final user-caption receipt to first decoded audible PCM, not exact acoustic onset or VAD duration.

Developer control commands expose bounded state/stage/timing and aggregate capture, encode/RTP, decode/playout, loss/drop, tool and utterance counters. They never return target identifiers or payloads. Event overflow ends the activation rather than silently dropping lifecycle events.

All new menu text is explicitly translated in every installed catalogue (currently English, German, Italian and Vietnamese). Catalogue enumeration and placeholder checks are automated.

## Verification and remaining acceptance

Synthetic tests cover unchanged HA defaults, exclusive routing, alternate slot UI, localization/identifier redaction, context policy/restart/storage permissions, scoped RPC/correlation/cancellation/connection fences, event overflow, peer/provider/audio readiness, idempotent teardown, end phrases, response-aware inactivity and audio-only negotiation, unmodified Control-UI startup behavior, direct follow-ups without consultation, audible acknowledgements during consultation, ordered/deduplicated caption draining, startup silence, jitter prefill/bounds and short-tail draining. Device builds and real hardware results are reported separately in the issue.

A newer microphone-free synthetic-speech probe verified one native agent delegation and the expected selected-agent identity in the returned caption. It sent a locally synthesized neutral question and did not acquire the room microphone or play into the room. It does not establish audible quality or acoustic AEC/barge-in.

A prior separate microphone-free probe verified real scoped handshake/catalogue, Gateway-controlled session creation, SDP offer/answer, peer connection and explicit closure. It did not verify speech or consultation. Its standalone Android/CGO build and linker workaround are NOT production requirements: the normal application uses Linux/ARM and the existing DHCP-backed resolver.

Before acceptance: install the concrete tested build with a verified binary rollback, inspect the real slot menus/layouts in all supported languages, verify transport/audio counters on the Echo, then perform actual spoken consultation, two follow-ups/context reactivation, interruption/AEC, explicit stop/end phrase, physical mute, disconnect/error and shutdown tests. No merge or issue closure until rainu accepts that concrete implementation.


## Control UI parity correction

The earlier agent-first workaround forced consultation on every request and withheld otherwise valid Live responses. It has been removed, including its output block/hold machinery. This is not a new conversation mode: the provider now controls its native Live conversation as in the Control UI. Numeric delegation counters are observation only, never permission to speak.

Installed-source comparison: src/gateway/talk/handlers/client-create.ts creates the session with selected-agent profile instructions and initial history; extensions/openai/realtime-voice-provider-factory.ts composes native Live instructions; the Control UI uses voice-transcript/voice-selection and opens oai-events without injecting an additional routing policy. It attaches received audio directly.

A microphone-free synthetic UI-contract probe recognized three questions and produced three final replies without forced consultations. It did not confirm the selected identity in that isolated run. This remaining initial-context/model discrepancy must be distinguished from the now-removed output suppression; do not claim identity acceptance from transport or unit tests.

The model picker, default behavior, future synthetic catalogue entry and per-slot persistence have regression coverage. No additional contextual cue, startup persona override or one-off forced consultation from exploratory probes is shipped. Initial identity in the raw isolated UI-contract probe remains unconfirmed; the deployed physical test must verify it without semantic output suppression.

## End controls, feedback and activation ownership

End-command matching accepts a final explicit command clause with ordinary polite lead-ins, instead of requiring the entire final caption to equal the command. Common negations and explanatory prefixes are not stripped as courtesy. A separately installed local stop-word model is optional; the transcript end command does not require one. The end button remains available even when speech recognition fails.

Each OpenClaw slot has separate start/end sound choices, including Default, Off, existing built-ins and installed custom tones. Default start retains the previous readiness sound; default end is its descending pair. Readiness, not wake detection, triggers the start cue. End feedback is emitted once after capture/playout close for a session that reached readiness, before potentially slow network cleanup. Failed startup does not emit a successful start/end pair. HA-only activation-tone and delivery settings are hidden only for slots whose alternate conversation explicitly owns feedback; HA defaults remain unchanged.

Speaker-preemption and provider-end callbacks retain the cancellation function of their own activation, never a global stop targeting a possible successor. Terminal and activity updates are generation-fenced. User cancellation during connecting is reported as ended rather than a random provider failure. This closes a demonstrable stale-callback race; it does not prove that race caused the previously reported silent attempt.

Diagnostics retain at most six completed activation reports in device memory, plus current counters. Reports contain lifecycle stage, safe failure class, timing and aggregate media counters, never voice/target IDs, captions, audio or signaling. The ring is not persisted across restart. Queued-output clearing is counted separately to distinguish provider-confirmed interruption from packet loss or hardware underrun. No requests are replayed and no extra speech suppression, model override or automatic reconnection loop is added.


## Caption display and punctuation regression

The active Talk page now displays recognized user and assistant turns, newest first, with wrapped text and explicit role labels. The screen uses provider-final captions; it does not promise word-by-word transcription or sample-accurate synchronization with speech. A separate capture indicator requires a fresh microphone frame within 500ms and current capture ownership. Transport readiness alone cannot light it. Capture activity is not proof of correct speech recognition or acoustic echo rejection.

A history entry in the assistant submenu and the active page opens this device's last six activations. The volatile display cache retains at most 32 captions per activation and 4096 Unicode code points per caption. It is not written to disk or included in diagnostics; restarting the service clears it. It does not retrieve or display older Gateway contexts, tool results or other devices' conversations. Late callbacks and duplicate turn IDs cannot enter the current activation.

ASR may place a courtesy suffix in a separate punctuation-delimited clause. End-command recognition now ignores only complete trailing courtesy clauses before checking the explicit command; negated/explanatory utterances remain excluded. End feedback remains tied to actual local teardown, not the model saying farewell.

The playout reserve starts at 120ms on a healthy connection. Actual packet-arrival stalls can raise the next rebuffer target to at most 400ms, with a bounded 600ms software queue. Learning does not pause an already playing queue, does not gate semantic Live responses, and does not add a provider round trip. This can mitigate repeated jitter after a stall; it cannot eliminate the first unexpected stall or a slow model/agent response. Audible quality still requires hardware acceptance.

The live display refreshes only on changed captions, lifecycle or capture freshness, rather than rebuilding a long conversation ten times per second. The developer `talk show` command opens the current/last page without starting capture, allowing a real-device display inspection without room audio.


## Centralized settings and local-history deletion

The assistant submenu now contains only the speech-target selector for the OpenClaw integration; shared wake-word and device options remain unchanged. The OpenClaw root contains Connection (the existing connection editor), Assistant 1/2 conversation settings, and a shared local-history browser. Agent, catalogue model, resume/context, start/end tones and inactivity timeout remain per slot, but are edited centrally. Inactive slots explain where to choose their speech target.

The local caption browser supports deleting a single activation or clearing all local display histories, each with a confirmation that explicitly excludes Gateway conversation deletion. It neither deletes a remote context nor changes resume/context selection. Deleting an active cache hides subsequent captions for that generation until a fresh activation. Detail pages look up the generation on redraw instead of retaining a stale caption snapshot; the live page cannot fall back to an older conversation after deletion. Core tests cover erase, non-resurrection and future activations.
