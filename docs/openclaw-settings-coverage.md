# Node settings coverage audit

Pinned library: e3fd8ed21859270023ff62f0c50a2398b1539038. Baseline app: 573cb283.

Generated from fresh, isolated registry initialization for every board in board.All, with matching build tags and pinned board identity. No setting values were collected. Only reviewed fields were inspected for kind/choice metadata; sensitive/authority fields were classified by exact bound names without reading them or their choice providers.

## Board coverage

| Board | Registered fields | Reviewed safe | Intentionally excluded | Safe rows without fitted hardware | Node adapter |
| --- | ---: | ---: | ---: | ---: | --- |
| athena | 297 | 258 | 39 | 3 | compiled |
| checkers | 298 | 259 | 39 | 3 | compiled |
| cronos | 296 | 257 | 39 | 3 | compiled |
| crown | 298 | 259 | 39 | 3 | compiled |
| rook | 292 | 257 | 35 | 20 | compiled |
| biscuit | 90 | 72 | 18 | 0 | not compiled; local-only |
| donut | 79 | 61 | 18 | 3 | not compiled; local-only |
| radar | 90 | 72 | 18 | 0 | not compiled; local-only |

312 distinct registered bound pairs across the eight variants; 273 reviewed safe pairs and 39 intentional exclusions. 24 reviewed safe pairs were blocked by the old domain/type filters. All those safe pairs now use the same resolver, discovery and parser validation; hardware and build availability still apply.

## Safe settings: before/after matrix

Every field listed below is supported after the change where registered, fitted and granted. Formats, numeric ranges and canonical choice values come from its bound table through live discovery. Required grants always include settings; the table lists the additional group. Empty dynamic choice lists are readable but not writable. No pair/triple/list field kind was found in these registered tables; toggle, number, choice and reviewed text are covered.

Rows are grouped only when domain, kind, prior access, grants and hardware/build availability agree. Each exact bound Name is listed (dots are part of the Name, not domain separators).

