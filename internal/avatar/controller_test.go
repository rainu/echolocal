package avatar

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRendererHelper(t *testing.T) {
	mode := os.Getenv("VRM_TEST_HELPER")
	if mode == "" {
		return
	}
	if mode == "no-ready" {
		os.Exit(0)
	}
	if mode == "wrong-protocol" {
		fmt.Println("AVATAR_READY=99")
		os.Exit(0)
	}
	if mode == "fail" {
		os.Exit(7)
	}
	fmt.Println("AVATAR_READY=4")
	fmt.Println("AVATAR_STATS 30 10 1 0 5")
	scan := bufio.NewScanner(os.Stdin)
	for scan.Scan() {
		var i, b, a, m int
		var f, l, yaw, pitch, zoom, panX, panY float64
		if _, e := fmt.Sscanf(scan.Text(), "S %d %d %d %d %f %f %f %f %f %f %f", &i, &b, &a, &m, &f, &l, &yaw, &pitch, &zoom, &panX, &panY); e == nil {
			mouth := 0.
			if m == 1 {
				mouth = f
			}
			if m == 3 {
				mouth = l
			}
			if mode == "pitch" {
				mouth = (pitch + 85) / 170
			}
			if mode == "zoom" {
				mouth = zoom / 10
			}
			if mode == "panX" {
				mouth = (panX + 1) / 2
			}
			if mode == "panY" {
				mouth = (panY + 1) / 2
			}
			if mode == "yaw" {
				mouth = (yaw + 180) / 360
			}
			fmt.Printf("AVATAR_STATS 30 20 2 %.4f 5\n", mouth)
		}
	}
	os.Exit(0)
}
func eventually(t *testing.T, condition func() bool) {
	t.Helper()
	end := time.Now().Add(3 * time.Second)
	for time.Now().Before(end) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met")
}
func helper(c *Controller, mode string, starts *atomic.Int32) {
	c.resolve = fakeAssets
	c.command = func(ctx context.Context, path string, args ...string) *exec.Cmd {
		if !strings.HasSuffix(path, "/echolocal-avatar") || (len(args) != 9 && len(args) != 11) || args[4] != "--control" || args[6] != "4" {
			panic(fmt.Sprintf("unexpected command %s %v", path, args))
		}
		if d, ok := ctx.Deadline(); !ok || time.Until(d) > 301*time.Second {
			panic("missing timeout")
		}
		starts.Add(1)
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRendererHelper$")
		cmd.Env = []string{"VRM_TEST_HELPER=" + mode}
		return cmd
	}
}
func TestLifecycleAndScalarControls(t *testing.T) {
	var starts atomic.Int32
	c := New(t.TempDir(), func() (<-chan []int16, func()) { t.Error("unexpected mic subscription"); return nil, func() {} }, func() bool { return false }, func() {})
	helper(c, "normal", &starts)
	c.Configure(Options{Mouth: 1, Fixed: .7})
	c.Start()
	c.Start()
	defer c.Stop()
	eventually(t, func() bool { return c.Snapshot().Mouth == .7 })
	if starts.Load() != 1 {
		t.Fatal("duplicate renderer")
	}
	c.Configure(Options{Mouth: 1, Fixed: .2})
	eventually(t, func() bool { return c.Snapshot().Mouth == .2 })
	c.Stop()
	eventually(t, func() bool { return !c.Snapshot().Running })
	c.Start()
	eventually(t, func() bool { return c.Snapshot().Running && starts.Load() == 2 })
	c.Stop()
	eventually(t, func() bool { return !c.Snapshot().Running })
}
func TestMicrophoneMuteAndRelease(t *testing.T) {
	var starts, listens, releases atomic.Int32
	var muted atomic.Bool
	audio := make(chan []int16, 8)
	c := New(t.TempDir(), func() (<-chan []int16, func()) { listens.Add(1); return audio, func() { releases.Add(1) } }, muted.Load, func() {})
	helper(c, "normal", &starts)
	c.Configure(Options{Mouth: 3})
	c.Start()
	defer c.Stop()
	eventually(t, func() bool { return listens.Load() == 1 })
	// First second is the quiet calibration; only local synthetic samples used.
	end := time.Now().Add(1100 * time.Millisecond)
	for time.Now().Before(end) {
		audio <- make([]int16, 320)
		time.Sleep(20 * time.Millisecond)
	}
	speech := make([]int16, 320)
	for i := range speech {
		speech[i] = 10000
	}
	audio <- speech
	eventually(t, func() bool { return c.Snapshot().Mouth > .9 })
	muted.Store(true)
	eventually(t, func() bool { return c.Snapshot().Muted && c.Snapshot().Mouth == 0 && releases.Load() == 1 })
	c.Configure(Options{})
	muted.Store(false)
	time.Sleep(80 * time.Millisecond)
	if listens.Load() != 1 {
		t.Fatal("subscribed while mic mode disabled")
	}
	c.Stop()
	eventually(t, func() bool { return !c.Snapshot().Running })
}
func TestMissingAndFailingRenderer(t *testing.T) {
	c := New(t.TempDir(), nil, func() bool { return false }, func() {})
	c.renderer = t.TempDir() + "/echolocal-avatar" // Also absent on a device with the production helper installed.
	c.resolve = fakeAssets
	c.Start()
	if c.Snapshot().Running || c.Snapshot().Notice != "missing_renderer" {
		t.Fatal(c.Snapshot())
	}
	var starts atomic.Int32
	helper(c, "fail", &starts)
	c.Start()
	eventually(t, func() bool { return !c.Snapshot().Running })
	if c.Snapshot().Notice != "renderer_failed" {
		t.Fatal(c.Snapshot())
	}
}
func TestRejectInvalidState(t *testing.T) {
	c := New("/fixed", nil, nil, func() {})
	before := c.Snapshot()
	c.Configure(Options{Mouth: 9})
	c.Configure(Options{Fixed: math.NaN()})
	if c.Snapshot() != before {
		t.Fatal("invalid config accepted")
	}
	c.stats(0, "AVATAR_STATS NaN 20 1 1 5")
	c.stats(0, "AVATAR_STATS 30 -1 1 1 5")
	if c.Snapshot() != before {
		t.Fatal("invalid stats accepted")
	}
	if target(0, .001) != 0 || target(.03, .001) < .9 {
		t.Fatal("bad gate")
	}
}

