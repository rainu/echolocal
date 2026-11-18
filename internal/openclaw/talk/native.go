package talk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/ygelfand/libcountertop/pkg/media/rtc"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type voiceTranscript struct{ role, text, id string }
type nativeState struct {
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
}

func newNative() *nativeState {
	return &nativeState{inbox: make(chan []byte, 64), done: make(chan struct{}), channelReady: make(chan struct{}), transcripts: make(chan voiceTranscript, 32), transcriptStop: make(chan struct{}), transcriptDone: make(chan struct{})}
}
func (s *Session) nativeFailure(reason string) { s.reason.Store(reason); s.fail() }

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
func (s *Session) nativeUserSpeech() {
	if s.native.inputActive.Swap(true) {
		return
	}
	s.responseEpoch.Add(1)
	// Clear only stale queued media at a provider-confirmed interruption. Future
	// live media remains enabled, including acknowledgements during consultation.
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
				DelegationItemID string `json:"delegation_item_id"`
				DelegationID     string `json:"delegation_id"`
				Item             struct{ ID, Target string }
				Delegation       struct{ ID, Target string }
				Turn             struct{ ID, Role, Transcript string }
			}
			if json.Unmarshal(raw, &e) != nil {
				s.nativeFailure("talk_control_invalid")
				return
			}
			switch e.Type {
			case "error":
				s.nativeFailure("talk_control_rejected")
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
					s.nativeUserSpeech()
				}
			case "session.input_transcript.delta":
				if !s.native.legacy.Load() {
					s.nativeUserSpeech()
				}
			case "output_audio_buffer.cleared":
				s.Audio.StopOutput()
			case "turn.done":
				s.native.legacy.Store(true)
				switch e.Turn.Role {
				case "user":
					s.native.inputActive.Store(false)
					if strings.TrimSpace(e.Turn.Transcript) == "" {
						if s.Activity != nil {
							s.Activity(Listening)
						}
						continue
					}
					s.nativeUserFinal()
					s.utterances.Add(1)
					s.appendTranscript(ctx, "user", e.Turn.Transcript, e.Turn.ID)
					if EndPhrase(e.Turn.Transcript) {
						if s.End != nil {
							go s.End()
						}
						return
					}
					s.activity(Thinking)
				case "assistant":
					s.native.responses.Add(1)
					s.appendTranscript(ctx, "assistant", e.Turn.Transcript, e.Turn.ID)
					go s.afterPlayback(ctx, s.responseEpoch.Load())
				}
			case "session.closed":
				if s.End != nil {
					go s.End()
				}
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
	out := make(chan []int16, 8)
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
