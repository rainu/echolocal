//go:build !board_biscuit

package clawtalk

import (
	shellfeature "github.com/ygelfand/echolocal/internal/feature/shell"
	"github.com/ygelfand/echolocal/internal/openclaw/talk"
	"github.com/ygelfand/libcountertop/pkg/display/menu"
	"github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/say"
	"time"
)

func (m *Manager) captionOwned(generation uint64, role, text, id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active && m.generation == generation {
		m.captions.Append(generation, role, text, id)
		m.uiDirty = true
	}
}
func microphoneLive(active bool, a *deviceAudio, now int64) bool {
	return active && a != nil && a.ctx.Err() == nil && a.lastCaptureMS.Load() > 0 && now-a.lastCaptureMS.Load() < 500
}
func captionRows(entries []talk.Caption) []menu.Row {
	// Newest-first keeps the current turn visible without stealing scroll focus.
	rows := []menu.Row{}
	if len(entries) == 0 {
		return []menu.Row{{Label: say.T("clawtalk.caption_empty"), Kind: menu.Plain}}
	}
	for i := len(entries) - 1; i >= 0; i-- {
		who := "clawtalk.caption_assistant"
		if entries[i].Role == "user" {
			who = "clawtalk.caption_user"
		}
		rows = append(rows, menu.Row{Label: say.T(who), Hint: entries[i].Text, Kind: menu.Plain})
	}
	return rows
}
func (m *Manager) conversationPage() *shell.Page {
	return &shell.Page{Title: "OpenClaw", Build: func() ([]menu.Row, []func(int)) {
		state := m.state()
		m.mu.Lock()
		active, a, generation := m.active, m.lastAudio, m.generation
		m.mu.Unlock()
		micKey := "clawtalk.mic_off"
		if active {
			micKey = "clawtalk.mic_wait"
		}
		if microphoneLive(active, a, time.Now().UnixMilli()) {
			micKey = "clawtalk.mic_live"
		}
		rows := []menu.Row{{Label: say.T("clawtalk.state." + string(state)), Hint: say.T("clawtalk.end_hint"), Kind: menu.Plain}, {Label: say.T(micKey), Hint: say.T("clawtalk.caption_hint"), Kind: menu.Plain}}
		calls := m.captions.Snapshot()
		var entries []talk.Caption
		for _, c := range calls {
			if c.Generation == generation {
				entries = c.Entries
				break
			}
		}
		rows = append(rows, captionRows(entries)...)
		return rows, make([]func(int), len(rows))
	}, Actions: []shell.Action{
		{Label: say.T("clawtalk.history"), MinHeight: 64, Do: func() { shellfeature.Get().Push(m.HistoryPage()) }},
		{Label: say.T("clawtalk.end"), MinHeight: 72, Role: shell.Danger, Do: func() { m.Stop() }},
	}}
}

// HistoryPage only exposes this device's bounded in-memory caption cache.
func (m *Manager) HistoryPage() *shell.Page {
	return &shell.Page{Title: say.T("clawtalk.history"), Actions: append(m.historyEndActions(), m.deleteHistoryAction(0)), Build: func() ([]menu.Row, []func(int)) {
		calls := m.captions.Snapshot()
		rows := []menu.Row{}
		for i := len(calls) - 1; i >= 0; i-- {
			generation, started := calls[i].Generation, calls[i].Started
			rows = append(rows, menu.Row{Label: started.Local().Format("15:04:05"), Kind: menu.Chevron, Open: func() menu.View {
				return &shell.Page{Title: started.Local().Format("15:04:05"), Actions: append(m.historyEndActions(), m.deleteHistoryAction(generation)), Build: func() ([]menu.Row, []func(int)) {
					var entries []talk.Caption
					for _, c := range m.captions.Snapshot() {
						if c.Generation == generation {
							entries = c.Entries
							break
						}
					}
					rows := captionRows(entries)
					return rows, make([]func(int), len(rows))
				}}
			}})
		}
		if len(rows) == 0 {
			rows = append(rows, menu.Row{Label: say.T("clawtalk.caption_empty"), Kind: menu.Plain})
		}
		return rows, make([]func(int), len(rows))
	}}
}

func (m *Manager) historyEndActions() []shell.Action {
	return []shell.Action{{Label: say.T("clawtalk.end"), MinHeight: 72, Role: shell.Danger, Do: func() { m.Stop() }}}
}

// Show opens the current/last display without starting audio or a new session.
func (m *Manager) Show() { shellfeature.Get().Push(m.conversationPage()) }

func (m *Manager) deleteHistoryAction(generation uint64) shell.Action {
	key := "clawtalk.delete_one"
	if generation == 0 {
		key = "clawtalk.delete_all"
	}
	return shell.Action{Label: say.T(key), MinHeight: 64, Role: shell.Danger, Do: func() {
		shellfeature.Get().Push(&shell.Page{Title: say.T(key), Build: func() ([]menu.Row, []func(int)) {
			return []menu.Row{{Label: say.T("clawtalk.history_local"), Hint: say.T("clawtalk.delete_hint"), Kind: menu.Plain}}, []func(int){nil}
		}, Actions: []shell.Action{
			{Label: say.T("openclaw.action.cancel"), MinHeight: 64, Do: func() { shellfeature.Get().Pop() }},
			{Label: say.T("openclaw.action.delete"), MinHeight: 64, Role: shell.Danger, Do: func() {
				m.mu.Lock()
				if generation == 0 {
					m.captions.Clear()
				} else {
					m.captions.Remove(generation)
				}
				m.uiDirty = true
				m.mu.Unlock()
				shellfeature.Get().Pop()
				shellfeature.Get().Redraw()
			}},
		}})
	}}
}
