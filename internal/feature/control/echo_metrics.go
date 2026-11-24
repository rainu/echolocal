package control

import (
	"github.com/ygelfand/echolocal/internal/hardware/mic"
	"github.com/ygelfand/echolocal/internal/lib/audio"
)

// Analyze only a bounded one-second window in memory. No audio is persisted.
// Positive lag means the microphone trails the reference; negative is acausal
// for a conventional echo filter and indicates an alignment problem.
func echoLag(m, r []int16) (int, float64) {
	n := min(len(m), len(r), mic.Rate)
	if n < 2 {
		return 0, 0
	}
	a, b := make([]float64, n), make([]float64, n)
	for i := range n {
		a[i], b[i] = float64(r[i]), float64(m[i])
	}
	return audio.BestLag(a, b, min(n/2, mic.Rate/10))
}
