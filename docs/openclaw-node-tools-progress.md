# EchoLocal Node tools (issue #13)

Development preview, not final owner acceptance. Operator/Talk credentials and scopes are unchanged. The Node uses its separate existing identity.

## Wire contract

The connect declaration includes only currently granted registered command names and their capability prefixes. Grant changes atomically persist local policy, cancel current invocations, release gestures and reconnect the Node. Unknown or missing permission fields default enabled; explicit false persists; unreadable or malformed policy fails closed.

Gateway events: `node.invoke.request` (id, nodeId, command, paramsJSON, timeoutMs), `node.invoke.cancel` (invokeId). Results use the authenticated socket's `node.invoke.result` request with id, nodeId, ok, payloadJSON or an error object. No shell, arbitrary control path, HTTP admin password or operator authority is used.

Arguments are JSON objects, at most 128 KiB; unknown fields are rejected. At most four requests execute concurrently; requests have a maximum 30-second lifetime. Results are bounded to 8 MiB. Errors are static identifiers (invalid_params, permission_denied, unsupported_command, cancelled, operation_failed, capture_unavailable, camera_unavailable, input_conflict, invalid_setting, confirmation_required, update_failed). Detailed underlying errors, credential objects, URLs and SDP are not returned.

## Operation matrix

| Command | Permission | Arguments and result |
|---|---|---|
| screen.snapshot | screen | {}; JPEG base64, width, height |
| camera.snap | camera | {}; JPEG base64, only camera-capable boards; existing privacy path |
| input.tap / swipe / hold | input | args array of decimal coordinates; existing serialized touch engine and guaranteed release; hold <=5 s, swipe <=200 steps |
| input.button | input | args: button name and optional repeat count; existing supported buttons |
| input.text | input | existing target/revision/action/text lifecycle; inspect returns target/revision only; replace/save/cancel; <=16 KiB; password fields and Node/admin menus unavailable |
| display.message / clear | display | existing local message args; 1..300 s lifetime; done acknowledgement |
| display.open | display | args: settings or camera; no arbitrary page name |
| display.pages / page | display | list current dashboard tabs; select known key or empty for clock |
| alarm.control / timer.control | alarms | existing bounded-request control args |
| action.alarm_list/set/enable/delete/stop/snooze/alarm | alarms | args object using existing registry schema; structured existing result |
| action.show_message / clear_message | display | existing registry arguments |
| audio.control.<registered-player-id> | audio | command play/pause/stop/mute/unmute/volume_up/volume_down/toggle; optional volume 0..1, HTTP(S) URL without embedded credentials, announcement |
| audio.background.<noise-layer-id> / audio.volume.<volume-id> | audio | optional value, existing options/range validation; current value/options or bounds |
| button.poster_next / poster_previous / dashboard_reload | display | {}; existing finite button |
| button.camera_reset | camera | {}; existing reset button |
| audio.test | audio | {}; existing test playback button |
| action.youtube_tv | audio | existing registry arguments |
| intercom.control | intercom | existing call-control args, no raw addressing/state report |
| intercom.state | intercom | {}; active/state/video/muted/cameraOff projection, no peer addresses or session credentials |
| action.call / call_answer / call_hangup | intercom | existing registry arguments; Node-created calls tracked for disconnect/policy-change cleanup |
| settings.export / import | settings plus field group | export safe typed fields; import values: domain -> field -> string. Full permission preflight before writes; each write rechecks cancellation/group. Field validation errors can leave earlier valid writes applied; not a transactional import. |
| device.info / sensors | state | identity/version/model projection and allowlisted numeric/binary sensors only |
| diagnostics.status | diagnostics | bounded safe diagnostic projection, not arbitrary logcat |
| maintenance.restart | maintenance | confirm:true; EchoLocal restart, NOT device reboot |
| maintenance.purgeCache | maintenance | confirm:true; existing cache-purge button |
| maintenance.updateCheck | maintenance | {}; existing EchoLocal update check |
| maintenance.updateInstall | maintenance | confirm:true and explicit version; existing context-aware application updater, never Android firmware flashing |
| tones.list / action.tone_write/rename/remove | tones | existing registry args; finite uploads limited to one complete <=32 KiB PCM page; no unfinished transfer survives revocation |

Actual registry metadata (all actions, fields and entities) is captured in [openclaw-node-registry.md](openclaw-node-registry.md). Hardware availability follows the instantiated component registry. Names not explicitly registered are not executable. Generic dispatch does not bypass policy: every exposed registry action has its own group-gated command.

## Settings boundary

Credential/secret kinds and authority domains (access, frontend, network, certs, cast, rtsp, OpenClaw policy/identities) are excluded. Free-text/list fields are excluded unless explicitly reviewed: clock date pattern, visual label, app order/dock and custom screen colors. Export never returns arbitrary configuration structs. Node cannot import its grants or edit credentials. Audio-related fields additionally require audio, camera/cameras require camera, call requires intercom, alarm/timer require alarms. Other approved non-secret domains use settings.

Local administration: Echo settings -> OpenClaw -> Node-Berechtigungen, individual groups and all-on/all-off; local root/ADB control also provides `ctl node-permissions [GROUP|all on|off]`. This command is never Node-advertised or reachable through a generic Node proxy. Disabling Node grants does not change HA/WebUI/operator permission settings.

## Explicit gaps and transport limits

This preview is not represented as complete issue acceptance:

- No invented live screen/camera streaming transport or general event subscription. Existing desktop/call WebRTC sessions have separate authenticated signaling and ownership; their URLs, credentials and raw SDP are not tunneled through Node.
- The existing broadcast registry action starts context-free asynchronous microphone capture or URL relaying. It is deliberately not exposed pending a cancellable, Node-owned service API. It cannot satisfy revocation cleanup by simply returning an acknowledgement.
- Existing call_offer/call_end signaling actions remain unavailable through Node; call controls use the existing peer discovery and calling service instead.
- Existing voice_enroll/voice_enroll_cancel/voice_enroll_accept/voice_profiles, recordings and turn_audio are not exposed: these need a separately reviewed biometric/private-recording surface.
- Registry settings_export/settings_import are replaced by the field-checked Node export/import commands.
- Raw registry logs are not exposed. diagnostics.status is a narrow projection; full bounded/redacted application-log delivery remains outstanding.
- Dashboard alert/choice methods are not in the instantiated local action registry on this build; separate finite adapters and event-result handling remain outstanding.
- wake_assistant_1/2 buttons are not exposed: they can activate operator/Talk authority, which a Node must not inherit.
- Full-device restart and implicit install-selected-version buttons are intentionally replaced by explicit application restart/version-selection commands.
- Settings tests, real Gateway checks, device installation and visual results must be reported separately. Synthetic tests are not hardware validation.

No merge or issue closure before owner acceptance of an exact tested commit.
