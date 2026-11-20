//go:build !board_biscuit

package clawtalk

import "github.com/ygelfand/echolocal/internal/openclaw/talk"

// CaptionsFor exposes only the app's owned activation, never another call.
func (m *Manager) CaptionsFor(generation uint64) []talk.Caption {
	for _, call := range m.captions.Snapshot() {
		if call.Generation == generation {
			return call.Entries
		}
	}
	return nil
}
