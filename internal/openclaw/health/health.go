// Package health collects bounded, device-wide read-only Linux/Android metrics.
package health

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Memory struct {
	Available      bool   `json:"available"`
	TotalBytes     uint64 `json:"totalBytes,omitempty"`
	AvailableBytes uint64 `json:"availableBytes,omitempty"`
	UsedBytes      uint64 `json:"usedBytes,omitempty"`
}
type Disk struct {
	Available      bool   `json:"available"`
	Path           string `json:"path"`
	TotalBytes     uint64 `json:"totalBytes,omitempty"`
	AvailableBytes uint64 `json:"availableBytes,omitempty"`
	UsedBytes      uint64 `json:"usedBytes,omitempty"`
}
type CPU struct {
	Available  bool     `json:"available"`
	Percent    *float64 `json:"percent,omitempty"`
	IntervalMS int64    `json:"intervalMs"`
	Count      int      `json:"count"`
}
type Snapshot struct {
	MeasuredAtMS int64     `json:"measuredAtMs"`
	CPU          CPU       `json:"cpu"`
	Memory       Memory    `json:"memory"`
	Disk         Disk      `json:"disk"`
	LoadAverage  []float64 `json:"loadAverage,omitempty"`
}
type Collector struct {
	mu   sync.Mutex
	last Snapshot
}

func ParseCPU(data string) (total, idle uint64, err error) {
	fields := strings.Fields(strings.SplitN(data, "\n", 2)[0])
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, 0, fmt.Errorf("cpu_unavailable")
	}
	// guest and guest_nice are already included in user/nice.
	for i := 1; i < len(fields) && i <= 8; i++ {
		v, e := strconv.ParseUint(fields[i], 10, 64)
		if e != nil {
			return 0, 0, e
		}
		if v > ^uint64(0)-total {
			return 0, 0, fmt.Errorf("cpu_overflow")
		}
		total += v
		if i == 4 || i == 5 {
			idle += v
		}
	}
	return
}
func Percent(t1, i1, t2, i2 uint64) *float64 {
	if t2 <= t1 || i2 < i1 || i2-i1 > t2-t1 {
		return nil
	}
	v := 100 * float64((t2-t1)-(i2-i1)) / float64(t2-t1)
	return &v
}
func ParseMemory(data string) Memory {
	values := map[string]uint64{}
	for _, line := range strings.Split(data, "\n") {
		f := strings.Fields(line)
		if len(f) != 3 || f[2] != "kB" {
			continue
		}
		v, e := strconv.ParseUint(f[1], 10, 64)
		if e == nil && v <= ^uint64(0)/1024 {
			values[f[0]] = v * 1024
		}
	}
	total, t := values["MemTotal:"]
	avail, a := values["MemAvailable:"]
	if !t || !a || total == 0 || avail > total {
		return Memory{}
	}
	return Memory{true, total, avail, total - avail}
}
func (c *Collector) Reset() { c.mu.Lock(); c.last = Snapshot{}; c.mu.Unlock() }
func (c *Collector) Collect(ctx context.Context) (Snapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ctx.Err() != nil {
		return Snapshot{}, ctx.Err()
	}
	if c.last.MeasuredAtMS != 0 && time.Since(time.UnixMilli(c.last.MeasuredAtMS)) < time.Minute {
		return c.last, nil
	}
	s := Snapshot{CPU: CPU{Count: runtime.NumCPU(), IntervalMS: 1000}, Disk: Disk{Path: "/data"}}
	started := time.Now()
	first, e1 := os.ReadFile("/proc/stat")
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return Snapshot{}, ctx.Err()
	case <-timer.C:
	}
	second, e2 := os.ReadFile("/proc/stat")
	s.CPU.IntervalMS = time.Since(started).Milliseconds()
	if e1 == nil && e2 == nil {
		t1, i1, e := ParseCPU(string(first))
		t2, i2, f := ParseCPU(string(second))
		if e == nil && f == nil {
			s.CPU.Percent = Percent(t1, i1, t2, i2)
			s.CPU.Available = s.CPU.Percent != nil
		}
	}
	if data, e := os.ReadFile("/proc/meminfo"); e == nil {
		s.Memory = ParseMemory(string(data))
	}
	var stat syscall.Statfs_t
	if syscall.Statfs("/data", &stat) == nil && uint32(stat.Type) != 0x01021994 && uint32(stat.Type) != 0x858458f6 && stat.Bsize > 0 && stat.Blocks > 0 && stat.Bavail <= stat.Blocks {
		total, avail := uint64(stat.Blocks)*uint64(stat.Bsize), uint64(stat.Bavail)*uint64(stat.Bsize)
		s.Disk = Disk{true, "/data", total, avail, total - avail}
	}
	if data, e := os.ReadFile("/proc/loadavg"); e == nil {
		f := strings.Fields(string(data))
		if len(f) >= 3 {
			v := []float64{}
			for _, x := range f[:3] {
				n, e := strconv.ParseFloat(x, 64)
				if e != nil || n < 0 || n > 100000 {
					v = nil
					break
				}
				v = append(v, n)
			}
			s.LoadAverage = v
		}
	}
	s.MeasuredAtMS = time.Now().UnixMilli()
	if ctx.Err() != nil {
		return Snapshot{}, ctx.Err()
	}
	c.last = s
	return s, nil
}

// HostStats returns only fields accepted by the current closed Gateway schema.
// RAM must be available: that schema cannot express missing mandatory values.
func (s Snapshot) HostStats() map[string]any {
	if !s.Memory.Available {
		return nil
	}
	p := map[string]any{"cpuCount": s.CPU.Count, "memoryTotalBytes": s.Memory.TotalBytes, "memoryFreeBytes": s.Memory.AvailableBytes}
	if len(s.LoadAverage) == 3 {
		p["loadAverage"] = s.LoadAverage
	}
	if s.Disk.Available {
		p["diskTotalBytes"] = s.Disk.TotalBytes
		p["diskAvailableBytes"] = s.Disk.AvailableBytes
	}
	return p
}
