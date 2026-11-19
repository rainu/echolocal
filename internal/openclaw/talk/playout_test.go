package talk

import "testing"

func TestPlayoutPrefillsAndKeepsBoundedReserve(t *testing.T) {
	var b PlayoutBuffer
	frame := make([]int16, 960*2)
	for i := range frame {
		frame[i] = 100
	}
	for i := 0; i < 5; i++ {
		b.Push(frame)
		if len(b.Take(0)) != 0 {
			t.Fatal("playout began before jitter reserve")
		}
	}
	b.Push(frame)
	out := b.Take(0)
	if len(out) != 4800*2 || b.Queued() != 960 {
		t.Fatal("wrong lane/reserve duration")
	}
	for i := 0; i < 100; i++ {
		b.Push(frame)
	}
	if b.Queued() > maxBufferFrames || b.Dropped == 0 {
		t.Fatal("unbounded output")
	}
	b.Reset()
	if b.Queued() != 0 || len(b.Take(0)) != 0 {
		t.Fatal("stopped output replayed")
	}
}
func TestSilenceDoesNotKeepSpeakingStateAlive(t *testing.T) {
	if Audible(make([]int16, 1920)) {
		t.Fatal("silence classified as speech")
	}
	quiet := make([]int16, 1920)
	for i := range quiet {
		quiet[i] = 2
	}
	if Audible(quiet) {
		t.Fatal("comfort noise classified as speech")
	}
	for i := range quiet {
		quiet[i] = 100
	}
	if !Audible(quiet) {
		t.Fatal("quiet speech suppressed")
	}
}

func TestShortTailEventuallyPlaysAndResetRestartsPrefill(t *testing.T) {
	var b PlayoutBuffer
	b.Push(make([]int16, 960*2))
	for i := 0; i < 11; i++ {
		if len(b.Take(0)) != 0 {
			t.Fatal("short tail bypassed prefill delay")
		}
	}
	if len(b.Take(0)) != 1920 || b.Queued() != 0 {
		t.Fatal("short tail stranded")
	}
	b.Reset()
	b.Push(make([]int16, 1920))
	if len(b.Take(0)) != 0 {
		t.Fatal("reset did not restore prefill")
	}
}

func TestPlayoutLearnsBoundedReserveWithoutPausingActiveSpeech(t *testing.T) {
	var b PlayoutBuffer
	b.ObserveGap(20)
	if b.reserveFrames != 0 {
		t.Fatal("normal packet cadence increased latency")
	}
	b.ObserveGap(386)
	if b.reserveFrames != PlayoutRate*400/1000 {
		t.Fatal("stall reserve not learned/bounded")
	}
	b.Push(make([]int16, 960*2))
	for i := 0; i < 39; i++ {
		if len(b.Take(0)) != 0 {
			t.Fatal("learned reserve bypassed")
		}
	}
	if len(b.Take(0)) == 0 {
		t.Fatal("short tail stranded by learned reserve")
	}
	b.Push(make([]int16, 960*2))
	b.ObserveGap(9999)
	if len(b.Take(0)) == 0 {
		t.Fatal("active speech paused to grow reserve")
	}
	if b.reserveFrames > PlayoutRate*400/1000 {
		t.Fatal("reserve unbounded")
	}
	b.Reset()
	if b.reserveFrames != PlayoutRate*400/1000 {
		t.Fatal("interruption discarded learned network reserve")
	}
}
