package health

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

func TestRunnableCounterAndObserver(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  int
	}{{"cpu 1 2 3 4\nprocs_running 5\nprocs_blocked 17\n", 4}, {"procs_running 1", 0}, {"procs_running 0", 0}, {"procs_running 100001", 100000}} {
		got, e := ParseRunnable(tc.input)
		if e != nil || got != tc.want {
			t.Fatal(tc, got, e)
		}
	}
	for _, input := range []string{"", "procs_blocked 17", "procs_running -1", "procs_running x", "procs_running 100002", "procs_running 1 2", "procs_running 1\nprocs_running 2"} {
		if _, e := ParseRunnable(input); e == nil {
			t.Fatal("accepted invalid counter", input)
		}
	}
}
func TestCPULoadEWMAUsesElapsedTime(t *testing.T) {
	at := time.Unix(1000, 0)
	var l cpuLoad
	l.observe(at, 2)
	l.observe(at.Add(5*time.Second), 10)
	for i, tau := range []float64{60, 300, 900} {
		want := 10 + (2-10)*math.Exp(-5/tau)
		if math.Abs(l.values[i]-want) > 1e-12 {
			t.Fatal(i, l.values[i], want)
		}
	}
	s := l.snapshot()
	if !s.Available || !s.WarmingUp || s.ObservedMS != 5000 || s.Runnable != 10 {
		t.Fatal(s)
	}
	s.Averages[0] = 999
	if l.values[0] == 999 {
		t.Fatal("snapshot aliases mutable state")
	}
}
func TestCPULoadConvergenceRecoveryAndGaps(t *testing.T) {
	at := time.Unix(1000, 0)
	var l cpuLoad
	l.observe(at, 8)
	for i := 1; i <= 180; i++ {
		l.observe(at.Add(time.Duration(i)*5*time.Second), 8)
	}
	s := l.snapshot()
	if s.WarmingUp || s.Averages[0] != 8 || s.Averages[1] != 8 || s.Averages[2] != 8 {
		t.Fatal(s)
	}
	l.observe(at.Add(905*time.Second), 0)
	if l.values[0] >= 8 || l.values[0] <= 0 {
		t.Fatal(l)
	}
	l.observe(at.Add(940*time.Second), 3)
	if l.values != [3]float64{3, 3, 3} || !l.snapshot().WarmingUp {
		t.Fatal("gap must reseed", l)
	}
	l.observe(at, 4)
	if l.values != [3]float64{4, 4, 4} {
		t.Fatal("clock reset", l)
	}
	l.observe(at, 100001)
	if l.snapshot().Available {
		t.Fatal("invalid must not become healthy zero")
	}
}
func TestCPULoadSamplingRateErrorsAndReset(t *testing.T) {
	var reads int
	fail := false
	c := Collector{readStat: func() ([]byte, error) {
		reads++
		if fail {
			return nil, errors.New("unavailable")
		}
		return []byte("procs_running 4\nprocs_blocked 99"), nil
	}}
	at := time.Now()
	c.sampleLoad(nil, at)
	for i := 1; i < 5; i++ {
		c.sampleLoad(nil, at.Add(time.Duration(i)*time.Second))
	}
	if reads != 1 || c.load.values != [3]float64{3, 3, 3} {
		t.Fatal(reads, c.load)
	}
	c.sampleLoad(context.Canceled, at.Add(5*time.Second))
	if reads != 1 {
		t.Fatal("cancelled sampler read")
	}
	fail = true
	c.sampleLoad(nil, at.Add(5*time.Second))
	if reads != 2 || c.load.snapshot().Available {
		t.Fatal(reads, c.load)
	}
	fail = false
	c.sampleLoad(nil, at.Add(10*time.Second))
	if !c.load.snapshot().Available {
		t.Fatal("did not recover")
	}
	c.Reset()
	if c.load.snapshot().Available || !c.loadAttempt.IsZero() {
		t.Fatal("reset retained history")
	}
}
func TestHostStatsUsesCPULoadNotKernelLoad(t *testing.T) {
	s := Snapshot{Memory: Memory{Available: true, TotalBytes: 100, AvailableBytes: 50}, CPU: CPU{Count: 8}, LoadAverage: []float64{17, 18, 19}}
	if _, ok := s.HostStats()["loadAverage"]; ok {
		t.Fatal("kernel load must not leak into UI")
	}
	s.CPULoad = CPULoad{Available: true, Averages: []float64{2, 3, 4}}
	p := s.HostStats()["loadAverage"].([]float64)
	if p[0] != 2 || s.LoadAverage[0] != 17 {
		t.Fatal(s, p)
	}
	s.CPULoad.Available = false
	if _, ok := s.HostStats()["loadAverage"]; ok {
		t.Fatal("unavailable must be omitted")
	}
}
