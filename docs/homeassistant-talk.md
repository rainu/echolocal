# Home Assistant Talk actions

EchoLocal exposes three response-capable native actions through Home Assistant's
built-in ESPHome integration. No EchoLocal HACS/custom integration or Home
Assistant restart is required. Reuse the device's existing authenticated
ESPHome connection; no new OpenClaw credentials, grants or protocol are needed.

| Native action suffix | Inputs | Response |
| --- | --- | --- |
| `openclaw_talk_start` | `session_id`, `startup_prompt` (both strings) | `{"success": true, "active": true}` after transport/audio readiness |
| `openclaw_talk_stop` | None | `{"success": true, "active": false}` after voice resources are released |
| `openclaw_sessions` | None | `{"sessions": [{"id": "agent:example:voice-demo", "name": "Voice demo"}]}` |

## Calling the native actions

In **Developer tools > Actions**, find your device's ESPHome actions ending in
`openclaw_talk_start`, `openclaw_talk_stop` and `openclaw_sessions`. Copy the exact action names
shown by Home Assistant: their device-specific prefix depends on the registered
device name. The names below use `YOUR_DEVICE` as a placeholder.

Stock ESPHome requires both declared Start Talk fields. Use an empty string
(`""`) to select the corresponding device default independently. Do not omit
the fields. No `device_id` field is needed: the native action name identifies
the device.

```yaml
# Retrieve selectable session IDs and labels.
- action: esphome.YOUR_DEVICE_openclaw_sessions
  response_variable: available_sessions

# Start with both device defaults.
- action: esphome.YOUR_DEVICE_openclaw_talk_start
  data:
    session_id: ""
    startup_prompt: ""
  response_variable: openclaw_talk_started

# End only this device's voice conversation.
- action: esphome.YOUR_DEVICE_openclaw_talk_stop
  response_variable: openclaw_talk_stopped
```

For an explicit session, replace `session_id: ""` with an exact routing ID from
`available_sessions.sessions`; names are display labels, not identifiers.
For a one-call prompt override, replace `startup_prompt: ""` with the prompt.
Either field can stay empty while the other supplies an override. Response
variables are optional for start/stop; use one to consume the session catalog.
Action failures propagate to Home Assistant instead of returning false success.

## Defaults and one-call overrides

The device's first assistant profile supplies the default session/agent,
startup greeting, provider/model, tones and inactivity timeout. Empty input
selects the corresponding default. A non-empty prompt uses the configured
greeting's validation and speakable startup path (maximum 1536 UTF-8 bytes).
Whitespace-only prompts are invalid.

Overrides belong only to the activation. Assistant settings and selected
session, greeting and provider/model defaults are never rewritten. If the
default profile creates a fresh context per activation, automation follows that
behavior without persisting its transient ID. No new settings or owner UI are
introduced.

Discovery reuses the device chat app's selectable sidebar-session catalog,
follows pagination and excludes archived/system/cron/subagent sessions. It never
fetches transcripts. A failed, malformed or incomplete catalog raises an error;
only a successful empty catalog returns `{"sessions":[]}`.

## Failure and lifetime semantics

An active or connecting Talk returns `talk_already_active` without switching,
stopping or restarting it. A busy Home Assistant assistant returns
`talk_assistant_busy`. Invalid override IDs return `talk_invalid_session`.
Disconnected devices/gateways report failure; muted capture returns
`talk_microphone_muted`. Other setup failures report `talk_start_failed`,
not a successful queued response.

Start waits for actual transport/audio setup. Native actions use opt-in bounded
asynchronous dispatch so keepalives, discovery and Stop are not queued behind
setup. Home Assistant's own request timeout can expire before the device's
bounded setup deadline. An uncertain/timed-out start is not proof that nothing
started: inspect the device or call Stop before retrying. Do not blindly retry.

Stop is idempotent: it succeeds when no Talk is active. It cancels only this
device's voice activation and closes its voice transport. It never calls
`chat.abort`, deletes/resets an OpenClaw session or stops an independent text
task.

## Verification and separate follow-up

On the installed candidate, live calls through Home Assistant's built-in
ESPHome integration verified all four default/explicit-input combinations,
actual readiness, session IDs/names, invalid inputs, duplicate protection,
responsive discovery and repeated Stop. Assistant settings stayed byte-identical.

Go tests cover overrides, unchanged settings, default contexts, validation,
paginated/error catalogs, ownership and generation-safe stop. Library tests
cover read-loop responsiveness, bounded concurrency, correlation and cleanup.
No new physical-user-speech, wake-word or acoustic listening check is claimed.

[HACS PR #3](https://github.com/rainu/echolocal-hacs/pull/3) is a separate,
deferred convenience wrapper with truly omittable fields. It is not required
for these native actions and is not a deployment or acceptance dependency.
