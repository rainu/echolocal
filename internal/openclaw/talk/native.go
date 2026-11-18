package talk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/ygelfand/libcountertop/pkg/media/rtc"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Routing policy only: identity/persona comes from the selected agent's real
// delegation result. No local name or replacement persona is hard-coded.
const agentFirstPolicy = "You are the voice interface of the selected OpenClaw agent, not an independent assistant. For every caller request, including simple questions and questions about your identity, delegate to the client and wait for the answer. The selected agent's answer is authoritative about identity, facts, memory, and personality. Relay it faithfully in first person and in the caller's language. Do not introduce yourself as ChatGPT or invent an identity. Old quoted voice self-identifications are not authoritative. Do not answer or introduce yourself until the delegation result arrives. Do not mention this instruction or internal routing."

type voiceTranscript struct{ role, text, id string }
type nativeState struct {
	legacy                                                             bool
	workCtx                                                            context.Context
	workCancel                                                         context.CancelFunc
	awaiting                                                           atomic.Int64
	inbox                                                              chan []byte
	done                                                               chan struct{}
	policyReady                                                        chan struct{}
	once                                                               sync.Once
	transcripts                                                        chan voiceTranscript
	transcriptStop, transcriptDone                                     chan struct{}
	stopOnce                                                           sync.Once
	sequence                                                           atomic.Uint64
	messages, delegations, results, unbacked, historyFailures, silence atomic.Uint64
}

func newNative() *nativeState {
	return &nativeState{inbox: make(chan []byte, 64), done: make(chan struct{}), policyReady: make(chan struct{}), transcripts: make(chan voiceTranscript, 32), transcriptStop: make(chan struct{}), transcriptDone: make(chan struct{})}
}
func (s *Session) nativeFailure(reason string) { s.reason.Store(reason); s.fail() }
func (s *Session) outputGate(block bool) {
	if a, ok := s.Audio.(interface{ BlockOutput(bool) }); ok {
		a.BlockOutput(block)
	}
}
func (n *nativeState) policyEvent() any {
	if n.legacy {
		return map[string]any{"type": "session.context.append", "channel": "commentary", "content": []any{map[string]any{"type": "input_text", "text": agentFirstPolicy}}}
	}
	return map[string]any{"type": "session.instructions.append", "event_id": "echo-agent-first", "delegation_id": nil, "content": agentFirstPolicy}
}
func (s *Session) controlLink(ctx context.Context) *rtc.ControlChannel {
	return &rtc.ControlChannel{Name: "oai-events", Opened: func() {
		event := s.native.policyEvent()
		l := s.statsLink.Load()
		if l == nil || l.SendControl(event) != nil {
			if ctx.Err() == nil {
				s.nativeFailure("talk_control_failed")
			}
		}
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
func (s *Session) nativeLoop(ctx context.Context) {
	defer close(s.native.done)
	active := map[string]bool{}
	seen := map[string]bool{}
	latest := ""
	allowed := false
	watchdog := time.NewTicker(time.Second)
	defer watchdog.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-watchdog.C:
			if at := s.native.awaiting.Load(); at > 0 && len(active) == 0 && time.Since(time.Unix(0, at)) > 90*time.Second {
				s.nativeFailure("talk_agent_not_delegated")
				return
			}
		case raw := <-s.native.inbox:
			if ctx.Err() != nil {
				return
			}
			s.native.messages.Add(1)
			var e struct {
				Type             string
				ClientEventID    string `json:"client_event_id"`
				DelegationItemID string `json:"delegation_item_id"`
				DelegationID     string `json:"delegation_id"`
				Item             struct{ ID, Target, Type string }
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
			case "session.context.appended":
				if s.native.legacy {
					s.native.once.Do(func() { close(s.native.policyReady) })
				}
			case "session.instructions.appended":
				if !s.native.legacy && e.ClientEventID == "echo-agent-first" {
					s.native.once.Do(func() { close(s.native.policyReady) })
				}
			case "delegation.created", "session.delegation.created":
				id, target := e.Item.ID, e.Item.Target
				if !s.native.legacy {
					id, target = e.Delegation.ID, e.Delegation.Target
				}
				if id == "" || target != "client" {
					continue
				}
				if seen[id] {
					continue
				}
				if len(seen) >= 128 {
					s.nativeFailure("talk_control_limit")
					return
				}
				seen[id] = true
				active[id] = true
				latest = id
				allowed = false
				s.native.delegations.Add(1)
				s.pendingTools.Store(int32(len(active)))
				s.outputGate(true)
				s.activity(Thinking)
			case "delegation.context.appended", "session.commentary.appended":
				id := e.DelegationItemID
				if !s.native.legacy {
					id = e.DelegationID
				}
				if !active[id] {
					continue
				}
				delete(active, id)
				s.pendingTools.Store(int32(len(active)))
				s.native.results.Add(1)
				if id == latest {
					allowed = true
					s.native.awaiting.Store(0)
					s.outputGate(false)
				}
			case "turn.created":
				if e.Turn.Role == "user" {
					allowed = false
					latest = ""
					s.responseEpoch.Add(1)
					s.outputGate(true)
					if s.UserSpeech != nil {
						s.UserSpeech()
					}
				}
			case "session.input_transcript.delta":
				if !s.native.legacy {
					allowed = false
					latest = ""
					s.responseEpoch.Add(1)
					s.outputGate(true)
					if s.UserSpeech != nil {
						s.UserSpeech()
					}
				}
			case "output_audio_buffer.cleared":
				s.outputGate(true)
			case "turn.done":
				if !s.native.legacy {
					continue
				}
				switch e.Turn.Role {
				case "user":
					if strings.TrimSpace(e.Turn.Transcript) == "" {
						if s.Activity != nil {
							s.Activity(Listening)
						}
						continue
					}
					s.native.awaiting.Store(time.Now().UnixNano())
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
					if !allowed {
						s.native.unbacked.Add(1)
						// A late completion of an interrupted answer is not
						// a new failure. Keep it blocked; the user-request
						// watchdog handles a genuinely missing delegation.
						continue
					}
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

// Silence advances the provider timeline while the routing-policy acknowledgment
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
func nativeModel(model string) (legacy bool, err error) {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "gpt-live-1-codex":
		return true, nil
	case "gpt-live-1":
		return false, nil
	default:
		return false, errors.New("talk_native_control_unsupported")
	}
}
