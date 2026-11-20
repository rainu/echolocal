# OpenClaw Node command contracts

For typed tool/skill publication, schemas, examples and troubleshooting, see
[Published tools and guidance](openclaw-node-tools.md). This inventory alone is
not the agent-visible tool catalog.

This change builds on the existing device adapters. It does not add live desktop
streaming, issue creation, unrestricted shell execution, or arbitrary settings.

## Namespaces

Commands with reviewed existing OpenClaw contracts retain their names:
`device.info`, `device.status`, `device.health`, `device.permissions`, `system.notify`, `screen.snapshot`, `screen.record`, and the existing `camera.snap` adapter.

Every other registered device adapter is advertised and dispatched only under
`echolocal.*`. The old unprefixed private names are deliberately not aliases.
The static local administration commands remain local-only. This is a wire-name
change, so previously approved pairing command sets and Gateway allowlists must
be updated through the authorized Gateway flow before remote use.

## New typed commands

All input is a JSON object. Unknown fields and missing required values fail.
Percentages use 0–100 inclusive. Mutating commands use explicit target states.

| Command | Input | Local permission |
|---|---|---|
| device.status | {} | state |
| device.health | {} | state |
| device.permissions | {} | state |
| system.notify | title?, body, delivery?: auto/overlay, priority? | display |
| screen.snapshot | format?: jpeg/png, maxWidth?, quality?: 0–1, screenIndex?: 0, executionId? | screen |
| screen.record | durationMs?: 3000 (maximum 10000), fps?: 15, format?: mp4, includeAudio?: false, screenIndex?: 0 | screen |
| echolocal.audio.status | {} | audio |
| echolocal.audio.volume.set | channel?: main, volume | audio |
| echolocal.audio.mute.set | channel?: main, muted: boolean | audio |
| echolocal.display.status | {} | display |
| echolocal.display.brightness.set | brightness | display |
| echolocal.display.power.set | on: boolean | display |
| echolocal.settings.get | domain, name | settings and field group |
| echolocal.settings.set | domain, name, value: string | settings and field group |
| echolocal.alarm.list | {} | alarms |
| echolocal.alarm.create | time: HH:MM, name?, days?: weekday list, enabled?, sound?, snoozeMinutes? | alarms |
| echolocal.alarm.update | id and any of the create fields | alarms |
| echolocal.alarm.delete | id | alarms |
| echolocal.timer.list | {} | alarms |
| echolocal.timer.create | seconds: 1–86400, name? | alarms |
| echolocal.timer.cancel | id | alarms |
| echolocal.talk.status | {} | talk |
| echolocal.talk.start | slot: 1 or 2 | talk |
| echolocal.talk.stop | {} | talk |

Audio channels: main, media, voice, alerts, feedback. Speaker mute is not
microphone mute. Existing granular player/entity adapters remain available
under the private prefix.

Display brightness explicitly selects manual mode. Display power is a
backlight-off/wake operation, not Android suspend or device power-off; repeated
off preserves the saved brightness/mode for the matching on. Brightness/mode are
restored on wake in the same process. Status reports configured brightness and
automatic/manual mode, not a measured hardware sleep state.

Timers are independent Node-created timers with generated IDs, not the single
local CLI timer. The list contains Node-created timers in this process only;
it does not claim to enumerate timers owned by Home Assistant. They share the
existing timer presentation/ringing service and survive socket disconnects,
but not an application restart. A cancel cannot accidentally remove a
non-Node timer. Alarm updates preserve unspecified fields.

Talk uses a separate local grant (the existing default-on policy applies).
Only preconfigured OpenClaw slots are callable; no target/agent/credential
configuration can be changed. A generation-scoped owner prevents stop or
revocation from ending a newer locally started conversation. Disconnect or
grant changes cancel the Node-owned activation. Local microphone mute still
applies. Status omits conversation text, endpoints, signaling, and credentials.

## Capture limits

Snapshots return actual PNG/JPEG bytes and dimensions, with aspect-preserving
downscaling and no upscaling. The longer dimension is bounded by maxWidth.
Video reuses the Android H.264 helper and muxes its access units into fragmented
MP4, rather than returning raw H.264 with an incorrect file extension.
At most one recording runs at once, with bounded memory and cancellation-driven
stream closure. The helper's current native frame rate is 15 fps; other requested
rates explicitly fail (use --fps 15 with the CLI). Audio capture is unsupported
(use --no-audio). A camera recording, live screen stream, or control protocol
is not implied by this command.

device.status/health return only available uptime, storage, and memory fields;
they do not fabricate battery, network, or health measurements. device.permissions
uses status/promptable entries and a namespaced local-grant extension.
Notifications are quiet overlays with a 15-second lifetime, not OS notification
center entries. Custom sounds and system-only delivery explicitly fail.

## Examples

```json
{"command":"echolocal.audio.volume.set","params":{"channel":"media","volume":35}}
{"command":"echolocal.audio.mute.set","params":{"channel":"media","muted":true}}
{"command":"screen.snapshot","params":{"format":"png","maxWidth":1024}}
{"command":"screen.record","params":{"durationMs":3000,"fps":15,"includeAudio":false}}
```

The outbound node.invoke.result lane permits a bounded 9 MiB envelope for the
existing 8 MiB result contract. Ordinary RPC requests retain their 128 KiB limit.

Node implementation, local adapter tests, hardware deployment, Gateway
authorization, and successful remote invocations are separate verification
stages. Do not infer one from another.

Use the agent nodes tool actions screen_snapshot and screen_record for media.
Generic invoke intentionally rejects media payloads to avoid base64 context bloat.
