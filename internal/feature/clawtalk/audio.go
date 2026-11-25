//go:build !board_biscuit

package clawtalk

import (
	"context"
	"errors"
	"github.com/ygelfand/echolocal/internal/config"
	"github.com/ygelfand/echolocal/internal/feature/mute"
	"github.com/ygelfand/echolocal/internal/feature/tones"
	"github.com/ygelfand/echolocal/internal/hardware/mic"
	"github.com/ygelfand/echolocal/internal/hardware/speaker"
	"github.com/ygelfand/echolocal/internal/openclaw/talk"
	"github.com/ygelfand/libcountertop/pkg/audio/tone"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

type deviceAudio struct {
	startTone, endTone               string
	feedback                         *setupFeedback
	ready                            atomic.Bool
	lastCaptureMS                    atomic.Int64
	playMu                           sync.Mutex
	playout                          talk.PlayoutBuffer
	playDone                         chan struct{}
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
	onMute                             func(bool)
	inputMuted                         atomic.Bool
}

func newAudio(parent context.Context, onMute func(bool)) *deviceAudio {
	ctx, cancel := context.WithCancel(parent)
	a := &deviceAudio{ctx: ctx, cancel: cancel, start: make(chan struct{}), done: make(chan struct{}), frames: make(chan []int16), playDone: make(chan struct{}), onMute: onMute}
	go a.pump()
	go a.playoutPump()
	return a
}
func (a *deviceAudio) Start(ctx context.Context) error {
	if !config.Get().Microphone.Cancel || !speaker.Get().Startup().Done || !mic.Get().Startup().Done {
		return errors.New("talk_audio_unavailable")
	}
	muted, e := mute.Get().Muted()
	if e != nil {
		return errors.New("talk_muted")
	}
	a.inputMuted.Store(muted)
	if a.onMute != nil {
		a.onMute(muted)
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
	runCapture(a.ctx, a.source, a.frames, captureHooks{
		muted: func() bool {
			muted, err := mute.Get().Muted()
			on := err != nil || muted // Unknown hardware state is fail-closed.
			if a.inputMuted.Swap(on) != on && a.onMute != nil {
				a.onMute(on)
			}
			return on
		},
		ready: func() bool {
			return a.ready.Load() && !speaker.Get().Heard(config.StreamFeedback, 250*time.Millisecond)
		},
		captured: func() { a.captured.Add(1); a.lastCaptureMS.Store(time.Now().UnixMilli()) },
		dropped:  func() { a.drops.Add(1) },
	})
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
		gap := now.Sub(a.lastPacket).Milliseconds()
		recordMax(&a.maxReceiveGapMS, gap)
		// Learn only a mid-speech stall, not ordinary idle/comfort-noise gaps.
		if audible && !a.lastSpeech.IsZero() && now.Sub(a.lastSpeech) < time.Second {
			a.playout.ObserveGap(int(gap))
		}
	}
	a.lastPacket = now
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
			if !a.ready.Load() {
				continue
			}
			now := time.Now()
			recordMax(&a.maxPlayoutGapMS, now.Sub(previous).Milliseconds())
			previous = now
			a.playMu.Lock()
			if a.ctx.Err() == nil {
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
	a.lastSpeech = time.Time{}
	speaker.Get().Lane(config.StreamVoice).Drain()
}
func recordMax(v *atomic.Int64, n int64) {
	for old := v.Load(); n > old; old = v.Load() {
		if v.CompareAndSwap(old, n) {
			return
		}
	}
}
func playTalkTone(choice string, end bool) {
	if choice == "" {
		speaker.Sound().Chime(config.StreamFeedback, DefaultNotes(end))
		return
	}
	speaker.Sound().Sound(config.StreamFeedback, tone.Level, tones.Get().Sound(schema.Chime(choice), schema.ChimeChirp))
}
func (a *deviceAudio) ReadyTone(ctx context.Context) error {
	if a.feedback != nil {
		if a.feedback.Ready(ctx) {
			a.ready.Store(true)
			slog.Info("talk feedback capture gate opened")
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("talk_feedback_unavailable")
	}
	playTalkTone(a.startTone, false)
	a.ready.Store(true)
	return nil
}
func (a *deviceAudio) Close() error {
	a.once.Do(func() {
		a.cancel()
		if a.feedback != nil {
			a.feedback.Stop()
		}
		select {
		case <-a.done:
		case <-time.After(time.Second):
		}
		select {
		case <-a.playDone:
		case <-time.After(time.Second):
		}
		a.StopOutput()
		if a.ready.Load() {
			playTalkTone(a.endTone, true)
		}
	})
	return nil
}

// Preview shares exactly the ready/end playback path, never capture or routing.
// Keep activation serialized until the short sound is enqueued. Never preempt
// an active conversation merely to audition a setting.
func (m *Manager) PreviewTone(choice string, end bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active {
		return
	}
	playTalkTone(choice, end)
}

// Queued PCM is used for setup cues so cancellation can remove pending samples.
// Append rather than overlay: separate cues must not be mixed together.
type deviceFeedback struct{ readyChoice, progressChoice string }

func (d *deviceFeedback) Cue(cue string) {
	slog.Info("talk feedback cue", "cue", cue)
	choice := ""
	if cue == "ready" {
		choice = d.readyChoice
	}
	if cue == "progress" {
		choice = d.progressChoice
	}
	pcm := tonePCM(choice, cue)
	speaker.Get().Lane(config.StreamFeedback).Play(pcm)
}

// Preview uses the same serial PCM path and selection as live setup.
func (m *Manager) PreviewProgressTone(choice string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active {
		return
	}
	(&deviceFeedback{progressChoice: choice}).Cue("progress")
}
func (*deviceFeedback) Clear() { speaker.Get().Lane(config.StreamFeedback).Drain() }
func (*deviceFeedback) Wait(ctx context.Context) bool {
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for speaker.Get().Lane(config.StreamFeedback).Queued() > 0 {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
		}
	}
	// Account for already submitted hardware frames and room/microphone tail.
	timer := time.NewTimer(speaker.Get().Latency() + 250*time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
