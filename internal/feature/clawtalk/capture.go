//go:build !board_biscuit

package clawtalk

import (
	"context"
	"github.com/ygelfand/libcountertop/pkg/media/rtc"
	"time"
)

type captureHooks struct {
	muted, ready      func() bool
	captured, dropped func()
}

// Keep the media clock running with synthetic silence when hardware is cut,
// even if its capture driver stops publishing frames. Never buffer microphone
// input across a mute transition; output channels are deliberately unbuffered.
func runCapture(ctx context.Context, source <-chan []int16, out chan<- []int16, h captureHooks) {
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	wasMuted := h.muted()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			on := h.muted()
			if on {
				wasMuted = true
			}
			if on && h.ready() {
				select {
				case out <- make([]int16, rtc.FrameSamples):
				default:
				}
			}
		case f, ok := <-source:
			if !ok {
				return
			}
			on := h.muted()
			if on {
				wasMuted = true
				continue
			}
			if wasMuted {
				// Drop every frame captured before the release was observed.
				for draining := true; draining; {
					select {
					case _, ok := <-source:
						if !ok {
							return
						}
					default:
						draining = false
					}
				}
				wasMuted = false
				continue
			}
			if !h.ready() {
				continue
			}
			select {
			case out <- f:
				h.captured()
			default:
				h.dropped()
			}
		}
	}
}
