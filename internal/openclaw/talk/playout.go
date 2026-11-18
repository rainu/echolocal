package talk

// PlayoutBuffer smooths packet-arrival jitter before the existing device lane.
// All sizes are interleaved stereo samples. The caller serializes access.
type PlayoutBuffer struct {
	samples            []int16
	started            bool
	waitTicks          int
	reserveFrames      int
	Underruns, Dropped uint64
	PeakFrames         int
}

const (
	PlayoutChannels = 2
	PlayoutRate     = 48000
	prefillFrames   = PlayoutRate * 120 / 1000
	laneFrames      = PlayoutRate * 100 / 1000
	maxBufferFrames = PlayoutRate * 600 / 1000
)

func (b *PlayoutBuffer) Push(pcm []int16) {
	n := len(pcm) - len(pcm)%PlayoutChannels
	b.samples = append(b.samples, pcm[:n]...)
	max := maxBufferFrames * PlayoutChannels
	if len(b.samples) > max {
		drop := len(b.samples) - max
		b.Dropped += uint64(drop / PlayoutChannels)
		copy(b.samples, b.samples[drop:])
		b.samples = b.samples[:max]
	}
	b.PeakFrames = maxInt(b.PeakFrames, len(b.samples)/PlayoutChannels)
}
func (b *PlayoutBuffer) Take(queuedFrames int) []int16 {
	frames := len(b.samples) / PlayoutChannels
	if !b.started {
		if frames == 0 {
			return nil
		}
		b.waitTicks++
		reserve := maxInt(prefillFrames, b.reserveFrames)
		if frames < reserve && b.waitTicks < reserve/(PlayoutRate/100) {
			return nil
		}
		b.waitTicks = 0
		b.started = true
	}
	if queuedFrames == 0 && frames == 0 {
		b.Underruns++
		b.started = false
		b.waitTicks = 0
		return nil
	}
	need := laneFrames - queuedFrames
	if need <= 0 || frames == 0 {
		return nil
	}
	if need > frames {
		need = frames
	}
	n := need * PlayoutChannels
	out := append([]int16(nil), b.samples[:n]...)
	copy(b.samples, b.samples[n:])
	b.samples = b.samples[:len(b.samples)-n]
	return out
}
func (b *PlayoutBuffer) Queued() int { return len(b.samples) / PlayoutChannels }
func (b *PlayoutBuffer) Reset() {
	clear(b.samples)
	b.samples = nil
	b.started = false
	b.waitTicks = 0
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Audible rejects provider comfort/silence packets, not normal speech. RTP
// continues through quiet periods; counting every packet as speech prevents
// truthful states and inactivity expiry.
func Audible(pcm []int16) bool {
	if len(pcm) == 0 {
		return false
	}
	var energy int64
	for _, sample := range pcm {
		v := int64(sample)
		energy += v * v
	}
	return energy > int64(len(pcm))*16*16
}

// ObserveGap learns a bounded reserve from actual packet-arrival stalls.
// Healthy connections keep the 120ms startup budget. A stalled stream uses
// up to 400ms on the next rebuffer, never pauses an already playing queue.
func (b *PlayoutBuffer) ObserveGap(milliseconds int) {
	if milliseconds < 60 {
		return
	}
	milliseconds = min(400, milliseconds+20)
	b.reserveFrames = maxInt(b.reserveFrames, PlayoutRate*milliseconds/1000)
}
