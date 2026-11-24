package control

import (
	"github.com/ygelfand/echolocal/internal/hardware/mic"
	"github.com/ygelfand/echolocal/internal/lib/audio"
	"github.com/ygelfand/libcountertop/pkg/audio/aec"
	"math"
)

// Analyze only a bounded one-second window in memory. No audio is persisted.
// Positive lag means the microphone trails the reference; negative is acausal
// for a conventional echo filter and indicates an alignment problem.
func echoLag(m, r []int16) (int, float64) {
	total := min(len(m), len(r))
	if total < 2 {
		return 0, 0
	}
	ref := make([]float64, total)
	for i := range ref {
		ref[i] = float64(r[i])
	}
	n := min(total, mic.Rate)
	start := audio.LoudestWindow(ref, n)
	a, b := ref[start:start+n], make([]float64, n)
	var power float64
	for i := range n {
		b[i] = float64(m[start+i])
		power += a[i] * a[i]
	}
	if power == 0 {
		return 0, 0
	}
	return audio.BestLag(a, b, min(n/2, mic.Rate/10))
}

// Dry-run fresh filters on the same transient samples, without changing the live
// filter or saving PCM. Compare the last half, excluding cold-start convergence.
func echoTrial(m, r []int16, taps int) (float64, float64, error) {
	c, err := aec.NewCanceller(1, taps, mic.Rate)
	if err != nil {
		return 0, 0, err
	}
	p, err := aec.NewSuppressor(mic.Rate, c.Filter(0))
	if err != nil {
		return 0, 0, err
	}
	var input, residual, suppressed float64
	out := make([]int16, mic.FrameSamples)
	total := min(len(m), len(r))
	for at := 0; at+len(out) <= total; at += len(out) {
		if err := c.Process(r[at:at+len(out)], [][]int16{m[at : at+len(out)]}, [][]int16{out}, func(_ int, block []int16) {
			if at >= total/2 {
				for _, v := range block {
					residual += float64(v) * float64(v)
				}
			}
			_ = p.Run(block)
		}); err != nil {
			return 0, 0, err
		}
		if at >= total/2 {
			for i, v := range out {
				x := float64(m[at+i])
				input += x * x
				suppressed += float64(v) * float64(v)
			}
		}
	}
	db := func(e float64) float64 { return 10 * math.Log10((input+1)/(e+1)) }
	return db(residual), db(suppressed), nil
}
