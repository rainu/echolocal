package talk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	connection "github.com/ygelfand/echolocal/internal/openclaw"
	"github.com/ygelfand/libcountertop/pkg/media/rtc"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeDevice struct {
	started, closed, readyTones atomic.Int32
	frames                      chan []int16
}

func (d *fakeDevice) Start(context.Context) error      { d.started.Add(1); return nil }
func (d *fakeDevice) Close() error                     { d.closed.Add(1); return nil }
func (d *fakeDevice) Frames() (<-chan []int16, func()) { return d.frames, func() {} }
func (d *fakeDevice) Play([]int16)                     {}
func (d *fakeDevice) Queued() int                      { return 0 }
func (d *fakeDevice) StopOutput()                      {}
func (d *fakeDevice) ReadyTone(context.Context) error  { d.readyTones.Add(1); return nil }

type fakeControl struct {
	mu                          sync.Mutex
	holdReady                   <-chan struct{}
	event                       chan connection.TalkEvent
	peer                        *rtc.Link
	id                          string
	capable                     bool
	created                     bool
	warming                     int
	catalogCalls                int
	createCalls                 int
	createParams, catalogParams map[string]any
	closed                      atomic.Int32
}

func (f *fakeControl) SubscribeTalk() (<-chan connection.TalkEvent, func(), error) {
	return f.event, func() {}, nil
}
func (f *fakeControl) CallTalk(ctx context.Context, method string, params, result any) error {
	switch method {
	case "talk.catalog":
		f.catalogParams = params.(map[string]any)
		f.catalogCalls++
		if f.warming > 0 {
			f.warming--
			return &connection.RPCRejection{Retryable: true}
		}
		if !f.created {
			return errors.New("synthetic context does not exist before creation")
		}
		b, _ := json.Marshal(map[string]any{"realtime": map[string]any{"ready": f.capable, "activeProvider": "synthetic", "providers": []any{map[string]any{"id": "synthetic", "configured": true, "transports": []string{"webrtc"}, "brains": []string{"agent-consult"}}}}})
		return json.Unmarshal(b, result)
	case "talk.client.create":
		f.createParams = params.(map[string]any)
		f.createCalls++
		f.created = true
		p := params.(map[string]any)
		f.id = p["voiceSessionId"].(string)
		launch := result.(*connection.TalkLaunch)
		launch.Transport = "webrtc"
		launch.VoiceSessionID = f.id
		launch.ClientControl.Owner = "gateway"
		return nil
	case "talk.client.close":
		f.closed.Add(1)
	}
	return nil
}
func (f *fakeControl) ExchangeOffer(ctx context.Context, _ connection.TalkLaunch, offer string) (string, error) {
	answer, err := f.peer.Accept(ctx, offer)
	if err == nil {
		var event connection.TalkEvent
		event.VoiceSessionID = f.id
		event.Event.Type = "session.ready"
		if f.holdReady != nil {
			go func() {
				select {
				case <-f.holdReady:
					f.event <- event
				case <-ctx.Done():
				}
			}()
		} else {
			f.event <- event
		}
	}
	return answer, err
}
func TestSessionWaitsForPeerProviderAndAudioThenClosesOnce(t *testing.T) {
	d := &fakeDevice{frames: make(chan []int16)}
	remote := &fakeDevice{frames: make(chan []int16)}
	peer, err := rtc.NewAudioOnly(rtc.Media{Audio: remote}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	control := &fakeControl{event: make(chan connection.TalkEvent, 8), peer: peer, capable: true}
	session := NewSession(control, "agent:synthetic:context", d)
	session.nativeMedia = false
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err = session.Open(ctx); err != nil {
		t.Fatal(err)
	}
	if d.started.Load() != 1 || d.readyTones.Load() != 1 || session.Diagnostics().Stage != "ready" {
		t.Fatal("readiness not gated on audio")
	}
	session.Close()
	session.Close()
	if d.closed.Load() != 1 || control.closed.Load() != 1 {
		t.Fatal("duplicate teardown")
	}
}
func TestSessionRejectsMissingCapabilitiesBeforeOpeningMicrophone(t *testing.T) {
	d := &fakeDevice{frames: make(chan []int16)}
	control := &fakeControl{event: make(chan connection.TalkEvent, 8)}
	session := NewSession(control, "agent:synthetic:context", d)
	session.nativeMedia = false
	if err := session.Open(context.Background()); err == nil {
		t.Fatal("unready provider accepted")
	}
	session.Close()
	if d.started.Load() != 0 || d.closed.Load() != 1 {
		t.Fatal("microphone opened before capability check")
	}
}

func TestPlaybackCompletionCannotExpireAnActiveOrNewerResponse(t *testing.T) {
	d := &fakeDevice{frames: make(chan []int16)}
	session := NewSession(nil, "agent:synthetic:context", d)
	var last atomic.Int32
	session.Activity = func(state State) {
		if state == Listening {
			last.Store(1)
		} else if state == Thinking {
			last.Store(2)
		}
	}
	session.responseEpoch.Store(1)
	session.pendingTools.Store(1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	session.afterPlayback(ctx, 1)
	if last.Load() != 2 {
		t.Fatal("agent work did not suspend inactivity")
	}
	last.Store(0)
	session.pendingTools.Store(0)
	session.responseEpoch.Store(2)
	session.afterPlayback(ctx, 1)
	if last.Load() != 0 {
		t.Fatal("old response changed newer activity")
	}
}

func TestSafeReasonNeverIncludesProviderOrContextDetails(t *testing.T) {
	if got := safeReason(errors.New("synthetic-private-context synthetic-secret")); got != "talk_failed" {
		t.Fatal("raw error leaked")
	}
	if got := safeReason(errors.New("rpc_rejected")); got != "rpc_rejected" {
		t.Fatal("safe reason missing")
	}
}

func TestCatalogWarmingRetriesOnlyTheReadNotSessionCreation(t *testing.T) {
	d := &fakeDevice{frames: make(chan []int16)}
	control := &fakeControl{event: make(chan connection.TalkEvent, 8), warming: 1}
	session := NewSession(control, "agent:synthetic:context", d)
	session.nativeMedia = false
	if err := session.Open(context.Background()); err == nil {
		t.Fatal("unready provider accepted")
	}
	session.Close()
	if control.catalogCalls != 2 || control.createCalls != 1 || !control.created || d.started.Load() != 0 {
		t.Fatal("warming retry or capture gate failed")
	}
}

func TestModelSelectionUsesGatewayDefaultOrExplicitCatalogueChoice(t *testing.T) {
	for _, model := range []string{"", "synthetic-future-live-model"} {
		control := &fakeControl{event: make(chan connection.TalkEvent, 8)}
		session := NewSession(control, "agent:synthetic:context", &fakeDevice{frames: make(chan []int16)})
		session.nativeMedia = false
		session.Model = model
		if model != "" {
			session.Provider = "synthetic-provider"
		}
		_ = session.Open(context.Background())
		session.Close()
		if model == "" {
			if _, ok := control.createParams["model"]; ok {
				t.Fatal("Gateway default overridden")
			}
			if _, ok := control.createParams["provider"]; ok {
				t.Fatal("default provider overridden")
			}
		} else {
			if control.createParams["model"] != model || control.catalogParams["model"] != model || control.createParams["provider"] != session.Provider {
				t.Fatal("model/catalogue selection mismatch")
			}
		}
	}
}

func TestQueuedOldCancellationCannotStopNextActivation(t *testing.T) {
	c := New()
	oldCtx, oldCancel := context.WithCancel(context.Background())
	if e := c.Start(oldCtx, &backend{}, time.Hour); e != nil {
		t.Fatal(e)
	}
	oldCancel()
	await(t, func() bool { return !c.Busy() })
	next := &backend{}
	if e := c.Start(context.Background(), next, time.Hour); e != nil {
		t.Fatal(e)
	}
	// Retiring speaker/provider callbacks now retain only this old cancel func.
	oldCancel()
	if !c.Busy() || next.closed.Load() != 0 {
		t.Fatal("old activation cancelled successor")
	}
	c.Stop()
}

func TestProviderReadinessGatesCueAndMicrophoneDuringSlowSetup(t *testing.T) {
	for _, abort := range []bool{false, true} {
		t.Run(fmt.Sprint("cancel=", abort), func(t *testing.T) {
			d := &fakeDevice{frames: make(chan []int16)}
			peer, err := rtc.NewAudioOnly(rtc.Media{Audio: &fakeDevice{frames: make(chan []int16)}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer peer.Close()
			ready := make(chan struct{})
			c := &fakeControl{event: make(chan connection.TalkEvent, 8), peer: peer, capable: true, holdReady: ready}
			s := NewSession(c, "synthetic-readiness", d)
			s.nativeMedia = false
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- s.Open(ctx) }()
			// Wait for transport setup while withholding authoritative provider readiness.
			until := time.Now().Add(3 * time.Second)
			for s.stage.Load() < 5 && time.Now().Before(until) {
				time.Sleep(time.Millisecond)
			}
			if s.stage.Load() < 5 {
				t.Fatal("peer setup did not complete")
			}
			time.Sleep(100 * time.Millisecond)
			if d.started.Load() != 0 || d.readyTones.Load() != 0 {
				t.Fatal("false readiness")
			}
			if abort {
				cancel()
			} else {
				close(ready)
			}
			err = <-done
			s.Close()
			if abort {
				if err == nil || d.started.Load() != 0 || d.readyTones.Load() != 0 {
					t.Fatal("cancelled setup emitted ready")
				}
			} else if err != nil || d.started.Load() != 1 || d.readyTones.Load() != 1 {
				t.Fatal("ready setup", err)
			}
		})
	}
}

type muteProbe struct {
	frames  chan []int16
	packets atomic.Uint64
	level   atomic.Int64
}

func (p *muteProbe) Frames() (<-chan []int16, func()) { return p.frames, func() {} }
func (p *muteProbe) Play(pcm []int16) {
	var total int64
	for _, v := range pcm {
		if v < 0 {
			total -= int64(v)
		} else {
			total += int64(v)
		}
	}
	if len(pcm) > 0 {
		p.level.Store(total / int64(len(pcm)))
	}
	p.packets.Add(1)
}

type muteDevice struct {
	fakeDevice
	probe *muteProbe
}

func (d *muteDevice) Frames() (<-chan []int16, func()) { return d.probe.Frames() }
func (d *muteDevice) Play(pcm []int16)                 { d.probe.Play(pcm) }

func TestSessionMuteKeepsBidirectionalTransportAndSameSession(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	local := &muteProbe{frames: make(chan []int16, 2)}
	remote := &muteProbe{frames: make(chan []int16, 2)}
	device := &muteDevice{probe: local}
	peer, err := rtc.NewAudioOnly(rtc.Media{Audio: remote}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	control := &fakeControl{event: make(chan connection.TalkEvent, 8), peer: peer, capable: true}
	session := NewSession(control, "agent:synthetic:kept", device)
	session.nativeMedia = false
	// Mute before the link exists must be applied at publication, not lost.
	session.SetInputMuted(true)
	if err := session.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	link := session.statsLink.Load()
	id := session.id
	if !link.Muted() {
		t.Fatal("setup lost encoder mute")
	}
	go func() {
		tick := time.NewTicker(20 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				pcm := make([]int16, rtc.FrameSamples)
				for i := range pcm {
					if i%32 < 16 {
						pcm[i] = 12000
					} else {
						pcm[i] = -12000
					}
				}
				for _, out := range []chan []int16{local.frames, remote.frames} {
					select {
					case out <- pcm:
					default:
					}
				}
			}
		}
	}()
	await(t, func() bool { return local.level.Load() > 1000 && remote.packets.Load() > 5 })
	session.SetInputMuted(false)
	await(t, func() bool { return remote.level.Load() > 1000 })
	for _, period := range []time.Duration{120 * time.Millisecond, 750 * time.Millisecond} {
		session.SetInputMuted(true)
		await(t, func() bool { return remote.level.Load() < 300 })
		before := local.packets.Load()
		time.Sleep(period)
		if !link.Muted() || local.packets.Load() <= before || local.level.Load() < 1000 || remote.level.Load() >= 300 {
			t.Fatal("mute stopped output or forwarded microphone")
		}
		if session.statsLink.Load() != link || session.id != id || session.Target != "agent:synthetic:kept" || control.createCalls != 1 || device.closed.Load() != 0 {
			t.Fatal("mute replaced session")
		}
		session.SetInputMuted(false)
		await(t, func() bool { return remote.level.Load() > 1000 })
	}
	session.Close()
	if control.closed.Load() != 1 || device.closed.Load() != 1 {
		t.Fatal("hangup did not close once")
	}
}
