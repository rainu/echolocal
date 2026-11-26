package talk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/ygelfand/libcountertop/pkg/media/rtc"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type voiceTranscript struct{ role, text, id string }
type nativeState struct {
	errorCode, errorType atomic.Value
	deviceContext        map[string]any
	deviceGuardContext   map[string]any
	endPhraseContext     map[string]any
	sendDeviceContext    func(map[string]any) error
	deviceContextOnce    sync.Once

	legacy                                                                              atomic.Bool
	workCtx                                                                             context.Context
	workCancel                                                                          context.CancelFunc
	inbox                                                                               chan []byte
	done, channelReady                                                                  chan struct{}
	channelOnce                                                                         sync.Once
	transcripts                                                                         chan voiceTranscript
	transcriptStop, transcriptDone                                                      chan struct{}
	stopOnce                                                                            sync.Once
	inputActive                                                                         atomic.Bool
	responseAt                                                                          atomic.Int64
	firstPCMMS, maxFirstPCMMS                                                           atomic.Int64
	messages, delegations, results, responses, historyFailures, silence, timedResponses atomic.Uint64
	outputClears                                                                        atomic.Uint64
}

func newNative() *nativeState {
	return &nativeState{inbox: make(chan []byte, 64), done: make(chan struct{}), channelReady: make(chan struct{}), transcripts: make(chan voiceTranscript, 32), transcriptStop: make(chan struct{}), transcriptDone: make(chan struct{})}
}
func (s *Session) nativeFailure(reason string) { s.fail(reason) }

// Match the Control UI: Gateway creation supplies the selected agent's initial
// instructions and history. Opening oai-events does not send replacement
// instructions, force delegations, or gate the provider's live audio.
func (s *Session) controlLink(ctx context.Context) *rtc.ControlChannel {
	return &rtc.ControlChannel{Name: "oai-events", Opened: func() {
		s.native.channelOnce.Do(func() { close(s.native.channelReady) })
	}, Received: func(raw []byte) {
		if ctx.Err() != nil {
			return
		}
		select {
		case s.native.inbox <- append([]byte(nil), raw...):
		default:
			s.nativeFailure("talk_control_overflow")
		}
	}, Closed: func() {
		if ctx.Err() == nil {
			s.nativeFailure("talk_control_closed")
		}
	}}
}

// Configure before starting nativeLoop. Transport is loaded atomically; the
// context is sent only at the first provider-confirmed user speech event.
func (s *Session) configureDeviceContext(model string) {
	s.native.endPhraseContext = deviceContextAppend(model, endPhraseGuard(s.Ending.Phrase))
	if s.DeviceContext != "" {
		s.native.deviceGuardContext = deviceContextAppend(model, deviceContextGuard())
		s.native.deviceContext = deviceContextAppend(model, s.DeviceContext)
	}
	s.native.sendDeviceContext = func(event map[string]any) error {
		link := s.statsLink.Load()
		if link == nil {
			return errors.New("talk_device_context_unavailable")
		}
		return link.SendControl(event)
	}
}

func (s *Session) nativeUserSpeech() {
	if s.native.inputActive.Swap(true) {
		return
	}
	failed := false
	s.native.deviceContextOnce.Do(func() {
		// Guard first, then optional metadata; never replace provider instructions
		// or append policy to every transcript. A new activation owns a new once.
		for _, event := range []map[string]any{s.native.endPhraseContext, s.native.deviceGuardContext, s.native.deviceContext} {
			if event == nil {
				continue
			}
			if s.native.sendDeviceContext == nil || s.native.sendDeviceContext(event) != nil {
				failed = true
				s.nativeFailure("talk_device_context_unavailable")
				return
			}
			slog.Info("talk startup context sent on first user speech", "wire_type", event["type"])
		}
	})
	if failed {
		return
	}
	s.responseEpoch.Add(1)
	// Clear only stale queued media at a provider-confirmed interruption. Future
	// live media remains enabled, including acknowledgements during consultation.
	if s.Audio.Queued() > 0 {
		s.native.outputClears.Add(1)
	}
	s.Audio.StopOutput()
	if s.UserSpeech != nil {
		s.UserSpeech()
	}
}
func (s *Session) nativeUserFinal() {
	s.native.inputActive.Store(false)
	s.native.responseAt.Store(time.Now().UnixNano())
}

