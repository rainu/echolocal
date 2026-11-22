package control

import (
	"github.com/ygelfand/echolocal/internal/board"
	"github.com/ygelfand/echolocal/internal/component"
	"strings"
)

// Explicitly reviewed bound Domain/Name pairs. Field kinds, choices, ranges and
// parsers come from the bound table, never from entity IDs or UI labels.
// Unknown additions fail closed; the all-board coverage audit must classify them.
type nodeSettingsPolicy struct{ Group, Names string }

var nodeSettingsPolicies = map[string]nodeSettingsPolicy{
	"avatar":     {"settings", "path zoom yaw pitch x y idle blink animation"},
	"access":     {"settings", "webdav"},
	"youtube":    {"audio", "lounge"},
	"alerts":     {"audio", "info success warning alert timer ring_for wake"},
	"apps":       {"settings", "order dock"},
	"assistants": {"audio", "1.threshold 1.tone 1.delivery 1.follow_up 1.buffer 1.max_listen 1.max_think 1.recordings 1.wake_screen 2.threshold 2.tone 2.delivery 2.follow_up 2.buffer 2.max_listen 2.max_think 2.recordings 2.wake_screen arbitration_wait arbitration_offset pryon pryon_stop gemini stop_sensitivity"},
	"bluetooth":  {"audio", "speaker"},
	"calendar":   {"settings", "span"},
	"call":       {"intercom", "incoming broadcasts pause_wake stream ring video_ring ringback"},
	"camera":     {"camera", "ev scene banding iso whitebalance effect brightness contrast saturation sharpness hue framerate_auto framerate main_on main_size sub_on sub_size keyframe quality noise"},
	"cameras":    {"camera", "tab scale refresh rotate"},
	"cast":       {"audio", "receiver youtube.skip"},
	"clock":      {"settings", "hours face position align size color backing date date_format date_pattern"},
	"diag":       {"diagnostics", "interval min_cores low_memory page_memory"},
	"doorbell":   {"settings", "chime ring_for live_for notify style visitor tab door1.name door1.video door1.events door1.talk door2.name door2.video door2.events door2.talk door3.name door3.video door3.events door3.talk door4.name door4.video door4.events door4.talk"},
	"feedback":   {"audio", "chime failure cancel hold"},
	"home":       {"settings", "enabled combine dashboard tessera.columns tessera.rows tessera.home_after_idle tessera.landscape tessera.portrait tessera.themed tessera.turn tessera.bottom tessera.follow rings.look rings.group rings.depth rings.side rings.hidden_floors rings.hidden_areas rings.hidden_kinds rings.labels control.lights group.lights control.switches group.switches"},
	"homealarm":  {"settings", "look tab triggered.show triggered.sound pending.show pending.sound arming.show arming.sound settled.show settled.for settled.sound"},
	"idle":       {"settings", "saver after media face position align size backing night night.lux night.after wake photo_every photo_fit photo_transition visual1 visual1.source visual2 visual2.source"},
	"lovelace":   {"settings", "dashboard view scale saver_dashboard saver_view saver_scale theme"},
	"media":      {"audio", "duck buttons player mini sendspin airplay airplay_audio_mode"},
	"microphone": {"audio", "mute_sound mute_led mixing gain leveling cancel sensitivity"},
	"photos":     {"settings", "app.every app.fit app.transition immich.enabled immich.favorites immich.memories immich.quality google.enabled google.album1.weight google.album2.weight google.album3.weight google.album4.weight"},
	"poster":     {"settings", "on every fit transition"},
	"ring":       {"audio", "muted trouble reaction incoming talking 1.effect 1.thinking_effect 1.replying_effect 2.effect 2.thinking_effect 2.replying_effect"},
	"screen":     {"display", "backlight auto style font size theme drawer drawer.hide alert timer keyboard language marks touches custom.background custom.surface custom.text custom.muted custom.accent custom.accent2"},
	"speaker":    {"audio", "eq resampling speaker_output headphone_output speaker_gain"},
	"thermostat": {"settings", "look tab"},
	"todo":       {"settings", "completed"},
	"visual":     {"settings", "kind fps seed label lift"},
	"voice":      {"audio", "actions action.call action.broadcast action.alarm action.youtube_tv action.call_answer action.call_hangup commands label command.radar command.mirror"},
	"weather":    {"settings", "look dashboard idle alerts radar_source radar_range animate themed position align size saver.look saver.animate saver.themed saver.position saver.align saver.size"},
}
var nodeSettingsExclusions = map[string]string{
	"rtsp/enabled":         "Enables a persistent camera network server; local administration only.",
	"rtsp/audio":           "Exposes microphone audio through a persistent camera server; local administration only.",
	"access/adb":           "Authority, transport security or credential configuration; local administration only.",
	"access/adopted":       "Authority, transport security or credential configuration; local administration only.",
	"access/password":      "Authority, transport security or credential configuration; local administration only.",
	"bluetooth/proxy":      "Changes network proxy exposure; local administration only.",
	"bluetooth/scan":       "Changes network scanning; local administration only.",
	"calendar/entities":    "Account, endpoint, entity/media selection or unstructured private configuration; local editor only.",
	"call/answer_api":      "Changes remote call authorization; local administration only.",
	"call/answer_from":     "Changes caller authorization; local administration only.",
	"call/answer_peers":    "Account, endpoint, entity/media selection or unstructured private configuration; local editor only.",
	"call/answer_users":    "Account, endpoint, entity/media selection or unstructured private configuration; local editor only.",
	"call/auto_answer":     "Changes unattended call acceptance; local administration only.",
	"call/auto_video":      "Changes unattended camera acceptance; local administration only.",
	"cameras/url":          "Account, endpoint, entity/media selection or unstructured private configuration; local editor only.",
	"certs/cert":           "Authority, transport security or credential configuration; local administration only.",
	"certs/cert_api_key":   "Authority, transport security or credential configuration; local administration only.",
	"certs/key_api_key":    "Authority, transport security or credential configuration; local administration only.",
	"certs/server":         "Authority, transport security or credential configuration; local administration only.",
	"homealarm/entity":     "Account, endpoint, entity/media selection or unstructured private configuration; local editor only.",
	"lovelace/user":        "Selects dashboard identity; local administration only.",
	"network/home_key":     "Authority, transport security or credential configuration; local administration only.",
	"network/verify":       "Authority, transport security or credential configuration; local administration only.",
	"network/via_home":     "Authority, transport security or credential configuration; local administration only.",
	"photos/google.album1": "Account, endpoint, entity/media selection or unstructured private configuration; local editor only.",
	"photos/google.album2": "Account, endpoint, entity/media selection or unstructured private configuration; local editor only.",
	"photos/google.album3": "Account, endpoint, entity/media selection or unstructured private configuration; local editor only.",
	"photos/google.album4": "Account, endpoint, entity/media selection or unstructured private configuration; local editor only.",
	"photos/immich.albums": "Account, endpoint, entity/media selection or unstructured private configuration; local editor only.",
	"photos/immich.key":    "Sensitive field; never read or advertised by Node settings.",
	"photos/immich.people": "Account, endpoint, entity/media selection or unstructured private configuration; local editor only.",
	"photos/immich.server": "Account, endpoint, entity/media selection or unstructured private configuration; local editor only.",
	"photos/immich.tags":   "Account, endpoint, entity/media selection or unstructured private configuration; local editor only.",
	"rtsp/password":        "Sensitive field; never read or advertised by Node settings.",
	"rtsp/username":        "Account, endpoint, entity/media selection or unstructured private configuration; local editor only.",
	"screen/lock_pin":      "Sensitive field; never read or advertised by Node settings.",
	"todo/entities":        "Account, endpoint, entity/media selection or unstructured private configuration; local editor only.",
	"tv/default":           "Account, endpoint, entity/media selection or unstructured private configuration; local editor only.",
	"weather/entity":       "Account, endpoint, entity/media selection or unstructured private configuration; local editor only.",
}

// NodeSettingAudit classifies metadata only; callers must never pass values.
func NodeSettingAudit(domain, name string) (group, reason string) {
	p, ok := nodeSettingsPolicies[domain]
	if ok {
		for _, n := range strings.Fields(p.Names) {
			if n == name {
				return p.Group, ""
			}
		}
	}
	if r, ok := nodeSettingsExclusions[domain+"/"+name]; ok {
		return "", r
	}
	return "", "UNREVIEWED"
}

// NodeSettingAvailable supplements registration where legacy tables retain rows
// for hardware not fitted to this board. It never probes or reads private state.
func NodeSettingAvailable(domain, name string) bool {
	b := component.Board()
	if domain == "ring" {
		return b.Has(board.Ring)
	}
	if domain == "microphone" && name == "mute_led" {
		return b.Has(board.MuteLight)
	}
	if domain == "camera" || domain == "rtsp" {
		return b.CameraWidth > 0
	}
	return true
}
