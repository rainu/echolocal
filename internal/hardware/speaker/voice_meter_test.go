package speaker

import (
	"github.com/ygelfand/echolocal/internal/config"
	"math"
	"testing"
	"time"
)

func TestPlaybackLevelsWaitForHardwareAndExpire(t *testing.T) {
	var m playbackMeter
	now := time.Unix(10, 0)
	start := now.Add(120 * time.Millisecond)
	m.offer(m.epoch(), start, .01)
	if m.level(now) != 0 || m.level(start.Add(-time.Nanosecond)) != 0 {
		t.Fatal("animated queued audio")
	}
	if m.level(start) <= 0 {
		t.Fatal("missing audible period")
	}
	m.offer(m.epoch(), start.Add(period*time.Second/Rate), 0)
	if m.level(start.Add(period*time.Second/Rate)) != 0 {
		t.Fatal("silence did not reset")
	}
	m.offer(m.epoch(), start.Add(50*time.Millisecond), .01)
	if m.level(start.Add(51*time.Millisecond)) == 0 {
		t.Fatal("new speech missing")
	}
	if m.level(start.Add(time.Second)) != 0 {
		t.Fatal("stale speech held mouth open")
	}
}
func TestPlaybackInterruptionRejectsInflightAndBounds(t *testing.T) {
	var m playbackMeter
	now := time.Unix(10, 0)
	old := m.epoch()
	m.offer(old, now, .01)
	m.reset()
	m.offer(old, now.Add(time.Second), .01)
	if m.level(now) != 0 || m.level(now.Add(time.Second)) != 0 {
		t.Fatal("retired output resurrected")
	}
	for i := 0; i < 65; i++ {
		m.offer(m.epoch(), now.Add(time.Duration(i)*time.Millisecond), .01)
	}
	if m.count > 64 {
		t.Fatal("unbounded")
	}
	m.reset()
	m.offer(m.epoch(), now, math.NaN())
	if m.level(now) != 0 {
		t.Fatal("nonfinite output")
	}
}
func TestVoiceMeterOnlyMeasuresSubmittedVoiceAndRespectsMute(t *testing.T) {
	pcm := make([]int16, period*Channels)
	for i := range pcm {
		pcm[i] = 8000
	}
	p := newTestPlayer()
	buf := make([]byte, len(pcm)*2)
	for _, stream := range []config.Stream{config.StreamMedia, config.StreamFeedback} {
		p.bus.Push(stream, pcm)
		p.fill(buf)
		if p.voicePeriod != 0 {
			t.Fatal("unrelated audio animated mouth", stream)
		}
	}
	p.bus.Push(config.StreamVoice, pcm)
	if p.VoiceLevel() != 0 {
		t.Fatal("queued audio animated mouth")
	}
	p.fill(buf)
	if p.voicePeriod <= 0 {
		t.Fatal("missing voice measurement")
	}
	now := time.Now()
	p.voiceMeter.offer(p.voiceMeter.epoch(), now, p.voicePeriod)
	if p.VoiceLevel() == 0 {
		t.Fatal("submitted voice missing")
	}
	old := p.voiceMeter.epoch()
	p.Lane(config.StreamVoice).Drain()
	p.voiceMeter.offer(old, now, .5)
	if p.VoiceLevel() != 0 {
		t.Fatal("drain retained output")
	}
	p.Mute(config.StreamVoice, true)
	p.bus.Push(config.StreamVoice, pcm)
	p.fill(buf)
	if p.voicePeriod != 0 {
		t.Fatal("muted stream animated mouth")
	}
	p.Mute(config.StreamVoice, false)
	p.SetVolume(0)
	p.bus.Push(config.StreamVoice, pcm)
	p.fill(buf)
	if p.voicePeriod != 0 {
		t.Fatal("zero output volume animated mouth")
	}
}
