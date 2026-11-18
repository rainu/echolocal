//go:build !board_biscuit

package clawtalk

import (
	"context"
	"errors"
	"github.com/ygelfand/echolocal/internal/config"
	"github.com/ygelfand/echolocal/internal/feature/mute"
	"github.com/ygelfand/echolocal/internal/hardware/mic"
	"github.com/ygelfand/echolocal/internal/hardware/speaker"
	"github.com/ygelfand/libcountertop/pkg/audio/tone"
	"sync"
	"sync/atomic"
	"time"
)

type deviceAudio struct {
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
	a := &deviceAudio{ctx: ctx, cancel: cancel, start: make(chan struct{}), done: make(chan struct{}), frames: make(chan []int16, 8), onMute: onMute}
	go a.pump()
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
	lane := speaker.Get().Lane(config.StreamVoice)
	// Half a second of decoded audio is the hard playback budget.
	if lane.Queued() > speaker.Rate/2 {
		lane.Drain()
		a.overflows.Add(1)
	}
	if a.onPlay != nil {
		a.onPlay()
	}
	lane.Play(pcm)
	a.played.Add(1)
}
func (a *deviceAudio) Queued() int { return speaker.Get().Lane(config.StreamVoice).Queued() }
func (a *deviceAudio) StopOutput() { speaker.Get().Lane(config.StreamVoice).Drain() }
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
		a.StopOutput()
	})
	return nil
}