func fakeAssets(string) (AssetSet, string, string, error) {
	return AssetSet{Name: "Sample", Credit: "Author"}, "/model.vrm", "/motion.vrma", nil
}

func TestRendererReadinessIsRequired(t *testing.T) {
	for _, tc := range []struct{ mode, notice string }{{"no-ready", "renderer_failed"}, {"wrong-protocol", "protocol_error"}} {
		t.Run(tc.mode, func(t *testing.T) {
			var starts atomic.Int32
			c := New(t.TempDir(), nil, func() bool { return false }, func() {})
			helper(c, tc.mode, &starts)
			c.Start()
			defer c.Stop()
			eventually(t, func() bool { return !c.Snapshot().Running })
			if c.Snapshot().Notice != tc.notice {
				t.Fatal(c.Snapshot())
			}
		})
	}
}
func TestViewportChangeStopsOldSurface(t *testing.T) {
	var starts atomic.Int32
	c := New(t.TempDir(), nil, func() bool { return false }, func() {})
	helper(c, "normal", &starts)
	c.Start()
	defer c.Stop()
	eventually(t, func() bool { return c.Snapshot().Ready })
	c.SetViewport(Viewport{X: 400, Y: 80, Size: 400})
	eventually(t, func() bool { return !c.Snapshot().Running })
}

func TestYawWrapAndInvalidValues(t *testing.T) {
	c := New("/unused", nil, nil, func() {})
	for _, tc := range []struct{ in, want float64 }{{0, 0}, {90, 90}, {180, -180}, {195, -165}, {-195, 165}, {360, 0}, {-360, 0}} {
		c.Configure(Options{Yaw: tc.in})
		if got := c.Snapshot().Options.Yaw; got != tc.want {
			t.Fatalf("yaw %v => %v, want %v", tc.in, got, tc.want)
		}
	}
	before := c.Snapshot()
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		c.Configure(Options{Yaw: bad})
		if c.Snapshot() != before {
			t.Fatal("nonfinite yaw accepted")
		}
	}
}

