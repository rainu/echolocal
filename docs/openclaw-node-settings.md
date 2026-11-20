# Safe Node settings

Start with `echolocal_settings_discover({})` (command
`echolocal.settings.discover`). On clients that have not yet exposed the new
tool, `echolocal_settings_get({"domain":"","name":""})` returns the same
value-free discovery through the existing command. An empty name lists settings;
an empty domain lists all currently available domains.

Discovery returns exact bound `domain`/`name`, `kind`, accepted string format,
numeric range (including a declared zero lower bound), units/examples, canonical choice values, read/write availability and `requiredGrants`. It does not return
current values, secret fields, private endpoints, authority controls or unreviewed
fields. Only settings registered in this hardware/build and permitted by current
local grants are listed. Absence is not proof a device has no such hardware:
build tags and local policy also matter. Legacy ring/Mute-LED rows are additionally checked against board capabilities. Choices without configured options remain readable but not writable. Never derive names from UI labels or
Home Assistant entity IDs.

Examples (shapes only; no action is implied):

```json
{"domain":"cast","name":"receiver"}
{"domain":"cast","name":"receiver","value":"on"}
{"domain":"cast","name":"receiver","value":"off"}
{"domain":"clock","name":"date_pattern","value":"dd.MM.yyyy"}
```

Use the first object with settings.get and the others with settings.set.
Every value is a JSON **string**. Toggles accept exactly `"on"` or `"off"`;
numbers use decimal integers in the discovered range; choices use their exact
wire values, not translated labels. Other fields use the documented field parser.
Successful get returns the accepted wire representation, not a localized label.
A successful set reports a saved configuration, not independently proven hardware
operation. For Cast, verify the effective receiver listener too. Restore the
original value after a temporary test.

All settings operations require the local `settings` grant plus each discovered
domain grant. Cast/media/Bluetooth-speaker settings additionally require `audio`;
display settings require `display`, camera settings `camera`, call preferences
`intercom`, diagnostics `diagnostics`. Persistent RTSP camera/microphone server controls remain local-administration-only.
Discovery and export omit fields whose required grants are disabled. Revocation
still rejects get/set/import; no command can edit Node grants.

Errors (never include supplied values or secret names):

| Error | Meaning / next step |
| --- | --- |
| permission_denied | A genuinely required local grant is disabled. |
| unsupported_domain | Domain is not supported by this API. Discover available names. |
| unknown_setting | Name is not a reviewed setting in the specified domain. |
| setting_excluded | Known field is intentionally local-administration-only. |
| setting_unavailable | Reviewed identifier is absent in this hardware/build. |
| invalid_value | String failed wire-format, bounds, choice or actual table-parser validation. |
| operation_failed | Save/runtime application failed; read state before retrying. |
| cancelled | Invocation was cancelled or fenced by policy/lifecycle change. |

Thus `playback/cast_receiver` yields unsupported_domain, not a false permission
diagnostic. Persistent RTSP streaming controls, Cast credentials and authority configuration are not accessible
through discovery, get, set, import or export.

Import uses nested domain -> exact name -> string objects. Every identifier,
grant and value is checked before the first write, including fields split across
multiple bound tables in the same domain. Saves are not transactional: an actual
save failure can leave a partially applied import, explicitly reported as such.
Export uses the same readable reviewed fields and returns raw strings rather than translated labels. Currently non-writable fields must be omitted from imports until their choices are configured; an export is not a restore-all transaction. Text parsers may normalize, trim or truncate input, so read back the stored value.
Treat all returned settings as private.

## Maintained contract and hardware coverage

`internal/feature/control/node_settings_policy.go` holds exact reviewed bound
pairs and explicit exclusion reasons. No new field is admitted by kind alone.
The bound library tables remain the source of truth for names, kinds, ranges,
choices and parser validation. `Inspectable.Metadata` never reads configuration;
only an authorized, reviewed single-field request can invoke `RawValue`.
UI-oriented `Fields()` must not be used for Node discovery or filtering.

The isolated `TestNodeSettingsCoverageAudit` initializes each supported board
and fails on any unreviewed registered field. It only requests metadata for safe
fields; it never requests values or private-field choice providers.
See [coverage matrix](openclaw-settings-coverage.md) for before/after access,
required groups and actual board registration. Biscuit-family builds currently
do not compile the Node adapter at all; their settings remain local-only even
where the underlying safe pair is reviewed. The audit covers these builds so
safe fields do not silently disappear if that platform gains a Node adapter.

Gateway command policy is a separate authorization boundary. Metadata publication
does not prove normal-agent or Talk delivery. A client can use existing
settings.get with an empty name for discovery if the additional discover command
is unavailable; never add a shell, widen Gateway grants or change owner prompts
to work around missing tool/skill delivery.
