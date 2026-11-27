//go:build !board_biscuit

package control

import (
	"reflect"
	"testing"

	"github.com/ygelfand/libcountertop/pkg/settings/settingtest"

	"github.com/ygelfand/echolocal/internal/config"
)

var coverage = settingtest.Shared.With(settingtest.Coverage{
	Rows: map[string]string{
		"Avatar.Foreground":           "avatar.foreground",
		"Avatar.FullscreenForeground": "avatar.fullscreen_foreground",
		"Avatar.TalkMode":             "avatar.talk_mode",
		"Avatar.FullscreenBackground": "avatar.fullscreen_background",
		"Avatar.FullscreenZoom":       "avatar.fullscreen_zoom",
		"Avatar.FullscreenYaw":        "avatar.fullscreen_yaw",
		"Avatar.FullscreenPitch":      "avatar.fullscreen_pitch",
		"Avatar.FullscreenX":          "avatar.fullscreen_x",
		"Avatar.FullscreenY":          "avatar.fullscreen_y",
		"Avatar.MouthIntensity":       "avatar.mouth_intensity",
		"Avatar.IdleDisabled":         "avatar.idle",
		"Avatar.BlinkDisabled":        "avatar.blink",
		"Avatar.AnimationDisabled":    "avatar.animation",
		"Avatar.Background":           "avatar.background",
		"Avatar.Path":                 "avatar.path",
		"Avatar.Zoom":                 "avatar.zoom",
		"Avatar.Yaw":                  "avatar.yaw",
		"Avatar.Pitch":                "avatar.pitch",
		"Avatar.X":                    "avatar.x",
		"Avatar.Y":                    "avatar.y",
		"Microphone.Gain":             "microphone.gain",
		"Microphone.Leveling":         "microphone.leveling",
		"Microphone.Mixing":           "microphone.mixing",
		"Microphone.Cancel":           "microphone.cancel",
		"Microphone.Sensitivity":      "microphone.sensitivity",
		"Microphone.LEDBright":        "microphone.mute_led",
		"Microphone.MuteSound":        "microphone.mute_sound",
		"Microphone.VisualizerLift":   "visual.lift",
		"Ring.Reaction":               "ring.reaction",
		"Ring.Trouble":                "ring.trouble",
		"Ring.Muted":                  "ring.muted",
		"Ring.Incoming":               "ring.incoming",
		"Ring.Talking":                "ring.talking",
		"Speaker.Resampling":          "speaker.resampling",
		"Speaker.ASP":                 "speaker.eq",
		"Speaker.SpeakerOutput":       "speaker.speaker_output",
		"Speaker.HeadphoneOutput":     "speaker.headphone_output",
		"Speaker.SpeakerGain":         "speaker.speaker_gain",
	},
	ByHand: map[string]string{
		"Files.View":       "on-device Files toolbar; not a picker or remote filesystem setting",
		"Files.Tile":       "on-device Files tile-size control",
		"Files.Sort":       "on-device Files sort control",
		"Files.Descending": "on-device Files direction control",
		"Files.Hidden":     "on-device Files visibility control; immutable private-path policy still enforced",
		"Device.Name":      "told to the process at start-up, never written to the file",
		"Device.Addr":      "the same",
		"Device.Board":     "the same, read from the hardware",

		"Speaker.Volume":   "the volume buttons and Home Assistant's media player",
		"Volume.Media":     "the volume command, by stream",
		"Volume.Voice":     "the volume command",
		"Volume.Alerts":    "the volume command",
		"Volume.Feedback":  "the volume command",
		"Microphone.Muted": "the mute button and Home Assistant's mute switch",

		"Ring.Light.On":         "Home Assistant's light entity for the ring",
		"Ring.Light.Brightness": "the same",
		"Ring.Light.Red":        "the same",
		"Ring.Light.Green":      "the same",
		"Ring.Light.Blue":       "the same",
		"Ring.Light.Effect":     "the same",

		"Lounge.Device":   "the Echo Dot's YouTube lounge; the Shows run none",
		"Lounge.Music":    "the same",
		"Lounge.Video":    "the same",
		"Lounge.OnDemand": "the same",
	},
})

func TestEverySettingIsReachedOrAccountedFor(t *testing.T) {
	coverage.Accounts(t, reflect.TypeFor[config.Config]())
}

func TestEverySettingSaysWhatItChanges(t *testing.T) {
	var names []string
	for _, s := range settings() {
		names = append(names, s.Name)
	}
	coverage.Names(t, names)
}
