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
