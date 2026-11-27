package talk

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

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
	ReadyTone(context.Context) error
}
type Control interface {
	SubscribeTalk() (<-chan connection.TalkEvent, func(), error)
	CallTalk(context.Context, string, any, any) error
	ExchangeOffer(context.Context, connection.TalkLaunch, string) (string, error)
}
type Session struct {
	Ending                    EndSettings
	endTranscript             endTranscript
	endOnce                   sync.Once
	inputDeltas, outputDeltas atomic.Uint64
	native                    *nativeState
	nativeEnabled             atomic.Bool
	nativeMedia               bool
	audioGate                 chan struct{}

	Client            Control
	Target            string
	Provider, Model   string
	runtimeModel      string
	runtimeProvider   string
	intentionalEnd    atomic.Bool
	audibleFrames     atomic.Uint64
	Greeting          string
	DeviceContext     string
	Audio             Device
	Activity          func(State)
	Caption           func(role, text, id string)
	PartialCaption    func(role, delta string)
	ClearPartial      func()
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
	failureReason     atomic.Value
	pendingTools      atomic.Int32
	tools, utterances atomic.Uint64
	responseEpoch     atomic.Uint64
	inputMu           sync.Mutex
	inputMuted        atomic.Bool
	providerReady     sync.Once
	rejected          sync.Once
}

func NewSession(c Control, target string, a Device) *Session {
	raw := make([]byte, 16)
	_, _ = rand.Read(raw)
	return &Session{native: newNative(), nativeMedia: true, audioGate: make(chan struct{}), Client: c, Target: target, Audio: a, id: "echo-" + hex.EncodeToString(raw), ready: make(chan struct{}), failed: make(chan struct{})}
}
func (s *Session) fail(reason string) {
	s.rejected.Do(func() {
		s.reason.Store(reason)
		s.failureReason.Store(reason)
		close(s.failed)
		if s.Failure != nil {
			go s.Failure()
		}
	})
}
func (s *Session) Open(parent context.Context) (openErr error) {
	s.Ending = s.Ending.Defaults("en")
	if err := s.Ending.Validate(); err != nil {
		return err
	}
	began := time.Now()
	slog.Info("talk setup started", "provider", s.Provider, "model", s.Model, "native_media", s.nativeMedia, "device_context_present", s.DeviceContext != "")
	defer func() {
		elapsed := time.Since(began).Milliseconds()
		s.setupMS.Store(elapsed)
		if openErr != nil {
			reason := safeReason(openErr)
			s.reason.Store(reason)
			slog.Error("talk setup failed", "stage", s.stage.Load(), "elapsed_ms", elapsed, "reason", reason)
		} else {
			slog.Info("talk setup ready", "elapsed_ms", elapsed)
		}
	}()
	s.mu.Lock()
	defer s.mu.Unlock()
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
	s.runtimeModel = launch.Model
	s.runtimeProvider = launch.Provider
	slog.Info("talk session created", "provider", launch.Provider, "model", launch.Model, "transport", launch.Transport, "native_media", s.nativeMedia)
	if launch.VoiceSessionID != s.id || launch.Transport != "webrtc" {
		return errors.New("talk_contract_invalid")
	}
	if s.nativeMedia {
		if !launch.ClientOwned() || launch.Provider != "openai" {
			return errors.New("talk_contract_invalid")
		}
		s.native.workCtx, s.native.workCancel = context.WithCancel(context.Background())
		s.configureDeviceContext(launch.Model)
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
		case webrtc.PeerConnectionStateDisconnected, webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateClosed:
			if ctx.Err() == nil {
				s.fail("talk_peer_failed")
			}
		}
	})
	if err != nil {
		return errors.New("talk_peer_failed")
	}
	s.link = link
	s.inputMu.Lock()
	s.statsLink.Store(link)
	link.Mute(s.inputMuted.Load())
	s.inputMu.Unlock()
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
	// Do not append context at startup: even silent provider context can resume
	// a historical turn. Native user-speech handling sends it once when needed.
	s.stage.Store(6)
	if err = s.Audio.Start(startup); err != nil {
		return errors.New("talk_audio_unavailable")
	}
	slog.Info("talk microphone capture started")
	if ctx.Err() != nil {
		return ctx.Err()
	}
	s.stage.Store(7)
	if err = s.playReadyTone(startup); err != nil {
		if startup.Err() != nil {
			return errors.New("talk_start_timeout")
		}
		return errors.New("talk_audio_unavailable")
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if s.nativeMedia {
		// The greeting is explicit speakable context, not a fake user message or
		// replacement instructions. Send only after readiness playback succeeds.
		if s.Greeting != "" {
			if err = s.link.SendControl(greetingAppend(launch.Model, s.Greeting)); err != nil {
				return errors.New("talk_control_failed")
			}
		}
		close(s.audioGate)
	}
	return nil
}

