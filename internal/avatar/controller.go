// Package avatar manages the local native avatar renderer and its asset catalog.
package avatar

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"github.com/ygelfand/echolocal/internal/layout"
	"io"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	MouthClosed = iota
	MouthManual
	MouthSynthetic
	MouthMicrophone
)

type Options struct {
	Idle, Blink, Animation bool
	Mouth                  int
	Fixed                  float64
	Yaw, Pitch, Zoom       float64
	PanX, PanY             float64
}

// Mouth: 0 closed, 1 fixed, 2 synthetic, 3 local shared microphone.
type State struct {
	Options                          Options
	Running                          bool
	Notice                           string
	FPS, Mouth, RMS, Seconds, WorkMS float64
	Frames, Blocks, Gaps             uint64
	Muted                            bool
	Ready                            bool
	ModelName, Credit                string
}
type Controller struct {
	mu         sync.Mutex
	state      State
	dir        string
	renderer   string
	source     func() string
	resolve    func(string) (AssetSet, string, string, error)
	viewport   Viewport
	listen     func() (<-chan []int16, func())
	muted      func() bool
	redraw     func()
	cancel     context.CancelFunc
	input      io.WriteCloser
	generation uint64
	command    func(context.Context, string, ...string) *exec.Cmd
}

func New(dir string, listen func() (<-chan []int16, func()), muted func() bool, redraw func()) *Controller {
	return &Controller{dir: dir, renderer: layout.AvatarRenderer, resolve: ResolveAssets, viewport: Viewport{X: 640, Y: 80, Size: 640}, listen: listen, muted: muted, redraw: redraw, command: exec.CommandContext, state: State{Options: Options{Idle: true, Blink: true, Animation: true, Fixed: .5, Zoom: 1}, Notice: "ready"}}
}

