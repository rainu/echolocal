// Package avatar manages the local native avatar renderer and its asset catalog.
package avatar

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"github.com/ygelfand/echolocal/internal/config"
	"github.com/ygelfand/echolocal/internal/layout"
	"image"
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
	MouthSpeechPreview
)

// MinZoom, MaxZoom and MaxPan are renderer-scale equivalents of the shared percent bounds.
const (
	MinZoom = float64(config.AvatarZoomMin) / 100
	MaxZoom = float64(config.AvatarZoomMax) / 100
	MaxPan  = float64(config.AvatarPositionLimit) / 100
)

type Options struct {
	Idle, Blink, Animation bool
	Mouth                  int
	Fixed                  float64
	MouthIntensity         float64
	Yaw, Pitch, Zoom       float64
	PanX, PanY             float64
}

// Mouth: 0 closed, 1 fixed, 2 synthetic, 3 local shared microphone, 4 speech preview.
type State struct {
	Options                          Options
	Running                          bool
	Notice                           string
	FPS, Mouth, RMS, Seconds, WorkMS float64
	Frames, Blocks, Gaps             uint64
	Muted                            bool
	ForegroundNotice                 string
	BackgroundNotice                 string
	Ready                            bool
	ModelName, Credit                string
}
type Controller struct {
	motions              MotionState
	motionFile           *os.File
	motionRevision       uint64
	pose                 PoseState
	labOccluded          bool
	mu                   sync.Mutex
	state                State
	dir                  string
	renderer             string
	background           func() string
	foreground           func() string
	fullscreenForeground func() string
	source               func() string
	resolve              func(string) (AssetSet, string, string, error)
	overlay              func() image.Rectangle
	agentWorking         func() bool
	talkLayout           bool
	fullscreen           bool
	fullscreenBackground func() string
	viewport             Viewport
	listen               func() (<-chan []int16, func())
	muted                func() bool
	output               func() float64
	duration             time.Duration
	redraw               func()
	cancel               context.CancelFunc
	input                io.WriteCloser
	generation           uint64
	command              func(context.Context, string, ...string) *exec.Cmd
}

func New(dir string, listen func() (<-chan []int16, func()), muted func() bool, redraw func()) *Controller {
	return &Controller{dir: dir, renderer: layout.AvatarRenderer, resolve: ResolveAssets, viewport: Viewport{X: 640, Y: 80, Size: 640}, listen: listen, muted: muted, redraw: redraw, command: exec.CommandContext, state: State{Options: Options{Idle: true, Blink: true, Animation: true, Fixed: .5, MouthIntensity: .5, Zoom: 1}, Notice: "ready"}}
}

// SetSource is configured during GUI initialization. Changes are read at Start;
// a running model remains stable until Stop/Start, with its loaded name visible.
func (c *Controller) SetSource(source func() string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.source = source
}
func (c *Controller) SetBackground(source func() string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.background = source
}