// SetInputMuted gates the actual encoder, including already queued PCM.
// It is independent of output, provider context and transport cancellation.
func (s *Session) SetInputMuted(on bool) {
	s.inputMu.Lock()
	defer s.inputMu.Unlock()
	s.inputMuted.Store(on)
	if link := s.statsLink.Load(); link != nil {
		link.Mute(on)
	}
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
	defer s.clearPartial()
	pending := map[string]bool{}
	for {
		select {
		case <-ctx.Done():
			slog.Info("talk control stream stopped", "reason", safeReason(ctx.Err()))
			return
		case e, ok := <-events:
			if !ok {
				if ctx.Err() == nil {
					slog.Error("talk control stream closed unexpectedly")
					s.fail("control_stream_closed")
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
				slog.Info("talk provider event", "type", e.Event.Type)
				s.providerReady.Do(func() { close(s.ready) })
			case "session.error", "session.replaced":
				reason := "provider_session_error"
				if e.Event.Type == "session.replaced" {
					reason = "provider_session_replaced"
				} else if e.FailureReason == "provider_input_unconfirmed" {
					reason = e.FailureReason
				}
				if e.Event.Type == "session.error" && s.nativeEnabled.Load() {
					// The parallel native channel owns the structured error.
					// Block this control loop briefly so a following generic
					// close cannot win over an auth/permanent verdict either.
					timer := time.NewTimer(providerErrorDetailWait)
					select {
					case <-ctx.Done():
						timer.Stop()
						return
					case <-s.failed:
						timer.Stop()
						return
					case <-timer.C:
					}
				}
				slog.Error("talk provider event", "type", e.Event.Type, "reason", reason)
				s.fail(reason)
			case "session.closed":
				slog.Info("talk provider event", "type", e.Event.Type)
				if ctx.Err() == nil {
					s.fail("talk_transport_closed")
				}
				return
			case "transcript.delta", "output.text.delta":
				// The native channel owns partials when active; Gateway finals remain
				// authoritative on public Live transports.
				if s.nativeEnabled.Load() {
					continue
				}
				var p struct{ Role, Text, Delta, ID string }
				if json.Unmarshal(e.Event.Payload, &p) != nil {
					continue
				}
				if e.Event.Type == "output.text.delta" {
					p.Role = "assistant"
				}
				if p.Text == "" {
					p.Text = p.Delta
				}
				s.partial(p.Role, p.Text)
				if s.userTranscript(p.Role, p.Text, p.ID, false, false) {
					return
				}
			case "transcript.updated":
				// A complete interim revision is authoritative, never another delta.
				var p struct{ Role, Text, ID string }
				if !s.nativeEnabled.Load() && json.Unmarshal(e.Event.Payload, &p) == nil {
					if s.userTranscript(p.Role, p.Text, p.ID, true, false) {
						return
					}
				}
			case "transcript.done", "output.text.done":
				var p struct{ Role, Text, ID string }
				if json.Unmarshal(e.Event.Payload, &p) != nil {
					slog.Warn("talk transcript event malformed")
					continue
				}
				if e.Event.Type == "output.text.done" {
					p.Role = "assistant"
				}
				slog.Info("talk transcript event", "role", p.Role)
				if p.Role == "assistant" {
					if s.Caption != nil {
						s.Caption(p.Role, p.Text, "")
					}
					if s.nativeEnabled.Load() {
						s.native.responses.Add(1)
					}
					continue
				}
				if p.Role != "user" {
					continue
				}
				s.utterances.Add(1)
				if s.Caption != nil {
					s.Caption(p.Role, p.Text, "")
				}
				if !s.nativeEnabled.Load() {
					s.Audio.StopOutput()
				}
				if s.userTranscript(p.Role, p.Text, p.ID, true, true) {
					return
				}
				if s.nativeEnabled.Load() {
					s.nativeUserFinal()
				}
				s.activity(Thinking)
			case "tool.call", "tool.result", "tool.error":
				slog.Info("talk provider event", "type", e.Event.Type)
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
				slog.Info("talk provider event", "type", e.Event.Type)
				s.activity(Thinking)
			case "output.audio.started":
				slog.Info("talk provider event", "type", e.Event.Type)
				s.activity(Speaking)
			case "turn.cancelled":
				s.clearPartial()
				slog.Info("talk provider event", "type", e.Event.Type)
				s.Audio.StopOutput()
				if s.Activity != nil {
					s.Activity(Listening)
				}
			case "turn.ended", "output.audio.done":
				slog.Info("talk provider event", "type", e.Event.Type)
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

func (s *Session) activity(state State) {
	s.responseEpoch.Add(1)
	if s.Activity != nil {
		s.Activity(state)
	}
}

type Diagnostics struct {
	ProviderErrorCode, ProviderErrorType                                                                                string
	ControlMessages, NativeDelegations, NativeResults, NativeResponses, HistoryFailures, StartupSilence, ResponsesTimed uint64
	InputDeltas, OutputDeltas                                                                                           uint64
	OutputQueueClears                                                                                                   uint64
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
	if value := s.native.errorCode.Load(); value != nil {
		d.ProviderErrorCode = value.(string)
	}
	if value := s.native.errorType.Load(); value != nil {
		d.ProviderErrorType = value.(string)
	}
	d.ControlMessages = s.native.messages.Load()
	d.NativeDelegations = s.native.delegations.Load()
	d.NativeResults = s.native.results.Load()
	d.NativeResponses = s.native.responses.Load()
	d.InputDeltas = s.inputDeltas.Load()
	d.OutputDeltas = s.outputDeltas.Load()
	d.ResponsesTimed = s.native.timedResponses.Load()
	d.OutputQueueClears = s.native.outputClears.Load()
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
	case "talk_native_control_unsupported", "talk_control_failed", "talk_control_rejected", "talk_control_overflow", "rpc_rejected", "rpc_cancelled", "rpc_timeout", "rpc_busy", "rpc_disconnected", "rpc_delivery_uncertain", "rpc_invalid_response", "talk_control_unavailable", "talk_contract_invalid", "talk_capability_unavailable", "talk_peer_failed", "talk_offer_failed", "talk_offer_rejected", "talk_offer_origin", "talk_answer_failed", "talk_answer_invalid", "talk_provider_failed", "talk_start_timeout", "talk_audio_unavailable", "talk_connection_changed", "talk_launch_invalid", "talk_device_context_unavailable", "control_stream_closed":
		return err.Error()
	}
	return "talk_failed"
}

func (s *Session) partial(role, delta string) {
	if delta != "" {
		if role == "user" {
			s.inputDeltas.Add(1)
		}
		if role == "assistant" {
			s.outputDeltas.Add(1)
		}
	}
	if s.PartialCaption != nil && (role == "user" || role == "assistant") && delta != "" {
		s.PartialCaption(role, delta)
	}
}
func (s *Session) clearPartial() {
	if s.ClearPartial != nil {
		s.ClearPartial()
	}
}

// Only transport/setup availability and known uncertain-delivery failures are
// retried with a fresh voice transport, never with an input replay. Replaced
// sessions, authentication, invalid contracts, unknown provider errors and audio
// failures stop.
func (s *Session) RecoverableFailure() bool {
	if s.intentionalEnd.Load() {
		return false
	}
	reason := s.Diagnostics().Reason
	if first := s.failureReason.Load(); first != nil {
		reason = first.(string)
	}
	switch reason {
	case "talk_peer_failed", "talk_control_closed", "control_stream_closed",
		"talk_transport_closed", "rpc_disconnected", "rpc_timeout", "talk_start_timeout",
		"provider_input_unconfirmed", "provider_transient_error", "provider_session_expired", "provider_rate_limited":
		return true
	}
	return false
}

// Replacement is called only after this transport has closed. Routing and
// per-Talk settings survive; callbacks, pending media and the greeting do not.
func (s *Session) Replacement(audio Device) *Session {
	next := NewSession(s.Client, s.Target, audio)
	next.Provider, next.Model = s.Provider, s.Model
	if s.runtimeProvider != "" {
		next.Provider = s.runtimeProvider
	}
	if s.runtimeModel != "" {
		next.Model = s.runtimeModel
	}
	next.Ending, next.DeviceContext = s.Ending, s.DeviceContext
	next.nativeMedia = s.nativeMedia
	next.SetInputMuted(s.inputMuted.Load())
	return next
}
func (s *Session) IntentionalEnd() bool { return s.intentionalEnd.Load() }

// InterruptTransport is a local diagnostic fault injection: close only WebRTC,
// leaving the logical activation alive to exercise unexpected-loss handling.
func (s *Session) InterruptTransport() error {
	if s.stage.Load() != 7 {
		return errors.New("talk_not_ready")
	}
	link := s.statsLink.Load()
	if link == nil {
		return errors.New("talk_not_ready")
	}
	return link.Close()
}

// RecoveryFailure returns only locally bounded error classes, never a provider
// message, identifier, request body or authentication material.
func (s *Session) RecoveryFailure() (reason, code, kind string) {
	d := s.Diagnostics()
	reason, code, kind = d.Reason, d.ProviderErrorCode, d.ProviderErrorType
	if first := s.failureReason.Load(); first != nil {
		reason = first.(string)
	}
	return
}

// InterruptNativeProvider is a fixed local diagnostic fault, never a user turn
// or arbitrary control payload. It exercises the active native error receive
// queue and the logical Talk's real provider replacement path.
func (s *Session) InterruptNativeProvider() error {
	if s.stage.Load() != 7 || !s.nativeEnabled.Load() {
		return errors.New("talk_not_ready")
	}
	select {
	case <-s.failed:
		return errors.New("talk_not_ready")
	default:
	}
	select {
	case s.native.inbox <- []byte(`{"type":"error","error":{"type":"server_error","code":"server_error"}}`):
		return nil
	default:
		return errors.New("talk_control_busy")
	}
}

// playReadyTone cancels cue playback/draining when provider readiness is lost.
// A transport which fails during feedback must never open its capture gate.
func (s *Session) playReadyTone(ctx context.Context) error {
	select {
	case <-s.failed:
		return errors.New("talk_provider_failed")
	default:
	}
	ready, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		select {
		case <-s.failed:
			cancel()
		case <-ready.Done():
		}
	}()
	err := s.Audio.ReadyTone(ready)
	cancel()
	<-done
	select {
	case <-s.failed:
		return errors.New("talk_provider_failed")
	default:
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}