| Domain | Exact bound Names | Kind | Before | Additional grant | Registered boards | Fitted hardware |
| --- | --- | --- | --- | --- | --- | --- |
| alerts | `alert`, `info`, `success`, `wake`, `warning` | choice | supported* | audio | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| alerts | `ring_for`, `timer` | choice | supported* | audio | athena,checkers,cronos,crown,rook,biscuit,donut,radar | athena,checkers,cronos,crown,rook,biscuit,donut,radar |
| apps | `dock`, `order` | text | missing: field/ID filter | none | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| assistants | `1.delivery`, `1.tone`, `2.delivery`, `2.tone` | choice | supported* | audio | athena,checkers,cronos,crown,rook,biscuit,donut,radar | athena,checkers,cronos,crown,rook,biscuit,donut,radar |
| assistants | `1.buffer`, `1.follow_up`, `1.max_listen`, `1.max_think`, `1.recordings`, `1.threshold`, `2.buffer`, `2.follow_up`, `2.max_listen`, `2.max_think`, `2.recordings`, `2.threshold`, `arbitration_offset`, `arbitration_wait`, `stop_sensitivity` | number | supported* | audio | athena,checkers,cronos,crown,rook,biscuit,donut,radar | athena,checkers,cronos,crown,rook,biscuit,donut,radar |
| assistants | `1.wake_screen`, `2.wake_screen` | toggle | supported* | audio | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| assistants | `gemini`, `pryon`, `pryon_stop` | toggle | supported* | audio | athena,checkers,cronos,crown,rook,biscuit,donut,radar | athena,checkers,cronos,crown,rook,biscuit,donut,radar |
| bluetooth | `speaker` | toggle | missing: domain filter | audio | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| calendar | `span` | choice | supported* | none | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| call | `stream` | choice | supported* | intercom | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| call | `ring`, `ringback`, `video_ring` | choice | supported* | intercom | athena,checkers,cronos,crown,rook,biscuit,donut,radar | athena,checkers,cronos,crown,rook,biscuit,donut,radar |
| call | `broadcasts`, `incoming`, `pause_wake` | toggle | supported* | intercom | athena,checkers,cronos,crown,rook,biscuit,donut,radar | athena,checkers,cronos,crown,rook,biscuit,donut,radar |
| camera | `noise`, `sub_size` | choice | supported* | camera | athena | athena |
| camera | `banding`, `effect`, `main_size`, `quality`, `scene`, `whitebalance` | choice | supported* | camera | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown |
| camera | `hue`, `iso` | choice | supported* | camera | checkers,cronos,crown,rook | checkers,cronos,crown |
| camera | `brightness`, `contrast`, `ev`, `framerate`, `keyframe`, `saturation`, `sharpness` | number | supported* | camera | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown |
| camera | `sub_on` | toggle | supported* | camera | athena | athena |
| camera | `framerate_auto`, `main_on` | toggle | supported* | camera | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown |
| cameras | `refresh`, `rotate`, `scale` | number | supported* | camera | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| cameras | `tab` | toggle | supported* | camera | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| cast | `youtube.skip` | text | missing: domain filter | audio | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| cast | `receiver` | toggle | missing: domain filter | audio | athena,checkers,cronos,crown,rook,biscuit,donut,radar | athena,checkers,cronos,crown,rook,biscuit,donut,radar |
| clock | `align`, `backing`, `color`, `date_format`, `face`, `hours`, `position`, `size` | choice | supported* | none | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| clock | `date_pattern` | text | supported* | none | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| clock | `date` | toggle | supported* | none | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| diag | `page_memory` | choice | missing: domain filter | diagnostics | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| diag | `interval`, `min_cores` | number | missing: domain filter | diagnostics | athena,checkers,cronos,crown,rook,biscuit,donut,radar | athena,checkers,cronos,crown,rook,biscuit,donut,radar |
| diag | `low_memory` | toggle | missing: domain filter | diagnostics | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| doorbell | `chime`, `door1.events`, `door1.talk`, `door1.video`, `door2.events`, `door2.talk`, `door2.video`, `door3.events`, `door3.talk`, `door3.video`, `door4.events`, `door4.talk`, `door4.video`, `live_for`, `notify`, `ring_for`, `style`, `visitor` | choice | supported* | none | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| doorbell | `door1.name`, `door2.name`, `door3.name`, `door4.name` | text | missing: field/ID filter | none | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| doorbell | `tab` | toggle | supported* | none | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| feedback | `cancel`, `chime`, `failure`, `hold` | choice | supported* | audio | athena,checkers,cronos,crown,rook,biscuit,donut,radar | athena,checkers,cronos,crown,rook,biscuit,donut,radar |
| home | `dashboard`, `rings.group`, `rings.look`, `rings.side`, `tessera.bottom`, `tessera.turn` | choice | supported* | none | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| home | `group.lights`, `group.switches`, `rings.depth`, `tessera.columns`, `tessera.rows` | number | supported* | none | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| home | `rings.hidden_areas`, `rings.hidden_floors`, `rings.hidden_kinds`, `rings.labels` | text | missing: field/ID filter | none | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| home | `combine`, `control.lights`, `control.switches`, `enabled`, `tessera.follow`, `tessera.home_after_idle`, `tessera.landscape`, `tessera.portrait`, `tessera.themed` | toggle | supported* | none | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| homealarm | `arming.sound`, `look`, `pending.sound`, `settled.for`, `settled.sound`, `triggered.sound` | choice | supported* | none | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| homealarm | `arming.show`, `pending.show`, `settled.show`, `tab`, `triggered.show` | toggle | supported* | none | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| idle | `after`, `align`, `backing`, `face`, `media`, `night`, `night.after`, `photo_every`, `photo_fit`, `photo_transition`, `position`, `saver`, `size`, `visual1`, `visual1.source`, `visual2`, `visual2.source`, `wake` | choice | supported* | none | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| idle | `night.lux` | number | supported* | none | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| lovelace | `dashboard`, `saver_dashboard`, `saver_view`, `theme`, `view` | choice | supported* | none | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| lovelace | `saver_scale`, `scale` | number | supported* | none | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| media | `player` | choice | supported* | audio | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| media | `airplay_audio_mode`, `buttons` | choice | supported* | audio | athena,checkers,cronos,crown,rook,biscuit,donut,radar | athena,checkers,cronos,crown,rook,biscuit,donut,radar |
| media | `duck` | number | supported* | audio | athena,checkers,cronos,crown,rook,biscuit,donut,radar | athena,checkers,cronos,crown,rook,biscuit,donut,radar |
| media | `mini` | toggle | supported* | audio | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| media | `airplay`, `sendspin` | toggle | supported* | audio | athena,checkers,cronos,crown,rook,biscuit,donut,radar | athena,checkers,cronos,crown,rook,biscuit,donut,radar |
| microphone | `mixing`, `mute_sound` | choice | supported* | audio | athena,checkers,cronos,crown,rook,biscuit,donut,radar | athena,checkers,cronos,crown,rook,biscuit,donut,radar |
| microphone | `mute_led` | choice | supported* | audio | athena,checkers,cronos,crown,rook,biscuit,donut,radar | biscuit,radar |
| microphone | `gain`, `sensitivity` | number | supported* | audio | athena,checkers,cronos,crown,rook,biscuit,donut,radar | athena,checkers,cronos,crown,rook,biscuit,donut,radar |
| microphone | `cancel`, `leveling` | toggle | supported* | audio | athena,checkers,cronos,crown,rook,biscuit,donut,radar | athena,checkers,cronos,crown,rook,biscuit,donut,radar |
| photos | `app.every`, `app.fit`, `app.transition`, `immich.quality` | choice | supported* | none | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| photos | `google.album1.weight`, `google.album2.weight`, `google.album3.weight`, `google.album4.weight` | number | supported* | none | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| photos | `google.enabled`, `immich.enabled`, `immich.favorites`, `immich.memories` | toggle | supported* | none | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| poster | `every`, `fit`, `transition` | choice | supported* | none | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| poster | `on` | toggle | supported* | none | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| ring | `muted`, `trouble` | choice | supported* | audio | athena,checkers,cronos,crown,rook,biscuit,donut,radar | biscuit,radar |
| ring | `1.effect`, `1.replying_effect`, `1.thinking_effect`, `2.effect`, `2.replying_effect`, `2.thinking_effect`, `incoming`, `reaction`, `talking` | choice | supported* | audio | biscuit,radar | biscuit,radar |
| screen | `alert`, `drawer`, `drawer.hide`, `font`, `keyboard`, `language`, `size`, `style`, `theme`, `timer`, `touches` | choice | supported* | display | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| screen | `backlight` | number | supported* | display | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| screen | `custom.accent`, `custom.accent2`, `custom.background`, `custom.muted`, `custom.surface`, `custom.text` | text | missing: field/ID filter | display | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| screen | `auto`, `marks` | toggle | supported* | display | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| speaker | `resampling` | choice | supported* | audio | athena,checkers,cronos,crown,rook,biscuit,donut,radar | athena,checkers,cronos,crown,rook,biscuit,donut,radar |
| speaker | `headphone_output`, `speaker_output` | choice | supported* | audio | checkers,crown,biscuit,radar | checkers,crown,biscuit,radar |
| speaker | `speaker_gain` | number | supported* | audio | rook,biscuit,donut,radar | rook,biscuit,donut,radar |
| speaker | `eq` | toggle | supported* | audio | athena,checkers,cronos,crown,rook,biscuit,donut,radar | athena,checkers,cronos,crown,rook,biscuit,donut,radar |
| thermostat | `look` | choice | supported* | none | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| thermostat | `tab` | toggle | supported* | none | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| todo | `completed` | toggle | supported* | none | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| visual | `fps`, `kind` | choice | supported* | none | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| visual | `lift`, `seed` | number | supported* | none | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| visual | `label` | text | supported* | none | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| voice | `label` | choice | supported* | audio | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| voice | `command.mirror` | toggle | supported* | audio | athena,checkers,cronos,crown | athena,checkers,cronos,crown |
| voice | `action.youtube_tv`, `command.radar`, `commands` | toggle | supported* | audio | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| voice | `action.alarm`, `action.broadcast`, `action.call`, `action.call_answer`, `action.call_hangup`, `actions` | toggle | supported* | audio | athena,checkers,cronos,crown,rook,biscuit,donut,radar | athena,checkers,cronos,crown,rook,biscuit,donut,radar |
| weather | `align`, `look`, `position`, `radar_range`, `radar_source`, `saver.align`, `saver.look`, `saver.position`, `saver.size`, `size` | choice | supported* | none | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| weather | `alerts`, `animate`, `dashboard`, `idle`, `saver.animate`, `saver.themed`, `themed` | toggle | supported* | none | athena,checkers,cronos,crown,rook | athena,checkers,cronos,crown,rook |
| youtube | `lounge` | choice | missing: domain filter | audio | biscuit,donut,radar | biscuit,donut,radar |

