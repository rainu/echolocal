package talk

import (
	"context"
	connection "github.com/ygelfand/echolocal/internal/openclaw"
	"time"
)

// AgentActivity describes verified session work, independently of microphone,
// voice playback and the provider's local response state. Unknown is not idle.
type AgentActivity struct{ Available, Working bool }
type activitySample struct {
	AgentActivity
	at    time.Time
	title string
}
type activityReader interface {
	ReadSessionActivity(context.Context, string) (connection.SessionActivity, error)
}

const activityInterval = time.Second
const activityReadTimeout = 2 * time.Second
const activityFreshness = 4 * time.Second

func (s *Session) AgentActivity() AgentActivity {
	if s.activityStopped.Load() {
		return AgentActivity{}
	}
	// Native voice consultations have their own observed request/result lifecycle.
	// A provider acknowledgment or speech alone is not work; an outstanding
	// client delegation is. Session metadata may report idle during this path.
	if s.nativeEnabled.Load() && s.native.activeDelegations.Load() > 0 {
		return AgentActivity{Available: true, Working: true}
	}
	sample := s.agentActivity.Load()
	if sample == nil || time.Since(sample.at) > activityFreshness {
		return AgentActivity{}
	}
	return sample.AgentActivity
}

// Each activation owns a fresh observer. A replaced/reset session incarnation is
// never silently adopted, even if it uses the same routing key. Reads are bounded,
// serialized and observational; failed or missing metadata clears the display.
func (s *Session) observeActivity(ctx context.Context) {
	reader, ok := s.Client.(activityReader)
	if !ok {
		return
	}
	s.pollActivity(ctx, reader, activityInterval)
}
func (s *Session) pollActivity(ctx context.Context, reader activityReader, interval time.Duration) {
	defer s.agentActivity.Store(nil)
	var incarnation string
	for {
		if ctx.Err() != nil || s.activityStopped.Load() {
			return
		}
		readCtx, cancel := context.WithTimeout(ctx, activityReadTimeout)
		row, err := reader.ReadSessionActivity(readCtx, s.Target)
		cancel()
		if ctx.Err() != nil || s.activityStopped.Load() {
			return
		}
		if err == nil && row.SessionID != "" {
			if incarnation == "" {
				incarnation = row.SessionID
			}
			if incarnation != row.SessionID {
				return
			}
			s.agentActivity.Store(&activitySample{AgentActivity: AgentActivity{Available: true, Working: row.Working}, at: time.Now(), title: row.Title})
		} else {
			s.agentActivity.Store(nil)
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// SessionTitle shares the exact-socket, incarnation and freshness fences of
// activity metadata, but a native delegation alone never invents a title.
func (s *Session) SessionTitle() string {
	if s.activityStopped.Load() {
		return ""
	}
	sample := s.agentActivity.Load()
	if sample == nil || time.Since(sample.at) > activityFreshness {
		return ""
	}
	return sample.title
}
