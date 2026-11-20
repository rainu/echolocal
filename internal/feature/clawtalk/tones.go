package clawtalk

import (
	"github.com/ygelfand/libcountertop/pkg/audio/tone"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

// Empty values preserve the original ready sound and add its descending pair.
func DefaultNotes(end bool) []tone.Note {
	if end {
		return []tone.Note{{Freq: 1320, Ms: 55}, {Freq: 880, Ms: 70}}
	}
	return []tone.Note{{Freq: 880, Ms: 70}, {Freq: 1320, Ms: 55}}
}
func ToneNotes(choice string, end bool) []tone.Note {
	if choice == "" {
		return DefaultNotes(end)
	}
	return tone.Wake(schema.Chime(choice))
}

// Setup cues are intentionally different in pitch, rhythm, and direction.
func SetupNotes(cue string) []tone.Note {
	switch cue {
	case "ack":
		return []tone.Note{{Freq: 1760, Ms: 45}}
	case "progress":
		return []tone.Note{{Freq: 440, Ms: 110}, {Ms: 65}, {Freq: 440, Ms: 110}}
	case "failure":
		return []tone.Note{{Freq: 660, Ms: 100}, {Freq: 440, Ms: 100}, {Freq: 220, Ms: 160}}
	case "ready":
		return DefaultNotes(false)
	}
	return nil
}
