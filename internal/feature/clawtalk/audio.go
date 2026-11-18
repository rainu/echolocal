//go:build !board_biscuit

package clawtalk

import (
	"context"
	"errors"
	"github.com/ygelfand/echolocal/internal/config"
	"github.com/ygelfand/echolocal/internal/feature/mute"
	"github.com/ygelfand/echolocal/internal/hardware/mic"
	"github.com/ygelfand/echolocal/internal/hardware/speaker"
	"github.com/ygelfand/echolocal/internal/openclaw/talk"
	"github.com/ygelfand/libcountertop/pkg/audio/tone"
	"sync"
	"sync/atomic"
	"time"
)

type deviceAudio struct {
	playMu                           sync.Mutex
	playout                          talk.PlayoutBuffer
	playDone                         chan struct{}
	blocked                          bool
	held                             []int16
	lastSpeech, lastPacket           time.Time
	decoded, silenceSkipped          atomic.Uint64
	maxReceiveGapMS, maxPlayoutGapMS atomic.Int64
	underrunBase                     atomic.Uint64

	ctx                                context.Context
	cancel                             context.CancelFunc
	start                              chan struct{}
	done                               chan struct{}
	frames                             chan []int16
	once                               sync.Once
	source                             <-chan []int16
	release                            func()
	captured, drops, played, overflows atomic.Uint64
	onPlay                             func()
	onMute                             func()
}

func newAudio(parent context.Context, onMute func()) *deviceAudio {
	ctx, cancel := context.WithCancel(parent)
	a := &deviceAudio{ctx: ctx, cancel: cancel, start: make(chan struct{}), done: make(chan struct{}), frames: make(chan []int16, 8), playDone: make(chan struct{}), onMute: onMute}
	go a.pump()
	go a.playoutPump()
	return a
}
func (a *deviceAudio) Start(ctx context.Context) error {
	if !config.Get().Microphone.Cancel || !speaker.Get().Startup().Done || !mic.Get().Startup().Done {
		return errors.New("talk_audio_unavailable")
	}
	muted, e := mute.Get().Muted()
	if e != nil || muted {
		return errors.New("talk_muted")
	}
	a.source, a.release = mic.Get().Listen("openclaw-talk")
	select {
	case _, ok := <-a.source:
		if !ok {
			a.release()
			return errors.New("talk_microphone")
		}
	case <-ctx.Done():
		a.release()
		return ctx.Err()
	}
	a.underrunBase.Store(speaker.Get().Underruns())
	close(a.start)
	return nil
}
func (a *deviceAudio) pump() {
	defer close(a.done)
	defer close(a.frames)
	select {
	case <-a.ctx.Done():
		return
	case <-a.start:
	}
	defer a.release()
	for {
		select {
		case <-a.ctx.Done():
			return
		case f, ok := <-a.source:
			if !ok {
				return
			}
			muted, e := mute.Get().Muted()
			if e != nil || muted {
				if a.onMute != nil {
					go a.onMute()
				}
				return
			}
			a.captured.Add(1)
			select {
			case a.frames <- f:
			default:
				a.drops.Add(1)
			}
		}
	}
}
func (a *deviceAudio) Frames() (<-chan []int16, func()) { return a.frames, func() {} }
func (a *deviceAudio) Play(pcm []int16) {
	if a.ctx.Err() != nil {
		return
	}
	now := time.Now()
	audible := talk.Audible(pcm)
	a.decoded.Add(1)
	a.playMu.Lock()
	if a.ctx.Err() != nil {
		a.playMu.Unlock()
		return
	}
	if !a.lastPacket.IsZero() {
		recordMax(&a.maxReceiveGapMS, now.Sub(a.lastPacket).Milliseconds())
	}
	a.lastPacket = now
	if a.blocked {
		// Keep only 80 ms of the most recent media to absorb data-channel/RTP
		// ordering skew when the native agent result is acknowledged.
		a.held = append(a.held, pcm...)
		max := speaker.Rate * 80 / 1000 * speaker.Channels
		if len(a.held) > max {
			copy(a.held, a.held[len(a.held)-max:])
			a.held = a.held[:max]
		}
		a.playMu.Unlock()
		return
	}
	if audible {
		a.lastSpeech = now
	} else if a.lastSpeech.IsZero() || now.Sub(a.lastSpeech) > 400*time.Millisecond {
		a.silenceSkipped.Add(1)
		a.playMu.Unlock()
		return
	}
	a.playout.Push(pcm)
	a.playMu.Unlock()
	if audible && a.onPlay != nil {
		a.onPlay()
	}
}
func (a *deviceAudio) playoutPump() {
	defer close(a.playDone)
	select {
	case <-a.ctx.Done():
		return
	case <-a.start:
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	previous := time.Now()
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			recordMax(&a.maxPlayoutGapMS, now.Sub(previous).Milliseconds())
			previous = now
			a.playMu.Lock()
			if a.ctx.Err() == nil && !a.blocked {
				lane := speaker.Get().Lane(config.StreamVoice)
				if pcm := a.playout.Take(lane.Queued()); len(pcm) > 0 {
					lane.Play(pcm)
					a.played.Add(1)
				}
			}
			a.playMu.Unlock()
		}
	}
}
func (a *deviceAudio) Queued() int {
	a.playMu.Lock()
	defer a.playMu.Unlock()
	return speaker.Get().Lane(config.StreamVoice).Queued() + a.playout.Queued()
}
func (a *deviceAudio) StopOutput() {
	a.playMu.Lock()
	defer a.playMu.Unlock()
	a.playout.Reset()
	clear(a.held)
	a.held = nil
	a.lastSpeech = time.Time{}
	speaker.Get().Lane(config.StreamVoice).Drain()
}
func (a *deviceAudio) BlockOutput(block bool) {
	a.playMu.Lock()
	audible := false
	if block {
		a.blocked = true
		a.playout.Reset()
		clear(a.held)
		a.held = nil
		a.lastSpeech = time.Time{}
		speaker.Get().Lane(config.StreamVoice).Drain()
	} else if a.blocked {
		a.blocked = false
		audible = talk.Audible(a.held)
		if audible {
			a.lastSpeech = time.Now()
		}
		a.playout.Push(a.held)
		clear(a.held)
		a.held = nil
	}
	a.playMu.Unlock()
	if audible && a.onPlay != nil {
		a.onPlay()
	}
}
func recordMax(v *atomic.Int64, n int64) {
	for old := v.Load(); n > old; old = v.Load() {
		if v.CompareAndSwap(old, n) {
			return
		}
	}
}
func (a *deviceAudio) ReadyTone() {
	speaker.Sound().Chime(config.StreamFeedback, []tone.Note{{Freq: 880, Ms: 70}, {Freq: 1320, Ms: 55}})
}
func (a *deviceAudio) Close() error {
	a.once.Do(func() {
		a.cancel()
		select {
		case <-a.done:
		case <-time.After(time.Second):
		}
		select {
		case <-a.playDone:
		case <-time.After(time.Second):
		}
		a.StopOutput()
	})
	return nil
}
