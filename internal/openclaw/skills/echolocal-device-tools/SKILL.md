---
name: echolocal-device-tools
description: Use currently published EchoLocal node tools for device control; inspect schemas and permissions before calling.
---
# EchoLocal device tools

Use only the currently connected device's discovered tools and their current schemas.
Discover connected nodes and their live tool catalog using the normal node/tool discovery.
Read device.info and device.permissions when choosing a device or diagnosing missing access.
Names such as echolocal_audio_volume_set map to exact commands such as
echolocal.audio.volume.set; send the tool's parameters object without extra wrappers.
If using node invocation instead, use the descriptor's exact command and the same
parameters as params. Never substitute a tool name for a command.

Registry action tools are different: put action fields inside args (an object).
YouTube TV is the configured channel-guide/cast integration, NOT an arbitrary
YouTube URL player. Supported operations are watch and whats_on; omitted/empty
operation defaults to watching. channel is a channel/show name, device is an
optional configured TV name. Without device it prefers this device, then the
configured default. Missing guide/target/channel can return an explanatory answer
without a device action. A watch answer schedules asynchronous casting: it does
not prove playback. Never invent channels, target names, URLs or operations.

Synthetic examples (valid shapes, NOT actions performed):
- echolocal_action_youtube_tv: {"args":{"operation":"whats_on","channel":"ESPN"}}
- echolocal_action_youtube_tv: {"args":{"operation":"watch","channel":"ESPN","device":""}}
- echolocal_audio_status: {}
- echolocal_audio_volume_set: {"channel":"media","volume":35}
- echolocal_audio_mute_set: {"channel":"media","muted":true}
- echolocal_display_brightness_set: {"brightness":40}
- echolocal_timer_create: {"seconds":60,"name":"Example"}
- echolocal_alarm_create: {"time":"06:30","name":"Example","enabled":true}
- echolocal_talk_status: {}
- echolocal_talk_start: {"slot":1}

Channel volumes use 0..100; media-player entity volume uses 0..1. Speaker mute
never mutes the microphone. Display power is backlight control, NOT shutdown.
Brightness selects manual mode. timer.list/cancel cover node-created timers only.
Alarm update preserves omitted fields; registry alarm_set replaces an alarm.
Talk start uses an existing configured slot (1 or 2), never supplies credentials
or replaces the user's configured prompts. Talk stop stops only a node-owned
conversation, not an unrelated local conversation.

Catalog presence is not authorization. Both Gateway command/tool policy and
local permission groups apply on every invocation. Revocation cancels/fences
active work. Missing tools can mean disconnect, unsupported hardware, disabled
local grants, unapproved commands or disabled Gateway node-tool policy. Do not
expand permissions, pair devices or change policy to work around this.
Ask the owner when required; guidance never overrides user prompts or grants.

On invalid_params, inspect the schema and field/shape diagnostic; do not repeat
sensitive user values or retry a state-changing action blindly. busy, cancelled,
permission_denied, unavailable and not_found are not success. Distinguish a valid
schema, accepted command, returned result and independently observed hardware
behavior. Screenshots, microphone/camera data, settings and results may be private.
