//go:build !board_biscuit

package clawtalk

import "github.com/ygelfand/echolocal/internal/openclaw/talk"

func (m *Manager) agentActivity() talk.AgentActivity {
	m.mu.Lock()
	active, b := m.active, m.lastBackend
	m.mu.Unlock()
	state := m.Controller.State()
	if !active || b == nil || state == talk.Connecting || state == talk.Ended || state == talk.Failed {
		return talk.AgentActivity{}
	}
	return b.AgentActivity()
}
