package talk

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/pion/webrtc/v4"
	connection "github.com/ygelfand/echolocal/internal/openclaw"
	"github.com/ygelfand/libcountertop/pkg/media/rtc"
)

type Device interface {
	rtc.Audio
	Start(context.Context) error
	Close() error
	Queued() int
	StopOutput()
	ReadyTone()
}
type Control interface {
	SubscribeTalk() (<-chan connection.TalkEvent, func(), error)
	CallTalk(context.Context, string, any, any) error
	ExchangeOffer(context.Context, connection.TalkLaunch, string) (string, error)
}
type Session struct {
	native        *nativeState
	nativeEnabled atomic.Bool
	nativeMedia   bool
	audioGate     chan struct{}

	Client            Control
	Target            string
	Provider, Model   string
	Audio             Device
	Activity          func(State)
	UserSpeech        func()
	End, Failure      func()
	mu                sync.Mutex
	id                string
	cancel            context.CancelFunc
	unsubscribe       func()
	link              *rtc.Link
	once              sync.Once
	closeErr          error
	ready             chan struct{}
	failed            chan struct{}
	statsLink         atomic.Pointer[rtc.Link]
	stage             atomic.Int32
	setupMS           atomic.Int64
	reason            atomic.Value
	pendingTools      atomic.Int32
	tools, utterances atomic.Uint64
	responseEpoch     atomic.Uint64
	providerReady     sync.Once
	rejected          sync.Once
}