// Playback observes the first decoded audible PCM after a final user caption.
// This is not a claim about exact acoustic onset, VAD delay or device latency.
func (s *Session) Playback() {
	s.audibleFrames.Add(1)
	if at := s.native.responseAt.Swap(0); at > 0 {
		ms := max(int64(0), time.Since(time.Unix(0, at)).Milliseconds())
		s.native.firstPCMMS.Store(ms)
		s.native.timedResponses.Add(1)
		for old := s.native.maxFirstPCMMS.Load(); ms > old; old = s.native.maxFirstPCMMS.Load() {
			if s.native.maxFirstPCMMS.CompareAndSwap(old, ms) {
				break
			}
		}
	}
	if s.Activity != nil {
		s.Activity(Speaking)
	}
}
func (s *Session) nativeLoop(ctx context.Context) {
	defer close(s.native.done)
	defer s.clearPartial()
	active := map[string]bool{}
	// Only recent IDs are retained for duplicate telemetry; this is not a
	// lifetime turn limit and does not stop a long-running conversation.
	recent := map[string]bool{}
	order := []string{}
	for {
		select {
		case <-ctx.Done():
			return
		case raw := <-s.native.inbox:
			if ctx.Err() != nil {
				return
			}
			s.native.messages.Add(1)
			var e struct {
				Type             string
				Delta            string
				DelegationItemID string `json:"delegation_item_id"`
				DelegationID     string `json:"delegation_id"`
				Item             struct{ ID, Target, Text string }
				Delegation       struct{ ID, Target string }
				Turn             struct{ ID, Role, Transcript string }
			}
			if json.Unmarshal(raw, &e) != nil {
				s.nativeFailure("talk_control_invalid")
				return
			}
			switch e.Type {
			case "error", "session.closed", "session.created", "session.updated", "turn.created", "turn.done", "session.context.appended", "session.thinking.appended":
				slog.Info("talk native event", "type", e.Type)
			}
			switch e.Type {
			case "error":
				var detail providerErrorEvent
				if json.Unmarshal(raw, &detail) != nil {
					slog.Error("talk native provider error details malformed")
					s.nativeFailure("talk_control_invalid")
					return
				}
				reason, code, kind := classifyNativeProviderError(detail)
				s.native.errorCode.Store(code)
				s.native.errorType.Store(kind)
				fields := []any{"reason", reason}
				for _, term := range []string{"unknown", "unsupported", "invalid", "channel", "content", "session.context.append", "session.thinking.append", "delegation"} {
					fields = append(fields, "mentions_"+term, strings.Contains(strings.ToLower(detail.Error.Message), term))
				}
				for key, value := range map[string]string{"code": detail.Error.Code, "type": detail.Error.Type, "param": detail.Error.Param} {
					safe := safeProviderErrorToken(value)
					if safe != "" && safe != "other" {
						fields = append(fields, key, safe)
					} else {
						fields = append(fields, key+"_present", value != "")
					}
				}
				slog.Error("talk native provider error classified", fields...)
				s.nativeFailure(reason)
				return
			case "delegation.created", "session.delegation.created":
				id, target := e.Item.ID, e.Item.Target
				if e.Type == "session.delegation.created" {
					id, target = e.Delegation.ID, e.Delegation.Target
				}
				if id == "" || target != "client" || recent[id] {
					continue
				}
				if len(active) >= 128 {
					s.nativeFailure("talk_control_limit")
					return
				}
				if len(order) >= 128 {
					delete(recent, order[0])
					order = order[1:]
				}
				recent[id] = true
				order = append(order, id)
				active[id] = true
				s.native.delegations.Add(1)
				s.pendingTools.Store(int32(len(active)))
				s.activity(Thinking)
			case "delegation.context.appended", "session.commentary.appended":
				id := e.DelegationItemID
				if e.Type == "session.commentary.appended" {
					id = e.DelegationID
				}
				if !active[id] {
					continue
				}
				delete(active, id)
				s.pendingTools.Store(int32(len(active)))
				s.native.results.Add(1)
				if len(active) == 0 {
					go s.afterPlayback(ctx, s.responseEpoch.Load())
				}
			case "turn.created":
				s.native.legacy.Store(true)
				if e.Turn.Role == "user" {
					s.endTranscript.reset(e.Turn.ID)
					s.nativeUserSpeech()
				}
			case "session.input_transcript.delta":
				s.partial("user", e.Delta)
				// The first public delta can already contain the whole end
				// phrase. Send the once-only guard before immediate closure.
				if !s.native.legacy.Load() {
					s.nativeUserSpeech()
				}
				if s.userTranscript("user", e.Delta, "", false, false) {
					return
				}
			case "session.output_transcript.delta":
				s.partial("assistant", e.Delta)
			case "input_transcript.added", "output_transcript.added":
				role := "assistant"
				if e.Type == "input_transcript.added" {
					role = "user"
					s.nativeUserSpeech()
				}
				s.partial(role, e.Item.Text)
				if s.userTranscript(role, e.Item.Text, e.Item.ID, true, false) {
					return
				}
			case "output_audio_buffer.cleared":
				s.clearPartial()
				if s.Audio.Queued() > 0 {
					s.native.outputClears.Add(1)
				}
				s.Audio.StopOutput()
			case "turn.done":
				s.native.legacy.Store(true)
				switch e.Turn.Role {
				case "user":
					s.native.inputActive.Store(false)
					if strings.TrimSpace(e.Turn.Transcript) == "" {
						s.endTranscript.reset("")
						if s.Activity != nil {
							s.Activity(Listening)
						}
						continue
					}
					s.nativeUserFinal()
					s.utterances.Add(1)
					s.appendTranscript(ctx, "user", e.Turn.Transcript, e.Turn.ID)
					if s.userTranscript("user", e.Turn.Transcript, e.Turn.ID, true, true) {
						return
					}
					s.activity(Thinking)
				case "assistant":
					s.native.responses.Add(1)
					s.appendTranscript(ctx, "assistant", e.Turn.Transcript, e.Turn.ID)
					go s.afterPlayback(ctx, s.responseEpoch.Load())
				}
			case "session.closed":
				s.nativeFailure("talk_transport_closed")
				return
			}
		}
	}
}
func (s *Session) appendTranscript(ctx context.Context, role, text, turnID string) {
	if ctx.Err() != nil || strings.TrimSpace(text) == "" {
		return
	}
	if len(text) > 32768 {
		s.nativeFailure("talk_control_limit")
		return
	}
	if turnID == "" {
		turnID = s.id + "-" + role + "-" + time.Now().Format("150405.000000000")
	}
	hash := sha256.Sum256([]byte(s.id + "|" + role + "|" + turnID))
	item := voiceTranscript{role, text, "echo-" + hex.EncodeToString(hash[:16])}
	if s.Caption != nil {
		s.Caption(role, text, item.id)
	}
	select {
	case s.native.transcripts <- item:
	default:
		s.nativeFailure("talk_control_overflow")
	}
}
func (s *Session) transcriptLoop() {
	defer close(s.native.transcriptDone)
	seen := map[string]bool{}
	write := func(item voiceTranscript) {
		if s.native.workCtx.Err() != nil {
			return
		}
		if seen[item.id] {
			return
		}
		if len(seen) >= 512 {
			s.native.historyFailures.Add(1)
			return
		}
		seen[item.id] = true
		ctx, cancel := context.WithTimeout(s.native.workCtx, 5*time.Second)
		defer cancel()
		err := s.Client.CallTalk(ctx, "talk.client.transcript", map[string]any{"sessionKey": s.Target, "voiceSessionId": s.id, "entryId": item.id, "role": item.role, "text": item.text, "timestamp": time.Now().UnixMilli()}, nil)
		if err != nil {
			s.native.historyFailures.Add(1)
		}
	}
	for {
		select {
		case item := <-s.native.transcripts:
			write(item)
		case <-s.native.transcriptStop:
			for {
				select {
				case item := <-s.native.transcripts:
					write(item)
				default:
					return
				}
			}
		}
	}
}

// Silence advances the provider timeline while transport/audio readiness
// is pending. It never opens or reads a microphone.
type preReadyAudio struct {
	device Device
	ctx    context.Context
	gate   <-chan struct{}
	count  *atomic.Uint64
}

func (a preReadyAudio) Play(pcm []int16) { a.device.Play(pcm) }
func (a preReadyAudio) Frames() (<-chan []int16, func()) {
	ctx, cancel := context.WithCancel(a.ctx)
	out := make(chan []int16)
	frames, release := a.device.Frames()
	go func() {
		defer close(out)
		defer release()
		tick := time.NewTicker(20 * time.Millisecond)
		defer tick.Stop()
		ready := false
		for {
			if ready {
				select {
				case <-ctx.Done():
					return
				case f, ok := <-frames:
					if !ok {
						return
					}
					select {
					case out <- f:
					case <-ctx.Done():
						return
					}
				}
				continue
			}
			select {
			case <-ctx.Done():
				return
			case <-a.gate:
				ready = true
			case <-tick.C:
				a.count.Add(1)
				select {
				case out <- make([]int16, rtc.FrameSamples):
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, cancel
}
