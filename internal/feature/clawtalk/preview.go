//go:build !board_biscuit

package clawtalk

import (
	"encoding/binary"
	"github.com/ygelfand/echolocal/internal/feature/tones"
	"github.com/ygelfand/echolocal/internal/hardware/speaker"
	"github.com/ygelfand/libcountertop/pkg/audio/mix"
	"github.com/ygelfand/libcountertop/pkg/audio/tone"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

// tonePCM is shared with live setup feedback. Rendering never touches a speaker lane.
func tonePCM(choice, cue string) []int16 {
	if choice != "" {
		sound := tones.Get().Sound(schema.Chime(choice), schema.ChimeChirp)
		if len(sound.Clip) > 0 {
			return mix.Scale(sound.Clip, tone.Level)
		}
		return mix.Chime(speaker.Rate, speaker.Channels, tone.Level, sound.Notes...)
	}
	notes := SetupNotes(cue)
	if cue == "end" {
		notes = DefaultNotes(true)
	}
	return mix.Chime(speaker.Rate, speaker.Channels, tone.Level, notes...)
}

// PreviewAudio provides owner-authenticated browser audition, without device playback,
// capture, routing changes or interruption of an active conversation.
func PreviewAudio(cue, choice string) []byte {
	pcm := tonePCM(choice, cue)
	// A valid short silent WAV represents the explicit "none" choice.
	if len(pcm) == 0 {
		pcm = make([]int16, speaker.Rate*speaker.Channels/50)
	}
	out := make([]byte, 44+len(pcm)*2)
	copy(out, "RIFF")
	binary.LittleEndian.PutUint32(out[4:], uint32(len(out)-8))
	copy(out[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(out[16:], 16)
	binary.LittleEndian.PutUint16(out[20:], 1)
	binary.LittleEndian.PutUint16(out[22:], uint16(speaker.Channels))
	binary.LittleEndian.PutUint32(out[24:], uint32(speaker.Rate))
	binary.LittleEndian.PutUint32(out[28:], uint32(speaker.Rate*speaker.Channels*2))
	binary.LittleEndian.PutUint16(out[32:], uint16(speaker.Channels*2))
	binary.LittleEndian.PutUint16(out[34:], 16)
	copy(out[36:], "data")
	binary.LittleEndian.PutUint32(out[40:], uint32(len(pcm)*2))
	for i, sample := range pcm {
		binary.LittleEndian.PutUint16(out[44+2*i:], uint16(sample))
	}
	return out
}
