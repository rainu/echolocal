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

func TestEchoTrialsExposeTailOutsideShortFilter(t *testing.T) {
	ref, in := make([]int16, 6*16000), make([]int16, 6*16000)
	rng := rand.New(rand.NewPCG(59, 2))
	for i := range ref {
		ref[i] = int16(rng.IntN(16000) - 8000)
		if i >= 13 {
			in[i] += ref[i-13] / 3
		}
		if i >= 1800 {
			in[i] += ref[i-1800] / 4
		}
	}
	short, _, err := echoTrial(in, ref, 1024)
	if err != nil {
		t.Fatal(err)
	}
	long, _, err := echoTrial(in, ref, 4096)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("short %.1f long %.1f", short, long)
	if long < short+6 {
		t.Fatalf("long tail not resolved: short %.1f long %.1f", short, long)
	}
}