func NewSession(c Control, target string, a Device) *Session {
	raw := make([]byte, 16)
	_, _ = rand.Read(raw)
	return &Session{native: newNative(), nativeMedia: true, audioGate: make(chan struct{}), Client: c, Target: target, Audio: a, id: "echo-" + hex.EncodeToString(raw), ready: make(chan struct{}), failed: make(chan struct{})}
}
func (s *Session) fail() {
	s.rejected.Do(func() {
		close(s.failed)
		if s.Failure != nil {
			go s.Failure()
		}
	})
}
func (s *Session) Open(parent context.Context) (openErr error) {
	defer func() {
		if openErr != nil {
			s.reason.Store(safeReason(openErr))
		}
	}()
	s.mu.Lock()
	defer s.mu.Unlock()
	began := time.Now()
	defer func() { s.setupMS.Store(time.Since(began).Milliseconds()) }()
	s.stage.Store(1)
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	startup, done := context.WithTimeout(ctx, 45*time.Second)
	defer done()
	events, unsubscribe, err := s.Client.SubscribeTalk()
	if err != nil {
		return errors.New("talk_control_unavailable")
	}
	s.unsubscribe = unsubscribe
	go s.control(ctx, events)
	// Creation is the authoritative capability/authentication check; no fallback.
	s.stage.Store(3)
	caps := []string{"voice-transcript", "voice-selection"}
	if !s.nativeMedia {
		caps = []string{"gateway-control-v1"}
	}
	var launch connection.TalkLaunch
	request := map[string]any{"sessionKey": s.Target, "voiceSessionId": s.id, "mode": "realtime", "transport": "webrtc", "brain": "agent-consult", "capabilities": caps}
	if s.Provider != "" {
		request["provider"] = s.Provider
	}
	if s.Model != "" {
		request["model"] = s.Model
	}
	err = s.Client.CallTalk(startup, "talk.client.create", request, &launch)
	if err != nil {
		return err
	}
	if launch.VoiceSessionID != s.id || launch.Transport != "webrtc" {
		return errors.New("talk_contract_invalid")
	}
	if s.nativeMedia {
		if !launch.ClientOwned() || launch.Provider != "openai" {
			return errors.New("talk_contract_invalid")
		}
		s.native.workCtx, s.native.workCancel = context.WithCancel(context.Background())
		s.nativeEnabled.Store(true)
		go s.nativeLoop(ctx)
		go s.transcriptLoop()
	} else if launch.ClientControl.Owner != "gateway" {
		return errors.New("talk_contract_invalid")
	}
	s.stage.Store(2)
	var catalogue struct {
		Realtime struct {
			Ready          bool
			ActiveProvider string
			Providers      []struct {
				ID                 string
				Configured         bool
				SupportsToolCalls  *bool
				Transports, Brains []string
			}
		}
	}
	for attempt := 0; attempt < 8; attempt++ {
		catalogParams := map[string]any{"sessionKey": s.Target}
		if s.Provider != "" {
			catalogParams["provider"] = s.Provider
		}
		if s.Model != "" {
			catalogParams["model"] = s.Model
		}
		err = s.Client.CallTalk(startup, "talk.catalog", catalogParams, &catalogue)
		if err == nil {
			break
		}
		var rejected *connection.RPCRejection
		if !errors.As(err, &rejected) || !rejected.Retryable {
			return err
		}
		if attempt == 7 {
			return err
		}
		wait := time.NewTimer(time.Duration(250*(1<<min(attempt, 2))) * time.Millisecond)
		select {
		case <-wait.C:
		case <-startup.Done():
			wait.Stop()
			return startup.Err()
		}
	}
	capable := false
	for _, provider := range catalogue.Realtime.Providers {
		if provider.ID == catalogue.Realtime.ActiveProvider && provider.Configured && has(provider.Transports, "webrtc") && has(provider.Brains, "agent-consult") {
			// Native consultation is Gateway-owned. Generic client tool-call
			// protocols require a different adapter, not guessed model names.
			capable = !s.nativeMedia || (provider.SupportsToolCalls != nil && !*provider.SupportsToolCalls)
		}
	}
	if !catalogue.Realtime.Ready || !capable {
		return errors.New("talk_capability_unavailable")
	}
	connected := make(chan struct{})
	var once sync.Once
	constructor := rtc.NewAudioOnly
	var audio rtc.Audio = s.Audio
	media := rtc.Media{}
	if s.nativeMedia {
		constructor = rtc.NewControlledAudio
		audio = preReadyAudio{device: s.Audio, ctx: ctx, gate: s.audioGate, count: &s.native.silence}
		media.Control = s.controlLink(ctx)
	}
	media.Audio = audio
	link, err := constructor(media, func(state webrtc.PeerConnectionState) {
		switch state {
		case webrtc.PeerConnectionStateConnected:
			once.Do(func() { close(connected) })
		case webrtc.PeerConnectionStateDisconnected, webrtc.PeerConnectionStateFailed:
			s.fail()
		}
	})
	if err != nil {
		return errors.New("talk_peer_failed")
	}
	s.link = link
	s.statsLink.Store(link)
	s.stage.Store(4)
	offer, err := link.Offer(startup)
	if err != nil {
		return errors.New("talk_offer_failed")
	}
	answer, err := s.Client.ExchangeOffer(startup, launch, offer)
	if err != nil {
		return err
	}
	if err = link.Answered(answer); err != nil {
		return errors.New("talk_answer_failed")
	}
	s.stage.Store(5)
	signals := []<-chan struct{}{connected, s.ready}
	if s.nativeMedia {
		signals = append(signals, s.native.channelReady)
	}
	for _, signal := range signals {
		select {
		case <-signal:
		case <-s.failed:
			return errors.New("talk_provider_failed")
		case <-startup.Done():
			return errors.New("talk_start_timeout")
		}
	}
	s.stage.Store(6)
	if err = s.Audio.Start(ctx); err != nil {
		return errors.New("talk_audio_unavailable")
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	s.stage.Store(7)
	if s.nativeMedia {
		close(s.audioGate)
	}
	s.Audio.ReadyTone()
	return nil
}
func (s *Session) Close() error {
	s.once.Do(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.cancel != nil {
			s.cancel()
		}
		// Shut the microphone gate before any potentially slow network cleanup.
		s.Audio.Close()
		if s.unsubscribe != nil {
			s.unsubscribe()
		}
		if s.link != nil {
			s.link.Close()
		}
		if s.nativeEnabled.Load() {
			select {
			case <-s.native.done:
			case <-time.After(time.Second):
			}
			s.native.stopOnce.Do(func() { close(s.native.transcriptStop) })
			select {
			case <-s.native.transcriptDone:
			case <-time.After(6 * time.Second):
				s.native.workCancel()
			}
			s.native.workCancel()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.Client.CallTalk(ctx, "talk.client.close", map[string]any{"sessionKey": s.Target, "voiceSessionId": s.id}, nil)
	})
	return s.closeErr
}
func (s *Session) control(ctx context.Context, events <-chan connection.TalkEvent) {
	pending := map[string]bool{}
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-events:
			if !ok {
				if ctx.Err() == nil {
					s.fail()
				}
				return
			}
			if e.VoiceSessionID != s.id {
				continue
			}
			if s.nativeEnabled.Load() && s.native.legacy.Load() && e.Event.Type != "session.ready" && e.Event.Type != "session.closed" && e.Event.Type != "session.error" && e.Event.Type != "session.replaced" {
				continue
			}
			switch e.Event.Type {
			case "session.ready":
				s.providerReady.Do(func() { close(s.ready) })
			case "session.error", "session.replaced":
				s.fail()
			case "session.closed":
				if ctx.Err() == nil && s.End != nil {
					go s.End()
				}
				return
			case "transcript.done":
				var p struct{ Role, Text string }
				if json.Unmarshal(e.Event.Payload, &p) != nil {
					continue
				}
				if p.Role == "assistant" {
					if s.nativeEnabled.Load() {
						s.native.responses.Add(1)
					}
					continue
				}
				if p.Role != "user" {
					continue
				}
				s.utterances.Add(1)
				if !s.nativeEnabled.Load() {
					s.Audio.StopOutput()
				}
				if EndPhrase(p.Text) {
					if s.End != nil {
						go s.End()
					}
					return
				}
				if s.nativeEnabled.Load() {
					s.nativeUserFinal()
				}
				s.activity(Thinking)
			case "tool.call", "tool.result", "tool.error":
				key := e.Event.CallID
				if key == "" {
					key = "uncorrelated"
				}
				if e.Event.Type == "tool.call" {
					if !pending[key] {
						s.tools.Add(1)
						pending[key] = true
					}
				} else {
					delete(pending, key)
				}
				s.pendingTools.Store(int32(len(pending)))
				s.activity(Thinking)
			case "turn.started":
				s.activity(Thinking)
			case "output.audio.started":
				s.activity(Speaking)
			case "turn.cancelled":
				s.Audio.StopOutput()
				if s.Activity != nil {
					s.Activity(Listening)
				}
			case "turn.ended", "output.audio.done":
				// A protocol end is not yet audible completion. Keep response ownership
				// until the existing speaker lane drains.
				go s.afterPlayback(ctx, s.responseEpoch.Load())
			}
		}
	}
}
func (s *Session) afterPlayback(ctx context.Context, epoch uint64) {
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	quiet := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if s.Audio.Queued() > 0 {
				quiet = time.Now()
				continue
			}
			if time.Since(quiet) > 400*time.Millisecond {
				if s.pendingTools.Load() > 0 {
					if s.Activity != nil {
						s.Activity(Thinking)
					}
					return
				}
				if s.responseEpoch.Load() == epoch && s.Activity != nil {
					s.Activity(Listening)
				}
				return
			}
		}
	}
}
func EndPhrase(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	text = strings.TrimFunc(text, func(r rune) bool { return unicode.IsPunct(r) || unicode.IsSpace(r) })
	switch text {
	case "end conversation", "stop conversation", "goodbye", "gespräch beenden", "unterhaltung beenden", "auf wiedersehen", "termina conversazione", "fine conversazione", "arrivederci", "kết thúc cuộc trò chuyện", "tạm biệt":
		return true
	}
	return false
}

