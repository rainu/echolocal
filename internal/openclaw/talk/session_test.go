package talk

import (
	"context"
	"encoding/json"
	"errors"
	connection "github.com/ygelfand/echolocal/internal/openclaw"
	"github.com/ygelfand/libcountertop/pkg/media/rtc"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeDevice struct {
	started, closed atomic.Int32
	frames          chan []int16
}

func (d *fakeDevice) Start(context.Context) error      { d.started.Add(1); return nil }
func (d *fakeDevice) Close() error                     { d.closed.Add(1); return nil }
func (d *fakeDevice) Frames() (<-chan []int16, func()) { return d.frames, func() {} }
func (d *fakeDevice) Play([]int16)                     {}
func (d *fakeDevice) Queued() int                      { return 0 }
func (d *fakeDevice) StopOutput()                      {}
func (d *fakeDevice) ReadyTone()                       {}

type fakeControl struct {
	mu                          sync.Mutex
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
		f.event <- event
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
	if d.started.Load() != 1 || session.Diagnostics().Stage != "ready" {
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
