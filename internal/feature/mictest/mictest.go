package mictest

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ygelfand/echolocal/internal/component"
	"github.com/ygelfand/echolocal/internal/config"
	"github.com/ygelfand/echolocal/internal/feature/mute"
	"github.com/ygelfand/echolocal/internal/hardware/mic"
	"github.com/ygelfand/echolocal/internal/hardware/speaker"
	core "github.com/ygelfand/echolocal/internal/mictest"
	"github.com/ygelfand/libcountertop/pkg/hook"
	sharedcomponent "github.com/ygelfand/libcountertop/pkg/runtime/component"
)

var Changed hook.Hook[struct{}]
var busyChecks []func() bool

// PauseWhen callbacks are registered during initialization, never while running.
func PauseWhen(f func() bool) { busyChecks = append(busyChecks, f) }
func busy() bool {
	for _, f := range busyChecks {
		if f() {
			return true
		}
	}
	return false
}
func Active() bool { return Get().Active() }

var Get = sync.OnceValue(func() *core.Monitor {
	return core.New(core.Options{
		Listen: func() (<-chan []int16, func()) { return mic.Get().Listen("microphone test (processed)") },
		Check: func() error {
			on, err := mute.Get().Muted()
			if err != nil {
				return errors.New("mute_unavailable")
			}
			if on {
				return errors.New("muted")
			}
			if !mic.Get().Startup().Done {
				return errors.New("capture_unavailable")
			}
			return nil
		},
		Busy:       busy,
		OutputBusy: func() bool { return speaker.Sound().Busy() || speaker.Get().Queued() > 0 },
		Processing: func() string {
			c := config.Get().Microphone
			return fmt.Sprintf("16 kHz mono · %s · gain %d dB · leveling %t · AEC %t · sensitivity %d dB", c.Mixing, c.Gain, c.Leveling, c.Cancel, c.Sensitivity)
		},
		Play:    play,
		Changed: func() { Changed.Emit(struct{}{}) },
	})
})

func init() { component.Register(sharedcomponent.Device, Get, sharedcomponent.Order(34)) }
func play(ctx context.Context, samples []int16) error {
	p := speaker.Get()
	if !p.Startup().Done {
		return errors.New("speaker_unavailable")
	}
	// No live monitoring: the capture subscription was closed before this call.
	// Use the ordinary voice resampler/output path, but never retain an audio file.
	claim := speaker.Sound().Claim("microphone test", func(claimCtx context.Context, p *speaker.Player) error {
		lane := p.Lane(config.StreamVoice)
		defer lane.Drain()
		tick := time.NewTicker(20 * time.Millisecond)
		defer tick.Stop()
		for at := 0; at < len(samples); {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-claimCtx.Done():
				return claimCtx.Err()
			case <-tick.C:
			}
			if lane.Queued() > speaker.Rate/5 {
				continue
			}
			end := min(at+mic.FrameSamples, len(samples))
			p.PlayVoice(samples[at:end])
			at = end
		}
		deadline := time.NewTimer(3 * time.Second)
		defer deadline.Stop()
		for lane.Queued() > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-claimCtx.Done():
				return claimCtx.Err()
			case <-deadline.C:
				return errors.New("playback_timeout")
			case <-tick.C:
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-claimCtx.Done():
			return claimCtx.Err()
		case <-time.After(p.Latency() + 200*time.Millisecond):
			return nil
		}
	})
	<-claim.Done()
	// Failure/cancellation paths do not run the sound driver success drain.
	// Keep the capture/wake gate shut through the hardware tail in every case.
	time.Sleep(p.Latency() + 200*time.Millisecond)
	if claim.Stopped() && ctx.Err() == nil {
		return errors.New("playback_interrupted")
	}
	return claim.Err()
}
