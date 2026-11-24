package health

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

const LoadSampleInterval = 5 * time.Second
const loadMaxGap = 30 * time.Second

// CPULoad is sampled CPU demand, not Linux loadavg and not CPU utilization.
// Values count running/runnable tasks (R), excluding this sampling thread.
// D-state tasks are never included.
type CPULoad struct {
	Available        bool      `json:"available"`
	Source           string    `json:"source"`
	Averages         []float64 `json:"averages,omitempty"`
	Runnable         int       `json:"runnable"`
	SampledAtMS      int64     `json:"sampledAtMs"`
	ObservedMS       int64     `json:"observedMs"`
	SampleIntervalMS int64     `json:"sampleIntervalMs"`
	WarmingUp        bool      `json:"warmingUp"`
}
type cpuLoad struct {
	values      [3]float64
	first, last time.Time
	runnable    int
	valid       bool
}

// ParseRunnable uses the kernel's system-wide scheduler counter; it does not
// enumerate tasks or inspect command lines. Discount one observer thread.
func ParseRunnable(stat string) (int, error) {
	found := false
	count := 0
	for _, line := range strings.Split(stat, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 || f[0] != "procs_running" {
			continue
		}
		if found || len(f) != 2 {
			return 0, fmt.Errorf("runnable_unavailable")
		}
		n, e := strconv.ParseUint(f[1], 10, 32)
		if e != nil || n > 100001 {
			return 0, fmt.Errorf("runnable_unavailable")
		}
		count = max(0, int(n)-1)
		found = true
	}
	if !found {
		return 0, fmt.Errorf("runnable_unavailable")
	}
	return count, nil
}
func (l *cpuLoad) observe(at time.Time, runnable int) {
	if runnable < 0 || runnable > 100000 {
		*l = cpuLoad{}
		return
	}
	if !l.valid || at.Before(l.last) || at.Sub(l.last) > loadMaxGap {
		*l = cpuLoad{values: [3]float64{float64(runnable), float64(runnable), float64(runnable)}, first: at, last: at, runnable: runnable, valid: true}
		return
	}
	dt := at.Sub(l.last).Seconds()
	for i, tau := range [...]float64{60, 300, 900} {
		// Use elapsed monotonic time, not an assumed fixed tick count.
		weight := -math.Expm1(-dt / tau)
		l.values[i] += (float64(runnable) - l.values[i]) * weight
	}
	l.last = at
	l.runnable = runnable
}
func (l *cpuLoad) snapshot() CPULoad {
	s := CPULoad{Available: l.valid, Source: "cpu_runnable_ewma_excluding_observer", SampleIntervalMS: LoadSampleInterval.Milliseconds()}
	if !l.valid {
		return s
	}
	s.Averages = append([]float64(nil), l.values[:]...)
	s.Runnable = l.runnable
	s.SampledAtMS = l.last.UnixMilli()
	s.ObservedMS = l.last.Sub(l.first).Milliseconds()
	s.WarmingUp = l.last.Sub(l.first) < 15*time.Minute
	return s
}
func (c *Collector) sampleLoad(ctxErr error, at time.Time) {
	if ctxErr != nil {
		return
	}
	if !c.loadAttempt.IsZero() && at.Sub(c.loadAttempt) < LoadSampleInterval {
		return
	}
	c.loadAttempt = at
	data, err := c.readStat()
	if err != nil {
		c.load = cpuLoad{}
		return
	}
	runnable, err := ParseRunnable(string(data))
	if err != nil {
		c.load = cpuLoad{}
		return
	}
	c.load.observe(at, runnable)
}
func (c *Collector) withLoad(s Snapshot) Snapshot {
	s.CPULoad = c.load.snapshot()
	return s
}