// SetSource is configured during GUI initialization. Changes are read at Start;
// a running model remains stable until Stop/Start, with its loaded name visible.
func (c *Controller) SetSource(source func() string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.source = source
}
func (c *Controller) Snapshot() State { c.mu.Lock(); defer c.mu.Unlock(); return c.state }
func (c *Controller) Configure(o Options) {
	if o.Mouth < 0 || o.Mouth > 3 || !finite(o.Fixed) || !finite(o.Yaw) || !finite(o.Pitch) || !finite(o.Zoom) || !finite(o.PanX) || !finite(o.PanY) {
		return
	}
	// Zero-value Options retain the default framing for existing callers.
	if o.Zoom == 0 {
		o.Zoom = 1
	}
	o.Zoom = math.Max(.5, math.Min(3, o.Zoom))
	o.PanX = math.Max(-1, math.Min(1, o.PanX))
	o.PanY = math.Max(-1, math.Min(1, o.PanY))
	o.Pitch = math.Max(-85, math.Min(85, o.Pitch))
	o.Fixed = math.Max(0, math.Min(1, o.Fixed))
	o.Yaw = math.Mod(o.Yaw+180, 360)
	if o.Yaw < 0 {
		o.Yaw += 360
	}
	o.Yaw -= 180
	c.mu.Lock()
	c.state.Options = o
	c.mu.Unlock()
	c.redraw()
}
func finite(n float64) bool { return !math.IsNaN(n) && !math.IsInf(n, 0) }
func (c *Controller) Start() {
	c.mu.Lock()
	if c.state.Running {
		c.mu.Unlock()
		return
	}
	var assets AssetSet
	var model, motion string
	var err error
	cleanup := func() {}
	custom := ""
	if c.source != nil {
		custom = c.source()
	}
	if custom != "" {
		assets, model, motion, cleanup, err = ResolveCustom("/", c.dir, custom)
	} else {
		assets, model, motion, err = c.resolve(c.dir)
	}
	started := false
	defer func() {
		if !started {
			cleanup()
		}
	}()
	if err != nil {
		c.state.Notice = "assets_unavailable"
		if custom != "" {
			c.state.ModelName = ""
			c.state.Credit = ""
			c.state.Ready = false
			c.state.Notice = err.Error()
		}
		c.mu.Unlock()
		c.redraw()
		return
	}
	vp := c.viewport
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	args := []string{model, "--display", "--seconds", "300", "--control", "--protocol", strconv.Itoa(ProtocolVersion), "--viewport", fmt.Sprintf("%d,%d,%d", vp.X, vp.Y, vp.Size)}
	if motion != "" {
		args = append(args, "--animation", motion)
	}
	cmd := c.command(ctx, c.renderer, args...)
	cmd.WaitDelay = 2 * time.Second
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	// Only a fixed local executable and fixed assets; no shell or caller paths.
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		c.state.Notice = "start_failed"
		c.mu.Unlock()
		c.redraw()
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		cancel()
		c.state.Notice = "start_failed"
		c.mu.Unlock()
		c.redraw()
		return
	}
	cmd.Stderr = io.Discard // No arbitrary paths/raw process logs in UI.
	if err = cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		cancel()
		c.state.Notice = "missing_renderer"
		c.mu.Unlock()
		c.redraw()
		return
	}
	started = true
	c.generation++
	gen := c.generation
	opts := c.state.Options
	c.state = State{Options: opts, Running: true, Notice: "starting", ModelName: assets.Name, Credit: assets.Credit}
	c.cancel = cancel
	c.input = stdin
	c.mu.Unlock()
	c.redraw()
	startup := time.AfterFunc(10*time.Second, func() {
		c.mu.Lock()
		waiting := c.generation == gen && c.state.Running && !c.state.Ready
		if waiting {
			c.state.Notice = "renderer_timeout"
		}
		c.mu.Unlock()
		if waiting {
			stdin.Close()
			cancel()
			c.redraw()
		}
	})
	go c.feed(ctx, gen, stdin, cancel)
	go func() {
		defer cleanup()
		scan := bufio.NewScanner(stdout)
		scan.Buffer(make([]byte, 4096), 4096)
		for scan.Scan() {
			c.stats(gen, scan.Text())
		}
		startup.Stop()
		scanErr := scan.Err()
		if scanErr != nil {
			cancel()
			stdin.Close()
		}
		err := cmd.Wait()
		cancel()
		stdin.Close()
		c.mu.Lock()
		if c.generation == gen {
			stopping := c.state.Notice == "stopping"
			c.state.Running = false
			previous := c.state.Notice
			c.state.Notice = "finished"
			if previous != "running" && previous != "starting" && previous != "stopping" {
				c.state.Notice = previous
			}
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				c.state.Notice = "timeout"
			} else if (err != nil || scanErr != nil || !c.state.Ready) && !stopping && (previous == "starting" || previous == "running") {
				c.state.Notice = "renderer_failed"
			}
			c.cancel = nil
			c.input = nil
		}
		c.mu.Unlock()
		c.redraw()
	}()
}
func (c *Controller) Stop() {
	c.mu.Lock()
	cancel, input := c.cancel, c.input
	if c.state.Running {
		c.state.Notice = "stopping"
	}
	c.mu.Unlock()
	if input != nil {
		input.Close()
	}
	if cancel != nil {
		cancel()
	}
	c.redraw()
}
func target(rms, floor float64) float64 {
	gate := math.Max(.00012, floor*2.5)
	if rms <= gate {
		return 0
	}
	return math.Min(1, math.Log(rms/gate)/math.Log(12))
}
func (c *Controller) feed(ctx context.Context, gen uint64, stdin io.WriteCloser, cancel context.CancelFunc) {
	tick := time.NewTicker(time.Second / 30)
	defer tick.Stop()
	var audio <-chan []int16
	var release func()
	floor := .0001
	level, rms := 0., 0.
	var began, last time.Time
	defer func() {
		if release != nil {
			release()
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-audio:
			if !ok {
				audio = nil
				if release != nil {
					release()
					release = nil
				}
				continue
			}
			if len(data) == 0 {
				continue
			}
			stamp := time.Now()
			sum := 0.
			for _, v := range data {
				f := float64(v) / 32768
				sum += f * f
			}
			rms = math.Sqrt(sum / float64(len(data)))
			if stamp.Sub(began) < time.Second {
				floor += (rms - floor) * .05
				level = 0
			} else {
				level = target(rms, floor)
			}
			c.mu.Lock()
			if gen == c.generation {
				c.state.Blocks++
				if !last.IsZero() && stamp.Sub(last) > 100*time.Millisecond {
					c.state.Gaps++
				}
				c.state.RMS = rms
			}
			c.mu.Unlock()
			last = stamp
		case <-tick.C:
			s := c.Snapshot()
			muted := c.muted()
			want := s.Options.Mouth == MouthMicrophone && !muted
			if want && release == nil {
				audio, release = c.listen()
				began = time.Now()
				last = time.Time{}
				floor = .0001
				level = 0
				rms = 0
			}
			if !want && release != nil {
				release()
				release = nil
				audio = nil
				level = 0
				rms = 0
			}
			if time.Since(last) > 200*time.Millisecond {
				level = 0
			}
			if muted {
				level = 0
			}
			c.mu.Lock()
			if gen == c.generation {
				c.state.Muted = muted
			}
			c.mu.Unlock()
			// Scalar-only IPC. Bounded pipe writes so a stuck child cannot pin capture.
			if f, ok := stdin.(*os.File); ok {
				_ = f.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))
			}
			o := s.Options
			if _, err := fmt.Fprintf(stdin, "S %d %d %d %d %.4f %.4f %.4f %.4f %.4f %.4f %.4f\n", bit(o.Idle), bit(o.Blink), bit(o.Animation), o.Mouth, o.Fixed, level, o.Yaw, o.Pitch, o.Zoom, o.PanX, o.PanY); err != nil {
				cancel()
				return
			}
		}
	}
}
func bit(v bool) int {
	if v {
		return 1
	}
	return 0
}
func (c *Controller) stats(gen uint64, line string) {
	if strings.HasPrefix(line, "AVATAR_READY=") && line != "AVATAR_READY=4" {
		c.mu.Lock()
		var cancel context.CancelFunc
		if gen == c.generation {
			c.state.Notice = "protocol_error"
			cancel = c.cancel
		}
		c.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		return
	}
	if line == "AVATAR_READY=4" {
		c.mu.Lock()
		if gen == c.generation {
			c.state.Ready = true
			c.state.Notice = "running"
		}
		c.mu.Unlock()
		c.redraw()
		return
	}
	if strings.HasPrefix(line, "AVATAR_ERROR=") {
		code := strings.TrimPrefix(line, "AVATAR_ERROR=")
		switch code {
		case "model_invalid", "motion_invalid", "gpu_failed", "protocol_error":
			c.mu.Lock()
			if gen == c.generation {
				c.state.Notice = code
			}
			c.mu.Unlock()
			c.redraw()
		}
		return
	}
	if !strings.HasPrefix(line, "AVATAR_STATS ") {
		return
	}
	fields := strings.Fields(line)
	if len(fields) != 6 {
		return
	}
	vals := make([]float64, 5)
	for i := range vals {
		v, err := strconv.ParseFloat(fields[i+1], 64)
		if err != nil || !finite(v) || v < 0 {
			return
		}
		vals[i] = v
	}
	c.mu.Lock()
	if gen == c.generation {
		c.state.FPS = vals[0]
		c.state.Frames = uint64(vals[1])
		c.state.Seconds = vals[2]
		c.state.Mouth = vals[3]
		c.state.WorkMS = vals[4]
	}
	c.mu.Unlock()
	c.redraw()
}

// Viewport follows the GUI's physical preview area; changes stop an old surface.
type Viewport struct{ X, Y, Size int }

func (c *Controller) SetViewport(v Viewport) {
	if v.X < 0 || v.Y < 0 || v.X+v.Size > 4096 || v.Y+v.Size > 4096 || v.Size < 128 || v.Size > 2048 {
		return
	}
	c.mu.Lock()
	changed := c.viewport != v
	c.viewport = v
	running := c.state.Running
	c.mu.Unlock()
	if changed && running {
		c.Stop()
	}
}

// EffectiveZoom treats zero-value Options as normal framing.
func EffectiveZoom(z float64) float64 {
	if z == 0 {
		return 1
	}
	return z
}