func (s *Session) activity(state State) {
	s.responseEpoch.Add(1)
	if s.Activity != nil {
		s.Activity(state)
	}
}

type Diagnostics struct {
	ControlMessages, NativeDelegations, NativeResults, NativeResponses, HistoryFailures, StartupSilence, ResponsesTimed uint64
	FinalToFirstPCMMS, MaxFinalToFirstPCMMS                                                                             int64

	Stage                                   string
	Reason                                  string
	SetupMS                                 int64
	Sent, Received, Lost, Tools, Utterances uint64
}

func (s *Session) Diagnostics() Diagnostics {
	names := []string{"idle", "control", "catalog", "create", "offer", "connect", "audio", "ready"}
	n := int(s.stage.Load())
	if n < 0 || n >= len(names) {
		n = 0
	}
	d := Diagnostics{Stage: names[n], SetupMS: s.setupMS.Load(), Tools: s.tools.Load(), Utterances: s.utterances.Load()}
	d.ControlMessages = s.native.messages.Load()
	d.NativeDelegations = s.native.delegations.Load()
	d.NativeResults = s.native.results.Load()
	d.NativeResponses = s.native.responses.Load()
	d.ResponsesTimed = s.native.timedResponses.Load()
	d.FinalToFirstPCMMS = s.native.firstPCMMS.Load()
	d.MaxFinalToFirstPCMMS = s.native.maxFirstPCMMS.Load()
	d.HistoryFailures = s.native.historyFailures.Load()
	d.StartupSilence = s.native.silence.Load()
	if value := s.reason.Load(); value != nil {
		d.Reason = value.(string)
	}
	if l := s.statsLink.Load(); l != nil {
		stats := l.Stats()
		d.Sent = stats.AudioSent
		d.Received = stats.AudioRecv
		d.Lost = stats.AudioLost
	}
	return d
}

func has(items []string, want string) bool {
	for _, v := range items {
		if v == want {
			return true
		}
	}
	return false
}

func safeReason(err error) string {
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	switch err.Error() {
	case "talk_native_control_unsupported", "talk_control_failed", "talk_control_rejected", "talk_control_overflow", "rpc_rejected", "rpc_cancelled", "rpc_timeout", "rpc_busy", "rpc_disconnected", "rpc_delivery_uncertain", "rpc_invalid_response", "talk_control_unavailable", "talk_contract_invalid", "talk_capability_unavailable", "talk_peer_failed", "talk_offer_failed", "talk_offer_rejected", "talk_offer_origin", "talk_answer_failed", "talk_answer_invalid", "talk_provider_failed", "talk_start_timeout", "talk_audio_unavailable", "talk_connection_changed", "talk_launch_invalid":
		return err.Error()
	}
	return "talk_failed"
}
