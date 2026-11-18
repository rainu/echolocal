package clawtalk

import (
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
	"testing"
)

func TestTalkTonesPreserveReadyAndProvideDistinctEndAndOff(t *testing.T) {
	start, end := ToneNotes("", false), ToneNotes("", true)
	if len(start) != 2 || len(end) != 2 || start[0].Freq != 880 || end[0].Freq != 1320 {
		t.Fatal("default feedback changed or end is indistinguishable")
	}
	if len(ToneNotes("none", false)) != 0 || len(ToneNotes("none", true)) != 0 {
		t.Fatal("off tone still plays")
	}
	if len(ToneNotes("ding", true)) == 0 {
		t.Fatal("selected tone missing")
	}
}

func TestEverySelectableBuiltInHasPlayback(t *testing.T) {
	for _, c := range schema.CallChimes() {
		if c != schema.ChimeNone && len(ToneNotes(string(c), false)) == 0 {
			t.Fatalf("no playback for %s", c)
		}
	}

}
