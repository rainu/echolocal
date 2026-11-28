//go:build !board_biscuit

package clawtalk

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"github.com/ygelfand/echolocal/internal/feature/mictest"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	claw "github.com/ygelfand/echolocal/internal/feature/openclaw"
	connection "github.com/ygelfand/echolocal/internal/openclaw"
	"github.com/ygelfand/echolocal/internal/openclaw/talk"
)

// AutomationSession exposes routing identifiers and titles, never transcripts.
type AutomationSession struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func sessionAgent(key string) (string, bool) {
	parts := strings.SplitN(key, ":", 3)
	if !utf8.ValidString(key) || strings.IndexFunc(key, unicode.IsControl) >= 0 || len(parts) != 3 || parts[0] != "agent" || parts[1] == "" || parts[2] == "" ||
		strings.ContainsAny(parts[1], "/\\ \t\r\n") || len(key) > 1024 {
		return "", false
	}
	return parts[1], true
}

// AutomationSessions reuses the existing Talk operator connection. Pagination
// must finish successfully; a partial catalog is not an empty/successful result.
func AutomationSessions(ctx context.Context, c TalkCaller) ([]AutomationSession, error) {
	out := []AutomationSession{}
	seen := map[string]bool{}
	for offset := 0; offset < 5000; {
		var page struct {
			Sessions   []struct{ Key, Label, DisplayName, DerivedTitle string }
			HasMore    bool
			NextOffset int
		}
		params := map[string]any{"source": "sidebar", "includeDerivedTitles": true,
			"excludeCron": true, "excludeSystem": true, "excludeSubagents": true,
			"archived": false, "limit": 100, "offset": offset}
		if err := c.CallTalk(ctx, "sessions.list", params, &page); err != nil {
			return nil, errors.New("talk_connection_unavailable")
		}
		if page.Sessions == nil {
			return nil, errors.New("talk_session_catalog_invalid")
		}
		for _, row := range page.Sessions {
			if _, valid := sessionAgent(row.Key); !valid || seen[row.Key] {
				continue
			}
			seen[row.Key] = true
			name := row.Label
			if name == "" {
				name = row.DisplayName
			}
			if name == "" {
				name = row.DerivedTitle
			}
			out = append(out, AutomationSession{ID: row.Key,
				Name: SafeLabel(name, row.Key, len(out), "clawtalk.context_number")})
		}
		if !page.HasMore {
			return out, nil
		}
		if len(page.Sessions) == 0 || page.NextOffset < offset+len(page.Sessions) {
			return nil, errors.New("talk_session_catalog_invalid")
		}
		offset = page.NextOffset
	}
	return nil, errors.New("talk_session_catalog_incomplete")
}

func automationTarget(v Slot, invalid bool, sessionID, prompt string) (Slot, error) {
	if invalid {
		return Slot{}, errors.New("talk_storage")
	}
	if prompt != "" {
		v.GreetingPrompt = prompt
	}
	if err := talk.ValidateGreetingPrompt(v.GreetingPrompt); err != nil {
		return Slot{}, err
	}
	if sessionID != "" {
		agent, valid := sessionAgent(sessionID)
		if !valid {
			return Slot{}, errors.New("talk_invalid_session")
		}
		v.Context, v.Agent, v.Resume, v.OpenClaw = sessionID, agent, true, true
	} else {
		if !v.OpenClaw || v.Agent == "" || strings.ContainsAny(v.Agent, ":/\\ \t\r\n") {
			return Slot{}, errors.New("talk_target_missing")
		}
		if v.Resume && v.Context != "" {
			if agent, valid := sessionAgent(v.Context); !valid || agent != v.Agent {
				return Slot{}, errors.New("talk_invalid_session")
			}
		} else {
			raw := make([]byte, 16)
			if _, err := rand.Read(raw); err != nil {
				return Slot{}, errors.New("talk_identity")
			}
			// Per-call selection must never call Store.Target: it persists generated IDs.
			v.Context = "agent:" + v.Agent + ":echolocal-" + hex.EncodeToString(raw)
		}
	}
	return v, nil
}

type automationLaunch struct {
	bound  *connection.TalkBinding
	result chan error
	once   sync.Once
}

func (l *automationLaunch) notify(err error) {
	if l != nil {
		l.once.Do(func() { l.result <- err })
	}
}

// StartAutomation reserves the same ownership used by wake/UI/Node starts,
// validates before opening audio, and returns success only after transport and
// audio are ready. Defaults are copied from assistant 1, never persisted.
func (m *Manager) StartAutomation(ctx context.Context, sessionID, prompt string, haBusy bool) error {
	m.mu.Lock()
	if mictest.Active() || m.active || m.transfer != nil {
		m.mu.Unlock()
		return errors.New("talk_already_active")
	}
	if haBusy {
		m.mu.Unlock()
		return errors.New("talk_assistant_busy")
	}
	v, err := automationTarget(m.Store.Get(0), m.Store.invalid, sessionID, prompt)
	if err != nil {
		m.mu.Unlock()
		return err
	}
	m.active = true
	m.generation++
	generation := m.generation
	base := m.ctx
	if base == nil {
		base = context.Background()
	}
	callCtx, cancel := context.WithCancel(base)
	m.cancel = cancel
	m.mu.Unlock()
	// Request timeout cancels only setup; a successful Talk outlives the HA call.
	stop := context.AfterFunc(ctx, cancel)
	done := make(chan struct{})
	fail := func(err error) error {
		stop()
		cancel()
		m.mu.Lock()
		if m.generation == generation {
			m.active = false
			m.cancel = nil
		}
		m.mu.Unlock()
		return err
	}
	bound, err := claw.Get().BindTalk()
	if err != nil {
		return fail(errors.New("talk_connection_unavailable"))
	}
	if sessionID != "" {
		lookupCtx, lookupCancel := context.WithTimeout(callCtx, 15*time.Second)
		sessions, err := AutomationSessions(lookupCtx, bound)
		lookupCancel()
		if err != nil {
			return fail(err)
		}
		found := false
		for _, s := range sessions {
			if s.ID == v.Context {
				found = true
				break
			}
		}
		if !found {
			return fail(errors.New("talk_invalid_session"))
		}
	}
	if ctx.Err() != nil || callCtx.Err() != nil {
		return fail(errors.New("talk_start_cancelled"))
	}
	m.mu.Lock()
	m.captions.Begin(generation)
	m.lastBackend, m.lastAudio, m.avatarRenderer = nil, nil, nil
	m.recovery = nil
	m.error, m.forcedMuted = false, false
	m.mu.Unlock()
	launch := &automationLaunch{bound: bound, result: make(chan error, 1)}
	go func() { defer close(done); m.activate(callCtx, cancel, 0, generation, &v, launch) }()
	select {
	case err := <-launch.result:
		stop()
		if err != nil {
			cancel()
			<-done
		}
		return err
	case <-ctx.Done():
		cancel()
		<-done
		stop()
		return errors.New("talk_start_cancelled")
	}
}

// StopAutomation is idempotent and waits for release of this generation only.
// No chat.abort, sessions.delete/reset, or text-task cancellation is issued.
func (m *Manager) StopAutomation(ctx context.Context) error {
	m.mu.Lock()
	if m.transfer != nil {
		m.transfer.cancel()
	}
	if !m.active {
		m.mu.Unlock()
		return nil
	}
	generation := m.generation
	if m.cancel != nil {
		m.cancel()
	}
	m.mu.Unlock()
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		m.mu.Lock()
		released := !m.active || m.generation != generation
		m.mu.Unlock()
		if released {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.New("talk_stop_timeout")
		case <-tick.C:
		}
	}
}
