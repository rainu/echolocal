package control

import (
	"math/rand/v2"
	"testing"
)

func TestEchoLagSignAndSilence(t *testing.T) {
	for _, delay := range []int{-183, 0, 217} {
		r, m := make([]int16, 8000), make([]int16, 8000)
		rng := rand.New(rand.NewPCG(59, 1))
		for i := range r {
			r[i] = int16(rng.IntN(10000) - 5000)
		}
		for i := range m {
			if j := i - delay; j >= 0 && j < len(r) {
				m[i] = r[j] / 2
			}
		}
		lag, corr := echoLag(m, r)
		if lag != delay || corr < .99 {
			t.Fatalf("delay=%d got=%d corr=%f", delay, lag, corr)
		}
	}
	if _, corr := echoLag(make([]int16, 8000), make([]int16, 8000)); corr != 0 {
		t.Fatal(corr)
	}
}
