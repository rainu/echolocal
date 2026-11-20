//go:build !board_biscuit

package control

import (
	"fmt"
	esphome "github.com/ygelfand/go-esphome-device"
	libalarm "github.com/ygelfand/libcountertop/pkg/assistant/alarm"
	"strings"
	"time"
)

type nodeSchema = map[string]any

func nodeObject(props nodeSchema, required ...string) nodeSchema {
	s := nodeSchema{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}
func nodeString(limit int, description string) nodeSchema {
	s := nodeSchema{"type": "string", "description": description}
	if limit > 0 {
		s["maxLength"] = limit
		s["x-maxBytes"] = limit
	}
	return s
}
func nodeEnum(values ...string) nodeSchema { return nodeSchema{"type": "string", "enum": values} }
func nodeNumber(kind string, min, max float64) nodeSchema {
	return nodeSchema{"type": kind, "minimum": min, "maximum": max}
}
func nodeBool() nodeSchema { return nodeSchema{"type": "boolean"} }
func nodeID() nodeSchema {
	s := nodeString(128, "Identifier returned by the matching list/create command.")
	s["minLength"] = 1
	return s
}
func nodeContract(name string) (string, nodeSchema) {
	empty := nodeObject(nodeSchema{})
	str := func(n int) nodeSchema { return nodeString(n, "") }
	channel := func() nodeSchema {
		s := nodeEnum("", "main", "media", "voice", "alerts", "feedback")
		s["default"] = "main"
		return s
	}
	switch name {
	case "device.info":
		return "Read EchoLocal version, build and hardware identity (no credentials).", empty
	case "device.status":
		return "Read device uptime and application storage usage.", empty
	case "device.health":
		return "Read bounded memory health statistics.", empty
	case "device.permissions":
		return "Inspect hardware availability, microphone mute and local node permission groups; cannot grant access.", empty
	case "device.sensors":
		return "Read the vetted environmental/system sensors and mic-mute/headphone indicators.", empty
	case "diagnostics.status":
		return "Read bounded local health and input-availability diagnostics.", empty
	case "camera.snap":
		return "Capture a JPEG still from the available camera; requires camera permission. Treat image as private.", empty
	case "tones.list":
		return "List available local custom tones.", empty
	case "display.pages":
		return "List currently configured dashboard page keys.", empty
	case "display.page":
		return "Show a key from display.pages; empty/omitted key returns to the clock.", nodeObject(nodeSchema{"key": str(256)})
	case "display.status":
		return "Read backlight brightness, automatic mode and display dimensions.", empty
	case "display.brightness.set":
		return "Set backlight brightness 0..100 percent, rounding to an integer and selecting manual mode.", nodeObject(nodeSchema{"brightness": nodeNumber("number", 0, 100)}, "brightness")
	case "display.power.set":
		return "Turn the backlight off/on; wake and restore remembered brightness/automatic mode. NOT device shutdown.", nodeObject(nodeSchema{"on": nodeBool()}, "on")
	case "audio.status":
		return "Read speaker channel volumes (0..100) and mute states; NOT microphone mute.", empty
	case "audio.volume.set":
		return "Set a speaker channel's 0..100 percent volume (rounded); omitted channel means main.", nodeObject(nodeSchema{"channel": channel(), "volume": nodeNumber("number", 0, 100)}, "volume")
	case "audio.mute.set":
		return "Mute/unmute a speaker output channel; never controls microphone mute.", nodeObject(nodeSchema{"channel": channel(), "muted": nodeBool()}, "muted")
	case "audio.test":
		return "Play the configured local test-playback sound.", empty
	case "alarm.list":
		return "List persistent device alarms.", empty
	case "alarm.create", "alarm.update":
		p := nodeSchema{"id": str(128), "name": str(256), "time": nodeString(0, "24-hour clock, e.g. 06:30; checked by the alarm parser."), "days": nodeString(0, "Comma-separated weekdays, e.g. mon,tue; empty rings once."), "enabled": nodeBool(), "sound": str(128), "snoozeMinutes": nodeNumber("integer", 0, 60)}
		if name == "alarm.create" {
			p["id"] = nodeEnum("")
			p["enabled"].(nodeSchema)["default"] = true
			return "Create a persistent alarm. Time is required; omitted enabled defaults true; static volume. Empty id only.", nodeObject(p, "time")
		}
		return "Patch an existing alarm by id; omitted fields retain their current values.", nodeObject(p, "id")
	case "alarm.delete":
		return "Delete a persistent alarm by id from alarm.list.", nodeObject(nodeSchema{"id": nodeID()}, "id")
	case "timer.list":
		return "List only active timers created through this node executor; not all device/HA timers.", empty
	case "timer.create":
		return "Create a node-owned timer for 1..86400 seconds; returns its id.", nodeObject(nodeSchema{"seconds": nodeNumber("integer", 1, 86400), "name": str(256)}, "seconds")
	case "timer.cancel":
		return "Cancel a timer owned by this node executor; other timers are not controllable here.", nodeObject(nodeSchema{"id": nodeID()}, "id")
	case "talk.status":
		return "Read whether Talk is active and its current state; no prompts or credentials.", empty
	case "talk.start":
		return "Start Talk using existing configured slot 1 or 2. Preserves user prompts; may fail when busy or unconfigured.", nodeObject(nodeSchema{"slot": nodeNumber("integer", 1, 2)}, "slot")
	case "talk.stop":
		return "Stop only the Talk generation started by this node executor; never an unrelated local conversation.", empty
	case "intercom.state":
		return "Read intercom activity, state, video and mute flags without peer addresses.", empty
	case "settings.export":
		return "Export only vetted settings fields in permitted domains, filtered by local group grants. Treat results as private.", empty
	case "settings.discover":
		return "Discover a compact safe domain/name/grant index; select a domain for full kinds, formats, ranges and choices. No values; filtered by hardware/build and local grants. Start here before settings.get/set.", nodeObject(nodeSchema{"domain": nodeString(128, "Optional exact domain; omit to list all available safe settings.")})
	case "settings.get":
		return "START with empty domain/name for a compact safe identifier/grant index; repeat with a selected domain and empty name for kinds/formats/ranges/choices. Both nonempty read one raw value, not a translated label. No credentials/authority.", nodeObject(nodeSchema{"domain": str(0), "name": str(0)}, "domain", "name")
	case "settings.set":
		return "Change a safe setting from settings.discover using its documented JSON string value (toggle: on or off). Requires listed grants. For Cast use domain cast, name receiver, value on. No authority/credential edits.", nodeObject(nodeSchema{"domain": str(0), "name": str(0), "value": str(4096)}, "domain", "name", "value")
	case "settings.import":
		fields := nodeSchema{"type": "object", "additionalProperties": str(4096)}
		values := nodeSchema{"type": "object", "additionalProperties": fields, "minProperties": 1}
		return "Apply nested domain -> setting-name -> string values, only to vetted fields with settings/domain grants. Validation of setting values remains domain-specific.", nodeObject(nodeSchema{"values": values}, "values")
	case "input.text":
		return "Inspect/edit the focused non-password field. Empty action returns only target/revision; replace/save/cancel require that fresh target and revision. OpenClaw/permission editors are blocked.", nodeObject(nodeSchema{"target": str(0), "text": str(16384), "action": nodeEnum("", "replace", "save", "cancel"), "key": str(0), "revision": nodeSchema{"type": "integer", "minimum": 0}})
	case "maintenance.restart", "maintenance.purgeCache":
		verb := "Restart EchoLocal, not the device"
		if name == "maintenance.purgeCache" {
			verb = "Purge the local application cache"
		}
		return verb + "; explicit confirm=true and local maintenance grant required.", nodeObject(nodeSchema{"confirm": nodeSchema{"type": "boolean", "enum": []bool{true}}}, "confirm")
	case "maintenance.updateCheck":
		return "Check the configured firmware release source; does not install an update.", empty
	case "maintenance.updateInstall":
		version := str(80)
		version["pattern"] = `^v?[0-9][a-zA-Z0-9._-]{0,79}$`
		return "Install a named build through the existing updater; confirm=true and maintenance permission required. Never use for a metadata check.", nodeObject(nodeSchema{"confirm": nodeSchema{"type": "boolean", "enum": []bool{true}}, "version": version}, "confirm", "version")
	case "system.notify":
		return "Show a quiet 15-second message overlay. Sound is unsupported; priority is accepted but ignored. No audio/microphone permission implied.", nodeObject(nodeSchema{"title": str(256), "body": nodeSchema{"type": "string", "minLength": 1, "maxLength": 4096, "x-maxBytes": 4096}, "sound": nodeEnum(""), "priority": str(0), "delivery": nodeEnum("", "overlay", "auto")}, "body")
	case "screen.snapshot":
		execution := str(0)
		execution["description"] = "Optional execution UUID; empty or lowercase UUID version 1..5."
		return "Capture the composed screen as JPEG (default quality .8) or PNG. maxWidth fits the larger dimension; zero is native. Only screenIndex 0. Private content may be visible.", nodeObject(nodeSchema{"executionId": execution, "screenIndex": nodeNumber("integer", 0, 0), "maxWidth": nodeSchema{"type": "integer", "minimum": 0}, "quality": nodeNumber("number", 0, 1), "format": nodeEnum("", "jpeg", "png")})
	case "screen.record":
		duration := nodeNumber("integer", 0, 10000)
		duration["default"] = 3000
		return "Record screen-only MP4, at 15fps, up to 10 seconds; durationMs=0 defaults to 3000. No audio capture. One recording at a time; bounded result.", nodeObject(nodeSchema{"durationMs": duration, "screenIndex": nodeNumber("integer", 0, 0), "fps": nodeSchema{"type": "number", "enum": []int{0, 15}}, "format": nodeEnum("", "mp4"), "includeAudio": nodeSchema{"type": "boolean", "enum": []bool{false}}})
	}
	// Legacy fixed control adapters accept bounded argv, never a shell string.
	legacy := map[string]string{
		"input.tap":        "Touch x y in current display coordinates; optional trailing harness arguments.",
		"input.swipe":      "Swipe x1 y1 x2 y2 [steps], each coordinate in current display bounds; steps 1..200.",
		"input.hold":       "Hold x y [milliseconds], duration 0..5000.",
		"input.button":     "Send the fixed harness button command; no arbitrary control path.",
		"display.message":  "Show a message using argv: TONE SECONDS TITLE : BODY; seconds 1..300.",
		"display.clear":    "Clear the local display message.",
		"display.open":     "Open exactly one vetted page: settings or camera.",
		"alarm.control":    "Rehearse an alarm with argv SECONDS [sunrise|ember|white] [silent], or snooze/stop. Not persistent alarm creation.",
		"timer.control":    "Display timer argv: SECONDS [NAME], ring [silent], stop or cancel. Uses the separate harness timer, not node-created timer IDs.",
		"intercom.control": "Legacy fixed call harness argv; peer details are never returned.",
	}
	if description, ok := legacy[name]; ok {
		args := nodeSchema{"type": "array", "items": str(4096), "maxItems": 32}
		min := map[string]int{"input.tap": 2, "input.swipe": 4, "input.hold": 2, "display.message": 3, "display.open": 1}[name]
		args["minItems"] = min
		if name == "display.open" {
			args["maxItems"] = 1
			args["items"] = nodeEnum("settings", "camera")
		}
		required := []string{}
		if min > 0 {
			required = append(required, "args")
		}
		return description + " Requires the corresponding local grant; input is disabled in sensitive editors.", nodeObject(nodeSchema{"args": args}, required...)
	}
	if strings.HasPrefix(name, "button.") {
		descriptions := map[string]string{"poster_next": "Show the next poster.", "poster_previous": "Show the previous poster.", "dashboard_reload": "Reload the dashboard.", "camera_reset": "Reset camera controls."}
		if d, ok := descriptions[strings.TrimPrefix(name, "button.")]; ok {
			return d, empty
		}
	}
	return "", nil
}

// Action argument names, primitive types and descriptions come from the actual
// registry. Optional fields retain the adapter's zero/empty defaults.
func nodeActionContract(action *esphome.Action) (string, nodeSchema) {
	props := nodeSchema{}
	for _, arg := range action.Args {
		var s nodeSchema
		switch arg.Type {
		case esphome.ArgString:
			s = nodeString(48000, arg.Description)
		case esphome.ArgInt:
			s = nodeNumber("integer", -2147483648, 2147483647)
		case esphome.ArgFloat:
			s = nodeSchema{"type": "number"}
		case esphome.ArgBool:
			s = nodeBool()
		default:
			panic("undocumented action argument type")
		}
		if arg.Description != "" {
			s["description"] = arg.Description
		}
		props[arg.Name] = s
	}
	description := action.Description
	if description == "" {
		description = fmt.Sprintf("Invoke the vetted %s registry action; action fields are nested in args and omitted fields use adapter defaults.", action.Name)
	}
	if action.Name == "youtube_tv" {
		description = "YouTube TV channel-guide/cast action, NOT arbitrary YouTube URLs. Use nested args: operation=watch (default) or whats_on, channel=channel/show name, device=optional configured TV name. Empty device prefers this device then configured default. A watch answer only schedules asynchronous casting; an explanatory result may mean no guide/channel/target."
		props["operation"] = nodeEnum("", "watch", "whats_on")
		props["operation"].(nodeSchema)["default"] = "watch"
	}

	if action.Name == "alarm_set" {
		props["snooze_minutes"] = nodeNumber("integer", 0, float64(libalarm.MaxSnooze/time.Minute))
		props["light_minutes"] = nodeNumber("integer", 0, float64(libalarm.MaxLight/time.Minute))
		volumes := []string{""}
		for _, volume := range libalarm.Volumes() {
			volumes = append(volumes, string(volume))
		}
		props["volume"] = nodeEnum(volumes...)
		props["volume"].(nodeSchema)["default"] = "static"
		props["ramp_seconds"].(nodeSchema)["description"] = "For gentle volume only: 15..300 seconds. Ignored by static/silent volume."
		return description + " Requires time in 24-hour HH:MM format; replaces rather than patches an existing id. Omitted enabled is false. Static volume is the default.", nodeObject(nodeSchema{"args": nodeObject(props, "time")}, "args")
	}
	if action.Name == "show_message" {
		props["body"].(nodeSchema)["minLength"] = 1
		props["seconds"].(nodeSchema)["description"] = "Omitted/zero/negative uses 15 seconds; greater than 3600 is clamped to 3600."
		props["tone"].(nodeSchema)["description"] = "info, success, warning or alert; empty/unrecognized uses info."
		return description, nodeObject(nodeSchema{"args": nodeObject(props, "body")}, "args")
	}
	if action.Name == "alarm" {
		description = "Spoken alarm action: operation set/list/next/enable/disable/delete/stop/snooze. Set accepts spoken time and optional weekdays/name; target identifies an existing alarm for enable/disable/delete. Missing/unknown operations or targets may return explanatory text without an action."
	}
	if action.Name == "tone_write" || action.Name == "tone_remove" {
		props["key"].(nodeSchema)["pattern"] = `^[a-z0-9][a-z0-9_-]{0,63}$`
	}
	if action.Name == "tone_remove" || action.Name == "tone_rename" || action.Name == "alarm_enable" || action.Name == "alarm_delete" {
		key := "key"
		if strings.HasPrefix(action.Name, "alarm_") {
			key = "id"
		}
		return description, nodeObject(nodeSchema{"args": nodeObject(props, key)}, "args")
	}
	if action.Name == "tone_write" {
		props["page"] = nodeNumber("integer", 0, 0)
		props["pages"] = nodeNumber("integer", 1, 1)
		description += " Node uploads must be complete: page=0 and pages=1, no partial transfers."
		return description, nodeObject(nodeSchema{"args": nodeObject(props, "key", "page", "pages", "data")}, "args")
	}
	return description, nodeObject(nodeSchema{"args": nodeObject(props)})
}