// SetForeground selects independent split/Lab and fullscreen image layers.
func (c *Controller) SetForeground(source, fullscreen func() string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.foreground, c.fullscreenForeground = source, fullscreen
}
func (c *Controller) Snapshot() State { c.mu.Lock(); defer c.mu.Unlock(); return c.state }
func (c *Controller) Configure(o Options) {
	if o.Mouth < 0 || o.Mouth > MouthSpeechPreview || !finite(o.MouthIntensity) || !finite(o.Fixed) || !finite(o.Yaw) || !finite(o.Pitch) || !finite(o.Zoom) || !finite(o.PanX) || !finite(o.PanY) {
		return
	}
	// Zero-value Options retain the default framing for existing callers.
	if o.Zoom == 0 {
		o.Zoom = 1
	}
	o.Zoom = math.Max(MinZoom, math.Min(MaxZoom, o.Zoom))
	o.PanX = math.Max(-MaxPan, math.Min(MaxPan, o.PanX))
	o.PanY = math.Max(-MaxPan, math.Min(MaxPan, o.PanY))
	o.Pitch = math.Max(-85, math.Min(85, o.Pitch))
	o.MouthIntensity = math.Max(0, math.Min(1, o.MouthIntensity))
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
	backgroundNotice := ""
	backgroundPath := ""
	if c.background != nil {
		backgroundPath = c.background()
	}
	if backgroundPath != "" && err == nil {
		file, release, e := ResolveBackground("/", c.dir, backgroundPath)
		if e != nil {
			backgroundNotice = e.Error()
			backgroundPath = ""
		} else {
			original := cleanup
			cleanup = func() { release(); original() }
			backgroundPath = file
		}
	}
	fullBackground := ""
	if c.talkLayout && c.fullscreenBackground != nil {
		if selected := c.fullscreenBackground(); selected != "" && err == nil {
			file, release, e := ResolveBackground("/", c.dir, selected)
			if e != nil {
				backgroundNotice = e.Error()
			} else {
				fullBackground = file
				original := cleanup
				cleanup = func() { release(); original() }
			}
		}
	}
	foregroundNotice := ""
	resolveForeground := func(source func() string) string {
		if source == nil || err != nil {
			return ""
		}
		selected := source()
		if selected == "" {
			return ""
		}
		file, release, e := ResolveForeground("/", c.dir, selected)
		if e != nil {
			foregroundNotice = e.Error()
			return ""
		}
		original := cleanup
		cleanup = func() { release(); original() }
		return file
	}
	foregroundPath := resolveForeground(c.foreground)
	fullForeground := ""
	if c.talkLayout {
		fullForeground = resolveForeground(c.fullscreenForeground)
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
		c.motions.Pending = false
		c.mu.Unlock()
		c.redraw()
		return
	}
	vp := c.viewport
	ctx, cancel := context.WithTimeout(context.Background(), c.runDuration())
	args := []string{model, "--display", "--seconds", strconv.Itoa(int(c.runDuration() / time.Second)), "--control", "--protocol", strconv.Itoa(ProtocolVersion), "--viewport", vp.argument()}
	if c.talkLayout {
		args = append(args, "--talk-layout")
		if c.fullscreen {
			args = append(args, "--fullscreen")
		}
		if fullBackground != "" {
			args = append(args, "--fullscreen-background", fullBackground)
		}
	}
	if motion != "" {
		args = append(args, "--animation", motion)
	}
	if backgroundPath != "" && backgroundNotice == "" {
		args = append(args, "--background", backgroundPath)
	}
	if foregroundPath != "" {
		args = append(args, "--foreground", foregroundPath)
	}
	if fullForeground != "" {
		args = append(args, "--fullscreen-foreground", fullForeground)
	}

	cmd := c.command(ctx, c.renderer, args...)
	cmd.WaitDelay = 2 * time.Second
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	// Only a fixed local executable and fixed assets; no shell or caller paths.
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		c.state.Notice = "start_failed"
		c.motions.Pending = false
		c.mu.Unlock()
		c.redraw()
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		cancel()
		c.state.Notice = "start_failed"
		c.motions.Pending = false
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
		c.motions.Pending = false
		c.mu.Unlock()
		c.redraw()
		return
	}
	started = true
	c.loadPoses(assets.Model)
	c.generation++
	gen := c.generation
	opts := c.state.Options
	c.state = State{Options: opts, Running: true, Notice: "starting", BackgroundNotice: backgroundNotice, ForegroundNotice: foregroundNotice, ModelName: assets.Name, Credit: assets.Credit}
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
			c.motions.Pending = false
			c.state.Mouth, c.state.RMS = 0, 0
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
		c.state.Mouth, c.state.RMS = 0, 0
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
	previewStart := time.Time{}
	var poseRevision uint64
	var motionRevision uint64
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
			c.mu.Lock()
			output := c.output
			c.mu.Unlock()
			if s.Options.Mouth != MouthSpeechPreview {
				previewStart = time.Time{}
			} else if previewStart.IsZero() {
				previewStart = time.Now()
			}
			want := output == nil && s.Options.Mouth == MouthMicrophone && !muted
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
			c.mu.Lock()
			talk, full, occluded := c.talkLayout, c.fullscreen, c.labOccluded
			overlay := c.overlay
			workingSource := c.agentWorking
			c.mu.Unlock()
			if !talk {
				if _, err := fmt.Fprintf(stdin, "K %d\n", bit(occluded)); err != nil {
					cancel()
					return
				}
			}
			if talk {
				working := workingSource != nil && workingSource()
				if _, err := fmt.Fprintf(stdin, "W %d\n", bit(working)); err != nil {
					cancel()
					return
				}
				r := image.Rectangle{}
				if overlay != nil {
					r = overlay().Intersect(image.Rect(0, 0, 4096, 4096))
				}
				if _, err := fmt.Fprintf(stdin, "V %d %d %d %d\n", r.Min.X, r.Min.Y, r.Dx(), r.Dy()); err != nil {
					cancel()
					return
				}
				if _, err := fmt.Fprintf(stdin, "T %d\n", bit(full)); err != nil {
					cancel()
					return
				}
			}
			if err := c.motionLine(gen, &motionRevision, stdin); err != nil {
				cancel()
				return
			}
			revision, lines := c.poseLines()
			if revision != poseRevision {
				if _, err := io.WriteString(stdin, lines); err != nil {
					cancel()
					return
				}
				poseRevision = revision
			}
			o := s.Options
			if output != nil {
				level = output()
				if !finite(level) {
					level = 0
				}
				level = SpeechMouth(level, o.MouthIntensity)
				o.Mouth = MouthMicrophone
			} else if o.Mouth == MouthSpeechPreview {
				level = SpeechMouth(PreviewLevel(time.Since(previewStart)), o.MouthIntensity)
				o.Mouth = MouthMicrophone
			}
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
	if c.motionStats(gen, line) {
		return
	}
	if c.poseStats(gen, line) {
		return
	}
	if strings.HasPrefix(line, "AVATAR_READY=") && line != "AVATAR_READY="+strconv.Itoa(ProtocolVersion) {
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
	if line == "AVATAR_READY="+strconv.Itoa(ProtocolVersion) {
		c.mu.Lock()
		if gen == c.generation {
			c.state.Ready = true
			c.state.Notice = "running"
		}
		c.mu.Unlock()
		c.redraw()
		return
	}
	if line == "AVATAR_FOREGROUND_ERROR" {
		c.mu.Lock()
		if gen == c.generation {
			c.state.ForegroundNotice = "foreground_invalid"
		}
		c.mu.Unlock()
		c.redraw()
		return
	}
	if line == "AVATAR_BACKGROUND_ERROR" {
		c.mu.Lock()
		if gen == c.generation {
			c.state.BackgroundNotice = "background_invalid"
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
		if c.state.Notice == "stopping" || !c.state.Running {
			c.mu.Unlock()
			return
		}
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
// Size retains the square Lab/legacy CLI form. Width/Height select a rectangle.
type Viewport struct{ X, Y, Size, Width, Height int }

func (v Viewport) dimensions() (int, int) {
	if v.Width == 0 && v.Height == 0 {
		return v.Size, v.Size
	}
	return v.Width, v.Height
}
func (v Viewport) argument() string {
	if v.Width == 0 && v.Height == 0 {
		return fmt.Sprintf("%d,%d,%d", v.X, v.Y, v.Size)
	}
	return fmt.Sprintf("%d,%d,%d,%d", v.X, v.Y, v.Width, v.Height)
}

func (c *Controller) SetViewport(v Viewport) {
	w, h := v.dimensions()
	if (v.Size != 0 && (v.Width != 0 || v.Height != 0)) || w < 128 || w > 2048 || h < 128 || h > 2048 || v.X < 0 || v.Y < 0 || v.X > 4096-w || v.Y > 4096-h {
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

// SetOutput binds scalar playback levels for Talk. It must be set before Start.
// No PCM, microphone subscription or additional audio connection is involved.
func (c *Controller) SetOutput(level func() float64, duration time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.output = level
	c.duration = max(time.Second, min(24*time.Hour, duration))
}
func (c *Controller) runDuration() time.Duration {
	if c.duration > 0 {
		return c.duration
	}
	return 300 * time.Second
}

// ConversationViewport is shared by standalone Talk and its visual-only preview.
func ConversationViewport(width, height int) Viewport {
	return Viewport{X: width / 2, Y: 0, Width: width - width/2, Height: height}
}

// EnableTalkLayout allocates one full-panel surface for a conversation. Layout
// switches change its scissor, not the process, animation clock or audio lane.
func (c *Controller) EnableTalkLayout(background func() string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.talkLayout = true
	c.fullscreenBackground = background
}
func (c *Controller) SetTalkMode(full bool) {
	c.mu.Lock()
	c.fullscreen = full
	c.mu.Unlock()
	c.redraw()
}

// SetLabOccluded reveals the native keyboard without stopping the renderer or
// discarding unsaved joint edits. It is never configured by Talk.
func (c *Controller) SetLabOccluded(hidden bool) {
	c.mu.Lock()
	c.labOccluded = hidden
	c.mu.Unlock()
}

// SetOverlay reveals an application-owned transient card through the Talk surface.
// The source is sampled with the scalar control feed; no audio or UI ownership changes.
func (c *Controller) SetOverlay(source func() image.Rectangle) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.overlay = source
}

// SetAgentWorking exposes only a scalar aperture flag; the application owns
// localized accessible text. Never sends session IDs or content to native code.
func (c *Controller) SetAgentWorking(source func() bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.agentWorking = source
}
