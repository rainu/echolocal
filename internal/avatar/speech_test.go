package avatar

import (
	"math"
	"sync/atomic"
	"testing"
	"time"
)

func TestSpeechMouthBoundaries(t *testing.T) {
	for _, intensity := range []float64{0, .1, .5, 1} {
		last := 0.
		for _, level := range []float64{0, .05, .25, .75, 1, 2} {
			got := SpeechMouth(level, intensity)
			if got < last || got > intensity || math.Abs(got-math.Min(1, level)*intensity) > 1e-10 {
				t.Fatal(level, intensity, got)
			}
			last = got
		}
	}
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if SpeechMouth(bad, .5) != 0 || SpeechMouth(.5, bad) != 0 {
			t.Fatal("nonfinite")
		}
	}
}
func TestSpeechPreviewContainsPausesAndDynamics(t *testing.T) {
	silence, quiet, loud := false, false, false
	for i := 0; i < len(speechPreview); i++ {
		v := PreviewLevel(time.Duration(i) * 100 * time.Millisecond)
		silence = silence || v == 0
		quiet = quiet || (v > 0 && v < .3)
		loud = loud || v > .7
	}
	if !silence || !quiet || !loud {
		t.Fatal(silence, quiet, loud)
	}
}
func TestIntensityChangesLivePlaybackWithoutCaptureOrRestart(t *testing.T) {
	var starts, listens atomic.Int32
	var level atomic.Uint64
	level.Store(math.Float64bits(.8))
	c := New(t.TempDir(), func() (<-chan []int16, func()) { listens.Add(1); return nil, func() {} }, func() bool { return false }, func() {})
	helper(c, "normal", &starts)
	c.SetOutput(func() float64 { return math.Float64frombits(level.Load()) }, time.Minute)
	c.Start()
	defer c.Stop()
	for _, gain := range []float64{1, .5, .1, 0, .5} {
		c.Configure(Options{MouthIntensity: gain})
		eventually(t, func() bool { return math.Abs(c.Snapshot().Mouth-.8*gain) < .0001 })
	}
	level.Store(math.Float64bits(0))
	eventually(t, func() bool { return c.Snapshot().Mouth == 0 })
	if listens.Load() != 0 || starts.Load() != 1 {
		t.Fatal("capture or restart", listens.Load(), starts.Load())
	}
	c.Stop()
	eventually(t, func() bool { return !c.Snapshot().Running && c.Snapshot().Mouth == 0 })
}
func TestSpeechPreviewNeverCaptures(t *testing.T) {
	var starts, listens atomic.Int32
	c := New(t.TempDir(), func() (<-chan []int16, func()) { listens.Add(1); return nil, func() {} }, func() bool { return false }, func() {})
	helper(c, "normal", &starts)
	c.Configure(Options{Mouth: MouthSpeechPreview, MouthIntensity: .5})
	c.Start()
	defer c.Stop()
	eventually(t, func() bool { return c.Snapshot().Mouth > 0 })
	c.Configure(Options{Mouth: MouthSpeechPreview, MouthIntensity: 0})
	eventually(t, func() bool { return c.Snapshot().Mouth == 0 })
	if listens.Load() != 0 {
		t.Fatal("preview captured microphone")
	}
}
