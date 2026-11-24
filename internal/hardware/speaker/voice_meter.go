package speaker

import (
	"math"
	"sync"
	"time"
)

// playbackMeter holds bounded scalar output levels, never audio. Only successfully
// submitted hardware periods are scheduled, at ALSA's reported playback delay.
// Reset generations reject late submissions racing interruption/stream drain.
type levelPeriod struct {
	at  time.Time
	rms float64
}
type playbackMeter struct {
	mu          sync.Mutex
	generation  uint64
	periods     [64]levelPeriod
	head, count int
	current     levelPeriod
}

func (m *playbackMeter) epoch() uint64 { m.mu.Lock(); defer m.mu.Unlock(); return m.generation }
func (m *playbackMeter) reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.generation++
	m.head, m.count = 0, 0
	m.current = levelPeriod{}
}
func (m *playbackMeter) offer(gen uint64, at time.Time, rms float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if gen != m.generation {
		return
	}
	if m.count >= len(m.periods) {
		m.head, m.count = 0, 0
		m.current = levelPeriod{}
		return
	}
	if !math.IsNaN(rms) && !math.IsInf(rms, 0) {
		m.periods[(m.head+m.count)%len(m.periods)] = levelPeriod{at, rms}
		m.count++
	}
}
func (m *playbackMeter) level(now time.Time) float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	for m.count > 0 && !m.periods[m.head].at.After(now) {
		m.current = m.periods[m.head]
		m.head = (m.head + 1) % len(m.periods)
		m.count--
	}
	if m.current.at.IsZero() || now.Sub(m.current.at) > 2*period*time.Second/Rate {
		return 0
	}
	// Amplitude envelope, not phonemes. Gain/mute was applied before measurement.
	rms := m.current.rms
	if rms <= .00012 {
		return 0
	}
	return math.Min(1, math.Log(rms/.00012)/math.Log(100))
}
func (p *Player) VoiceLevel() float64 { return p.voiceMeter.level(time.Now()) }
