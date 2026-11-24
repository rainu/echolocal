package health

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestCPU(t *testing.T) {
	total, idle, e := ParseCPU("cpu  100 10 20 200 30 4 5 6 90 8\n")
	if e != nil || total != 375 || idle != 230 {
		t.Fatal(total, idle, e)
	}
	p := Percent(100, 50, 300, 150)
	if p == nil || *p != 50 {
		t.Fatal(p)
	}
	for _, v := range [][4]uint64{{10, 5, 10, 5}, {20, 10, 10, 5}, {10, 5, 20, 4}, {10, 1, 20, 12}} {
		if Percent(v[0], v[1], v[2], v[3]) != nil {
			t.Fatal(v)
		}
	}
	for _, v := range []string{"", "cpu0 1 2 3 4", "cpu 1 broken 3 4"} {
		if _, _, e := ParseCPU(v); e == nil {
			t.Fatal(v)
		}
	}
}
func TestMemory(t *testing.T) {
	m := ParseMemory("MemTotal: 1024 kB\nMemAvailable: 256 kB\n")
	if !m.Available || m.TotalBytes != 1048576 || m.AvailableBytes != 262144 || m.UsedBytes != 786432 {
		t.Fatal(m)
	}
	for _, v := range []string{"MemTotal: 1024 kB", "MemTotal: 1 kB\nMemAvailable: 2 kB", "MemTotal: 0 kB\nMemAvailable: 0 kB", "MemTotal: 1 MB\nMemAvailable: 0 kB"} {
		if ParseMemory(v).Available {
			t.Fatal(v)
		}
	}
}
func TestHostStats(t *testing.T) {
	s := Snapshot{CPU: CPU{Count: 4}}
	if s.HostStats() != nil {
		t.Fatal("unknown RAM must not be zero")
	}
	s.Memory = Memory{Available: true, TotalBytes: 100, AvailableBytes: 25, UsedBytes: 75}
	s.Disk = Disk{Available: true, Path: "/data", TotalBytes: 200, AvailableBytes: 50, UsedBytes: 150}
	p := s.HostStats()
	if len(p) != 5 || p["memoryFreeBytes"] != uint64(25) || p["diskAvailableBytes"] != uint64(50) {
		t.Fatal(p)
	}
}
func TestCancellationCacheAndReset(t *testing.T) {
	c := &Collector{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := c.Collect(ctx); e == nil {
		t.Fatal("cancelled collection")
	}
	c.last = Snapshot{MeasuredAtMS: time.Now().UnixMilli(), CPU: CPU{Count: 17}}
	s, e := c.Collect(context.Background())
	if e != nil || s.CPU.Count != 17 {
		t.Fatal(s, e)
	}
	c.Reset()
	if c.last.MeasuredAtMS != 0 {
		t.Fatal("retained revoked metrics")
	}
}
func TestDeviceProbe(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	c := &Collector{}
	s, e := c.Collect(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if s.MeasuredAtMS == 0 {
		t.Fatal(s)
	}
	data, _ := json.Marshal(s)
	t.Log(string(data))
	if s.CPU.Available && (*s.CPU.Percent < 0 || *s.CPU.Percent > 100) {
		t.Fatal(s.CPU)
	}
}