func TestYawChangesReachRunningRenderer(t *testing.T) {
	var starts atomic.Int32
	c := New(t.TempDir(), nil, func() bool { return false }, func() {})
	helper(c, "yaw", &starts)
	c.Configure(Options{Yaw: 90})
	c.Start()
	defer c.Stop()
	eventually(t, func() bool { return c.Snapshot().Mouth == .75 })
	c.Configure(Options{Yaw: -90})
	eventually(t, func() bool { return c.Snapshot().Mouth == .25 })
	c.Configure(Options{})
	eventually(t, func() bool { return c.Snapshot().Mouth == .5 })
	if starts.Load() != 1 {
		t.Fatal("rotation restarted the renderer")
	}
}

func TestPitchZoomBoundsAndInvalidValues(t *testing.T) {
	c := New("", nil, func() bool { return false }, func() {})
	for _, tc := range []struct{ p, z, wp, wz float64 }{{100, 10, 85, 9.99}, {20, 8, 20, 8}, {20, 9.99, 20, 9.99}, {20, 3, 20, 3}, {-100, .1, -85, .5}, {30, 0, 30, 1}} {
		c.Configure(Options{Pitch: tc.p, Zoom: tc.z})
		o := c.Snapshot().Options
		if o.Pitch != tc.wp || o.Zoom != tc.wz {
			t.Fatal(o)
		}
	}
	before := c.Snapshot().Options
	for _, o := range []Options{{Pitch: math.NaN()}, {Zoom: math.Inf(1)}} {
		c.Configure(o)
		if c.Snapshot().Options != before {
			t.Fatal("accepted nonfinite", o)
		}
	}
}
func TestPitchZoomReachRenderer(t *testing.T) {
	for _, mode := range []string{"pitch", "zoom"} {
		t.Run(mode, func(t *testing.T) {
			c := New("", nil, func() bool { return false }, func() {})
			var starts atomic.Int32
			helper(c, mode, &starts)
			c.Configure(Options{Pitch: 51, Zoom: 2.4})
			c.Start()
			defer c.Stop()
			want := .8
			if mode == "zoom" {
				want = .24
			}
			eventually(t, func() bool { return math.Abs(c.Snapshot().Mouth-want) < .001 })
			c.Configure(Options{Pitch: -51, Zoom: .6})
			want = .2
			if mode == "zoom" {
				want = .06
			}
			eventually(t, func() bool { return math.Abs(c.Snapshot().Mouth-want) < .001 })
			if starts.Load() != 1 {
				t.Fatal("renderer restarted")
			}
		})
	}
}

func TestPanValidationAndScalarDelivery(t *testing.T) {
	for _, axis := range []string{"panX", "panY"} {
		t.Run(axis, func(t *testing.T) {
			c := New("", nil, func() bool { return false }, func() {})
			var starts atomic.Int32
			helper(c, axis, &starts)
			o := c.Snapshot().Options
			o.PanX = .4
			o.PanY = -.6
			c.Configure(o)
			c.Start()
			defer c.Stop()
			want := .7
			if axis == "panY" {
				want = .2
			}
			eventually(t, func() bool { return math.Abs(c.Snapshot().Mouth-want) < .001 })
			o.PanX = 6
			o.PanY = -6
			c.Configure(o)
			if c.Snapshot().Options.PanX != 5 || c.Snapshot().Options.PanY != -5 {
				t.Fatal("bounds")
			}
			before := c.Snapshot().Options
			o.PanX = math.NaN()
			c.Configure(o)
			if c.Snapshot().Options != before {
				t.Fatal("nan accepted")
			}
			o = before
			o.PanY = math.Inf(1)
			c.Configure(o)
			if c.Snapshot().Options != before {
				t.Fatal("inf accepted")
			}
		})
	}
}