\* Prior primitive-type exposure was broader than intended and returned localized choice labels. The old text filter also expected apps_order/apps_dock, while the bound tables use different entity IDs; Apps order/dock are fixed by resolving exact Domain/Name. Import additionally resolved only the first bound table in a domain, unlike get/export; multiple-table media, microphone and speaker settings could therefore be incorrectly denied on import. The new import resolves every exact pair and prevalidates all values before saving.

Ring controls require a fitted Ring capability. Microphone mute-LED brightness requires MuteLight. Camera controls require a fitted camera. Headphone/gain controls are additionally governed by their actual table registration. On speaker-only Biscuit-family builds, the Node adapter is not compiled: review of safe pairs does not advertise an unavailable Node capability.

## Intentional exclusions

Credential/authority names are deliberately not enumerated in agent-facing documentation or discovery. The exact reviewed pairs and individual reasons are maintained in node_settings_policy.go, and every registered field must match a safe review or an explicit exclusion. Counts below cover distinct pairs, not repeated board registrations.

| Rationale | Distinct excluded fields |
| --- | ---: |
| Account, endpoint, entity/media selection or unstructured private configuration; local editor only. | 17 |
| Authority, transport security or credential configuration; local administration only. | 10 |
| Changes caller authorization; local administration only. | 1 |
| Changes network proxy exposure; local administration only. | 1 |
| Changes network scanning; local administration only. | 1 |
| Changes remote call authorization; local administration only. | 1 |
| Changes unattended call acceptance; local administration only. | 1 |
| Changes unattended camera acceptance; local administration only. | 1 |
| Enables a persistent camera network server; local administration only. | 1 |
| Exposes microphone audio through a persistent camera server; local administration only. | 1 |
| Selects dashboard identity; local administration only. | 1 |
| Sensitive field; never read or advertised by Node settings. | 3 |

This deliberately tightens previously broad primitive-kind access for dashboard identity and unattended caller/camera authorization. Bluetooth proxy/scanning and persistent camera/microphone streaming servers remain local-only. Connection credentials, authority data, certificates and private Cast configuration never become Node settings; separately stored connection/Talk editors are not part of this registry API.

## Reproduce

Run TestNodeSettingsCoverageAudit in a fresh process per board, setting NODE_SETTINGS_AUDIT to the codename and optionally NODE_SETTINGS_AUDIT_OUT to a private JSON output file. Pin internal/board.pinned with the linker to that board before package initialization. Use board_biscuit for biscuit/donut/radar and the corresponding board tag otherwise. The test fails on unreviewed identifiers, duplicate bound pairs and reviewed fields becoming sensitive. Never publish private audit JSON or runtime settings.

See [wire contract, discovery and diagnostics](openclaw-node-settings.md). Publication acknowledgment, local executor tests, ordinary agent delivery and actual Talk verification are distinct checks; only report checks actually performed.
