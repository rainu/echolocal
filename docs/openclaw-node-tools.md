# Published EchoLocal tools and guidance

EchoLocal publishes typed metadata **after** the Node hello, using
`node.pluginTools.update({"tools":[...]})` and
`node.skills.update({"skills":[...]})`. Both are complete replacement catalogs.
The connect `commands` array remains an allowlist of command strings, never
descriptors. Pairing, signed identities, operator scopes and user Talk prompts
are unchanged.

## Sources of truth

- `internal/feature/control/node_catalog.go`: reviewed fixed-adapter contracts.
- Registry actions derive field names, primitive types and descriptions from
  their actual registered action arguments, with reviewed adapter constraints.
- Entity adapters derive number ranges and select options from their entities.
- Each `NodeOperation` carries the very schema used for validation and publication.
- [Shipped usage skill](../internal/openclaw/skills/echolocal-device-tools/SKILL.md):
  embedded directly into the binary; not a separately maintained documentation copy.
  Its synthetic examples are validated against descriptors by tests.

All remotely registered adapters must have reviewed metadata; an undocumented
addition fails immediately instead of silently exposing an undocumented command.
Future unreviewed component actions remain excluded by the existing action
allowlist. Catalog descriptors include pluginId, provider-safe unique name,
description, parameters and the exact command. Dots map to underscores; core
commands receive the echolocal_ tool-name prefix too. For example,
`echolocal_action_youtube_tv` maps to `echolocal.action.youtube_tv`.

## Calling conventions and important differences

Use the **current** discovered schema. Registry actions wrap named fields inside
an `args` object; fixed control adapters may instead use typed top-level fields
or the explicitly documented legacy `args` string array. These are not
interchangeable. Unknown fields are rejected before performing an action.

YouTube TV is the configured channel-guide/casting integration, not a general
YouTube URL player:

```json
{"args":{"operation":"whats_on","channel":"ESPN"}}
```

```json
{"args":{"operation":"watch","channel":"ESPN","device":""}}
```

These are synthetic valid examples, not executed actions. Only watch and
whats_on (or the empty watch default) are accepted through the node adapter.
Channel/show and TV names must exist in the configured integration. Empty device
prefers this device, then the configured default. Missing guide/target/channel
can return an explanatory answer with no action; a watch answer schedules an
asynchronous cast and **does not prove playback**. Top-level url is invalid.

Speaker channel mute is not microphone mute. Typed channel volume is 0..100;
media-player entity volume is 0..1. Display power changes the backlight, not
device power. Explicit brightness selects manual mode. Timer list/cancel cover
only timers created by the node executor. Alarm update preserves omitted fields;
the registry alarm_set operation replaces. Talk start selects an already
configured slot; stop owns only the node-started conversation. It does not
replace configured system/conversation prompts. See the shipped skill for
schema-tested examples.

Schemas describe field types, required fields, closed/nested objects, primitive
ranges, supported enums and relevant defaults. UTF-8 byte bounds are stricter
than JSON Schema code-point maxLength: the schema additionally records x-maxBytes.
Semantic constraints (existing IDs, clock/weekday parsing, current screen
coordinates, live settings validation and hardware/service availability) are
checked by their existing adapters and described in their field/tool guidance.

## Lifecycle, bounds and compatibility

One publisher belongs to each authenticated socket and runs independently of the
RPC receive loop. It sends each catalog with an 8-second deadline, awaits its
acknowledgment, and never sends through a replacement socket. Disconnect cancels
the publisher and pending calls. Gateway live catalogs are connection-scoped and
removed by the Gateway on disconnect.

Registry/grant edits notify a coalescing replacement loop. Normal local grant
edits also reconnect, refreshing the handshake command ceiling. Revocation still
cancels active operations and fences stale results immediately; metadata can
never grant rights. Hardware-specific commands are registered only when their
startup adapter exists. Temporary backend unavailability is reported by the
command, not treated as a new grant or hidden successful execution.

Publication is capped at 96 KiB per replacement, below the ordinary 128 KiB RPC
budget. Individual parameter schemas are bounded to 16 KiB; descriptions to 1024 bytes
and the catalog to 128 tools. An oversized replacement withdraws the previous
tool catalog rather than publishing a truncated surface. Rejections, missing
methods, policy-filtered catalogs and publication timeouts do not fail the Node
connection, retry-loop an unsupported method, or disable existing commands.
A reconnect republishes; changes trigger replacement. Safe local status reports
acknowledged, filtered, unavailable, payload_too_large, pending or disconnected.
A successful acknowledgment alone is **not proof of agent visibility**.

## Gateway and agent policy

The target Gateway contract was checked against its current protocol schemas and
handlers: tool names match `^[A-Za-z][A-Za-z0-9_-]{0,63}$`; skills use a valid
lowercase hyphenated name, description <=1024 and content <=65536 characters.
Only the node role publishes metadata. Skill descriptor name and description
must match the embedded SKILL.md frontmatter exactly; both are read from the
same canonical header.

Gateway approval and command allowlists still filter descriptors. On current
Gateways, `gateway.nodes.pluginTools.enabled=false` disables node-published tools;
`gateway.nodes.allowSkills=false` withholds remote skills. Additional
agent/runtime tool policy, owner-only invocation, Talk tool filtering or
provider limitations may withhold them even after acknowledgment. Do not change
these settings, approve broader commands, copy credentials, or replace prompts
to make a test pass.

### Current shell-less-node compatibility limitation

The inspected Gateway remote-skill projection additionally requires both an
exec-capable agent surface and `node.commands.includes("system.run")`.
A command-only EchoLocal node intentionally does not expose system.run, so its
acknowledged skill can still be absent from normal skill discovery. This is a
Gateway runtime limitation, not a reason to advertise a fake shell, add command
grants or install a second copy of the skill. A Gateway fix supporting published
guidance from shell-less nodes is required to satisfy that delivery criterion.
The exact frontmatter contract is independently enforced before that projection.

Published skills are procedural guidance, not authorization or higher-priority
conversation instructions. No private endpoints, credentials, actual device
addresses, conversation IDs or user content belong in published metadata.

## Diagnostics and verification

On the device, the local-only `echod ctl node-catalog` diagnostic returns current
descriptors, the shipped guide, and bounded publication status. It contains no
setting values or credentials. Do not add it as a remote execution bypass.
`node-commands` still lists only granted wire commands.

Parameter failures use safe bounded messages such as
`invalid_params: $.args.channel: expected string` and wire error code
INVALID_PARAMS. Unknown user keys and values are never echoed; only schema-owned
paths and documented field names are included. Some adapter-specific semantic
failures give a generic reminder to check the documented constraints.

Run focused Go tests for `internal/openclaw` and `internal/feature/control`.
They cover catalog completeness, names/collisions, schema-tested guide examples,
nested YouTube arguments, no-action invalid input, replacement after grants,
reconnect, disconnect status, unsupported methods/disabled catalogs and continued
normal commands. Validate the real connected catalog against the target Gateway
schemas, then inspect normal agent discovery **and actual Talk tool delivery**
separately. Report schema validation, command acceptance and observed device
behavior independently; do not substitute a local test double for a live model
or a publication response for delivered tools.
