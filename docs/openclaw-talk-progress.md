# Continuous OpenClaw Talk

Implementation for https://github.com/rainu/echolocal/issues/9. Application PR: https://github.com/rainu/echolocal/pull/10; library dependency PR: https://github.com/rainu/libcountertop/pull/5. Neither is to be merged before owner acceptance of the concrete tested revisions.

## Routing and settings

Each existing assistant slot retains Home Assistant by default. The slot submenu adds the conversation target. OpenClaw selection exposes agent, resume/new-context policy, context picker and inactivity timeout; HA selection retains the original pipeline row. OpenClaw can select an already installed wake-word model without depending on HA actions. Connection settings and credentials remain exclusively in the existing OpenClaw connection menu.

Agent and context identifiers remain in a separate private backend store (mode 0600). Menus use human names or localized numbered labels, never raw IDs. Agent selection clears the old context. Resume creates/reuses a dedicated context or continues the explicitly selected existing context. Disabling resume explicitly creates a new context each activation. Corrupt or mismatched routing fails closed, never silently selects HA or another agent.

The satellite routes before starting HA. Queued and arbitrating HA activations are visible to the alternate route, preventing overlap during the handoff. Repeated activations while Talk owns the slot are consumed without another session. Action/stop-word controls can end Talk.

## Backend, transport and audio

Talk opt-in requests exactly operator.read and operator.talk; Gateway authorization is separate. The existing paired identity is reused. Each activation has a fresh voice-session ID and a binding to one authenticated socket. Disconnects/endpoint changes cannot transfer RPCs, offer authorization or session cleanup to a replacement connection. Uncertain requests are not replayed.

Startup checks the selected context's ready realtime/WebRTC/agent-consult catalogue, negotiates gateway-control-v1, posts a bounded SDP offer to the paired same-origin endpoint without redirects/proxies, and waits for both connected peer and provider readiness. The Gateway owns the native agent-consult/control roundtrip for the selected context. This is the supported Gateway-owned transport, not a claim of client-data-channel/WebUI parity.

The microphone is acquired only after transport/provider readiness. Actual microphone frames, speaker readiness and enabled existing AEC are checked before the ready tone. Existing Echo microphone capture/AEC is reused: 16 kHz mono, 320-sample/20-ms frames, pure-Go wideband Opus encoding, 48-kHz RTP clock. Received Opus is decoded to the existing 48-kHz stereo speaker path. Input queues are bounded; decoded output is capped at half a second with aggregate displacement counters. One speaker claim covers the activation and preemption ends Talk.

The visible page provides Connecting, Listening, Thinking, Speaking, Ended, Error and Muted states and a large end button. Configured inactivity applies while listening, not during in-progress agent work or actual playback. Older playback completion cannot overwrite newer response activity. Exact localized end phrases terminate the session; physical mute is checked on every captured frame and via the existing mute-change hook. Stop, failure, disconnect and shutdown close capture, peer, logical session and speaker resources idempotently without reopening the microphone.

The provider's native interruption/cancellation path is retained; user transcription and cancelled-turn events clear queued output. Effective echo rejection and audible barge-in still require the real hardware/acoustic acceptance test, not just transport or codec tests.

## Privacy and diagnostics

No local recording or content-level diagnostic mode is added. Audio, transcripts, tool arguments/results, credentials, endpoints, SDP and ICE remain out of normal diagnostics. Final user transcripts are consumed transiently only for end-phrase matching; Gateway-native history remains part of the chosen conversation by design.

Developer control commands expose bounded state/stage/timing and aggregate capture, encode/RTP, decode/playout, loss/drop, tool and utterance counters. They never return target identifiers or payloads. Event overflow ends the activation rather than silently dropping lifecycle events.

All new menu text is explicitly translated in every installed catalogue (currently English, German, Italian and Vietnamese). Catalogue enumeration and placeholder checks are automated.

## Verification and remaining acceptance

Synthetic tests cover unchanged HA defaults, exclusive routing, alternate slot UI, localization/identifier redaction, context policy/restart/storage permissions, scoped RPC/correlation/cancellation/connection fences, event overflow, peer/provider/audio readiness, idempotent teardown, end phrases, response-aware inactivity and audio-only negotiation. Device builds and real hardware results are reported separately in the issue.

A prior separate microphone-free probe verified real scoped handshake/catalogue, Gateway-controlled session creation, SDP offer/answer, peer connection and explicit closure. It did not verify speech or consultation. Its standalone Android/CGO build and linker workaround are NOT production requirements: the normal application uses Linux/ARM and the existing DHCP-backed resolver.

Before acceptance: install the concrete tested build with a verified binary rollback, inspect the real slot menus/layouts in all supported languages, verify transport/audio counters on the Echo, then perform actual spoken consultation, two follow-ups/context reactivation, interruption/AEC, explicit stop/end phrase, physical mute, disconnect/error and shutdown tests. No merge or issue closure until rainu accepts that concrete implementation.