func TestCustomControllerReloadReleasesSnapshot(t *testing.T) {
	dir := t.TempDir()
	source := dir + "/custom.vrm"
	os.WriteFile(source, glb(`{"extensions":{"VRM":{"specVersion":"0.0","meta":{"title":"Custom"}}}}`), 0600)
	var starts atomic.Int32
	c := New(dir, nil, func() bool { return false }, func() {})
	helper(c, "normal", &starts)
	originalCommand := c.command
	var snapshot string
	c.command = func(ctx context.Context, path string, args ...string) *exec.Cmd {
		snapshot = args[0]
		return originalCommand(ctx, path, args...)
	}
	c.SetSource(func() string { return source })
	for i := 0; i < 5; i++ {
		c.Start()
		eventually(t, func() bool { return c.Snapshot().Ready })
		if c.Snapshot().ModelName != "Custom" {
			t.Fatal("wrong model")
		}
		c.Stop()
		eventually(t, func() bool { return !c.Snapshot().Running })
		eventually(t, func() bool { _, e := os.Stat(snapshot); return os.IsNotExist(e) })
	}
	c.SetSource(func() string { return dir + "/missing.vrm" })
	c.Start()
	if s := c.Snapshot(); s.Running || s.Notice != "custom_unavailable" {
		t.Fatal(s)
	}
	c.SetSource(func() string { return "" })
	c.Start()
	eventually(t, func() bool { return c.Snapshot().Ready })
	c.Stop()
	eventually(t, func() bool { return !c.Snapshot().Running })
}

func TestPlaybackScalarNeverListensToMicrophone(t *testing.T) {
	var starts, listens atomic.Int32
	var level atomic.Uint64
	level.Store(math.Float64bits(.75))
	c := New(t.TempDir(), func() (<-chan []int16, func()) { listens.Add(1); return nil, func() {} }, func() bool { return false }, func() {})
	helper(c, "normal", &starts)
	c.SetOutput(func() float64 { return math.Float64frombits(level.Load()) }, 2*time.Minute)
	// Even selecting the old microphone mode must not attach capture in Talk.
	c.Configure(Options{Mouth: MouthMicrophone})
	c.Start()
	defer c.Stop()
	eventually(t, func() bool { return c.Snapshot().Mouth == .75 })
	level.Store(math.Float64bits(0))
	eventually(t, func() bool { return c.Snapshot().Mouth == 0 })
	if listens.Load() != 0 || c.Snapshot().Blocks != 0 {
		t.Fatal("Talk attached microphone")
	}
	level.Store(math.Float64bits(math.NaN()))
	time.Sleep(70 * time.Millisecond)
	if c.Snapshot().Mouth != 0 {
		t.Fatal("invalid external level")
	}
	c.Stop()
	eventually(t, func() bool { return !c.Snapshot().Running })
}

func TestViewportRectangleAndLegacyValidation(t *testing.T) {
	c := New("/unused", nil, nil, func() {})
	for _, tc := range []struct {
		v   Viewport
		arg string
	}{
		{Viewport{X: 640, Y: 80, Size: 640}, "640,80,640"},
		{Viewport{X: 640, Width: 640, Height: 800}, "640,0,640,800"},
		{Viewport{X: 400, Width: 400, Height: 1280}, "400,0,400,1280"},
	} {
		c.SetViewport(tc.v)
		if c.viewport != tc.v || c.viewport.argument() != tc.arg {
			t.Fatal(c.viewport, tc.arg)
		}
	}
	before := c.viewport
	for _, v := range []Viewport{
		{Width: 640}, {Height: 800}, {Width: 127, Height: 800}, {Width: 640, Height: 2049},
		{X: 4096, Width: 640, Height: 800}, {Y: -1, Width: 640, Height: 800},
		{X: int(^uint(0) >> 1), Width: 640, Height: 800}, {Size: 640, Width: 640, Height: 800},
	} {
		c.SetViewport(v)
		if c.viewport != before {
			t.Fatal("invalid viewport accepted", v)
		}
	}
}

func TestCloseUpZoomDeliveredWithoutRestart(t *testing.T) {
	c := New("", nil, func() bool { return false }, func() {})
	var starts atomic.Int32
	helper(c, "zoom", &starts)
	c.Start()
	defer c.Stop()
	for _, zoom := range []float64{3, 5, 9.99} {
		c.Configure(Options{Zoom: zoom, Yaw: 25, Pitch: -15, PanX: .2, PanY: .75})
		eventually(t, func() bool { return math.Abs(c.Snapshot().Mouth-zoom/10) < .001 })
		if got := c.Snapshot().Options; got.Zoom != zoom || got.Yaw != 25 || got.PanY != .75 {
			t.Fatal(got)
		}
	}
	if starts.Load() != 1 {
		t.Fatal("zoom restarted renderer", starts.Load())
	}
}
