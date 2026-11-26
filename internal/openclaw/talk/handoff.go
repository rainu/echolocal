package talk

import (
	"context"
	"errors"
	"time"
)

// AnnounceHandoff uses the existing provider control channel, not synthesized
// microphone input. It never closes Talk. Missing output/turn completion leaves
// the existing conversation intact; a timer is not proof of audible completion.
func (s *Session) AnnounceHandoff(ctx context.Context, text string) error {
	if !s.nativeEnabled.Load() {
		return errors.New("talk_transfer_unsupported")
	}
	s.mu.Lock()
	link, model := s.link, s.runtimeModel
	s.mu.Unlock()
	if link == nil {
		return errors.New("talk_connection_unavailable")
	}
	return s.announceHandoff(ctx, func() error { return link.SendControl(greetingAppend(model, text)) })
}

func (s *Session) announceHandoff(ctx context.Context, send func() error) error {
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	quiet := time.Now()
	// Wait for the consultation/result and any existing output before issuing a
	// dedicated speakable confirmation. Never cut the Node result/agent response.
	for {
		if s.pendingTools.Load() != 0 || s.Audio.Queued() > 0 {
			quiet = time.Now()
		}
		if time.Since(quiet) >= 600*time.Millisecond {
			break
		}
		select {
		case <-ctx.Done():
			return errors.New("talk_confirmation_timeout")
		case <-tick.C:
		}
	}
	responses, pcm := s.native.responses.Load(), s.audibleFrames.Load()
	if ctx.Err() != nil {
		return errors.New("talk_confirmation_timeout")
	}
	if err := send(); err != nil {
		return errors.New("talk_confirmation_failed")
	}
	quiet = time.Now()
	lastPCM := pcm
	for {
		select {
		case <-ctx.Done():
			return errors.New("talk_confirmation_timeout")
		case <-tick.C:
		}
		currentPCM := s.audibleFrames.Load()
		if currentPCM != lastPCM || s.Audio.Queued() > 0 || s.pendingTools.Load() > 0 {
			quiet = time.Now()
			lastPCM = currentPCM
		}
		if s.native.responses.Load() > responses && currentPCM > pcm &&
			s.pendingTools.Load() == 0 && s.Audio.Queued() == 0 &&
			time.Since(quiet) >= 700*time.Millisecond {
			return nil
		}
	}
}
