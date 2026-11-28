package mictest

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const owner = "0123456789abcdef0123456789abcdef"
const other = "fedcba9876543210fedcba9876543210"

type fixture struct {
	m                   *Monitor
	frames              chan []int16
	muted, busy, output atomic.Bool
	listeners, plays    atomic.Int32
	got                 chan []int16
}

func setup(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{frames: make(chan []int16, 16), got: make(chan []int16, 4)}
	f.m = New(Options{
		Listen: func() (<-chan []int16, func()) { f.listeners.Add(1); return f.frames, func() { f.listeners.Add(-1) } },
		Check: func() error {
			if f.muted.Load() {
				return errors.New("muted")
			}
			return nil
		},
		Busy: f.busy.Load, OutputBusy: f.output.Load, Processing: func() string { return "processed settings" },
		Play: func(ctx context.Context, p []int16) error {
			if f.listeners.Load() != 0 {
				t.Error("playback overlapped capture")
			}
			f.plays.Add(1)
			f.got <- append([]int16(nil), p...)
			<-ctx.Done()
			return ctx.Err()
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = f.m.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	return f
}
func phase(t *testing.T, m *Monitor, want string) State {
	t.Helper()
	until := time.Now().Add(2 * time.Second)
	for time.Now().Before(until) {
		s := m.State(owner)
		if s.Phase == want {
			return s
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("want %s, got %+v", want, m.State(owner))
	return State{}
}
func record(t *testing.T, f *fixture) {
	t.Helper()
	if e := f.m.Command(owner, "record"); e != nil {
		t.Fatal(e)
	}
	until := time.Now().Add(time.Second)
	for f.listeners.Load() == 0 && time.Now().Before(until) {
		time.Sleep(time.Millisecond)
	}
}
func ready(t *testing.T, f *fixture) {
	t.Helper()
	record(t, f)
	f.frames <- []int16{12, -42, 63}
	until := time.Now().Add(time.Second)
	for f.m.State(owner).Seconds == 0 && time.Now().Before(until) {
		time.Sleep(time.Millisecond)
	}
	if e := f.m.Command(owner, "stop"); e != nil {
		t.Fatal(e)
	}
	phase(t, f.m, "ready")
}
func TestProcessedTakeStopPlayDiscard(t *testing.T) {
	f := setup(t)
	ready(t, f)
	s := f.m.State(owner)
	if s.Seconds != 3.0/Rate || s.Processing != "processed settings" {
		t.Fatal(s)
	}
	if e := f.m.Command(owner, "play"); e != nil {
		t.Fatal(e)
	}
	p := <-f.got
	if len(p) != 3 || p[1] != -42 {
		t.Fatal(p)
	}
	if !f.m.Active() {
		t.Fatal("playback gate open")
	}
	_ = f.m.Command(owner, "stop")
	if f.m.Active() {
		t.Fatal("gate not released")
	}
	held := f.m.pcm
	_ = f.m.Command(owner, "discard")
	if held[0] != 0 || len(f.m.pcm) != 0 || f.m.State(owner).Owned {
		t.Fatal("not erased")
	}
}
func TestOwnershipAndBusy(t *testing.T) {
	f := setup(t)
	f.busy.Store(true)
	if f.m.Command(owner, "record") != ErrBusy {
		t.Fatal("busy admitted")
	}
	f.busy.Store(false)
	record(t, f)
	for _, a := range []string{"record", "stop", "play", "discard", "ping"} {
		if f.m.Command(other, a) != ErrOwner {
			t.Fatal(a)
		}
	}
	if f.m.Command(owner, "record") != ErrBusy {
		t.Fatal("duplicate admitted")
	}
	_ = f.m.Command(owner, "discard")
	if f.listeners.Load() != 0 || f.m.Active() {
		t.Fatal("capture leaked")
	}
	f.output.Store(true)
	if f.m.Command(owner, "record") != ErrBusy {
		t.Fatal("output admitted")
	}
}
func TestBoundedCaptureAndNoSilentFeedbackLoop(t *testing.T) {
	f := setup(t)
	record(t, f)
	f.frames <- make([]int16, MaxSamples+100)
	s := phase(t, f.m, "ready")
	if s.Seconds != 8 || f.listeners.Load() != 0 || f.plays.Load() != 0 {
		t.Fatal(s)
	}
	if f.m.Active() {
		t.Fatal("gate not released")
	}
}
func TestMuteDiscardsInsteadOfKeepingPartialAudio(t *testing.T) {
	f := setup(t)
	f.muted.Store(true)
	if f.m.Command(owner, "record") == nil {
		t.Fatal("muted admitted")
	}
	f.muted.Store(false)
	record(t, f)
	f.frames <- []int16{1, 2}
	f.muted.Store(true)
	s := phase(t, f.m, "error")
	if s.Error != "muted" || s.Seconds != 0 || f.m.Active() {
		t.Fatal(s)
	}
}
func TestUnavailableCapture(t *testing.T) {
	f := setup(t)
	record(t, f)
	close(f.frames)
	s := phase(t, f.m, "error")
	if s.Error != "capture_unavailable" || f.listeners.Load() != 0 {
		t.Fatal(s)
	}
}
func TestLostPageAndRetentionErase(t *testing.T) {
	for _, retention := range []bool{false, true} {
		t.Run(map[bool]string{false: "lease", true: "retention"}[retention], func(t *testing.T) {
			f := setup(t)
			ready(t, f)
			held := f.m.pcm
			f.m.mu.Lock()
			if retention {
				f.m.expires = time.Now().Add(-time.Second)
			} else {
				f.m.lease = time.Now().Add(-time.Second)
			}
			f.m.mu.Unlock()
			s := phase(t, f.m, "error")
			if s.Error != "expired" || s.Seconds != 0 || held[0] != 0 || s.Owned {
				t.Fatal(s)
			}
		})
	}
}
func TestCompetingVoiceAbortsAndRecovers(t *testing.T) {
	f := setup(t)
	record(t, f)
	f.busy.Store(true)
	s := phase(t, f.m, "error")
	if s.Error != "busy" || f.m.Active() || f.listeners.Load() != 0 {
		t.Fatal(s)
	}
	f.busy.Store(false)
	record(t, f)
	_ = f.m.Command(owner, "discard")
}
func TestPlaybackFailureVisible(t *testing.T) {
	f := setup(t)
	ready(t, f)
	f.m.o.Play = func(context.Context, []int16) error { return errors.New("failed") }
	_ = f.m.Command(owner, "play")
	s := phase(t, f.m, "error")
	if s.Error != "playback_failed" || f.m.Active() {
		t.Fatal(s)
	}
}
func TestHTTPRejectsCrossOriginAndOversizedCommands(t *testing.T) {
	f := setup(t)
	h := f.m.Handler()
	for _, tc := range []struct {
		body, origin string
		want         int
	}{{`{"action":"record"}`, "https://other.invalid", 403}, {strings.Repeat("x", 300), "", 400}, {`{"action":"record","unknown":true}`, "", 400}} {
		r := httptest.NewRequest(http.MethodPost, "https://echo.invalid/", strings.NewReader(tc.body))
		r.Header.Set("X-Microphone-Test", owner)
		if tc.origin != "" {
			r.Header.Set("Origin", tc.origin)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
	}
	r := httptest.NewRequest("GET", "https://echo.invalid/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w)
	}
}
func TestRepeatUsesFreshFrames(t *testing.T) {
	f := setup(t)
	ready(t, f)
	_ = f.m.Command(owner, "discard")
	record(t, f)
	f.frames <- []int16{99}
	until := time.Now().Add(time.Second)
	for f.m.State(owner).Seconds == 0 && time.Now().Before(until) {
		time.Sleep(time.Millisecond)
	}
	_ = f.m.Command(owner, "stop")
	if len(f.m.pcm) != 1 || f.m.pcm[0] != 99 {
		t.Fatal(f.m.pcm)
	}
}

func TestStaleWatcherCannotCancelNewTakeOrRenewedLease(t *testing.T) {
	f := setup(t)
	ready(t, f)
	f.m.mu.Lock()
	generation := f.m.generation
	f.m.mu.Unlock()
	record(t, f)
	f.m.resetIf("busy", generation)
	if !f.m.Active() || f.m.State(owner).Phase != "recording" {
		t.Fatal("stale watcher cancelled a newer take")
	}
	_ = f.m.Command(owner, "stop")
	f.m.mu.Lock()
	generation = f.m.generation
	f.m.lease = time.Now().Add(-time.Second)
	f.m.mu.Unlock()
	_ = f.m.Command(owner, "ping")
	f.m.resetIf("expired", generation)
	if !f.m.State(owner).Owned {
		t.Fatal("stale expiry ignored renewed lease")
	}
}
