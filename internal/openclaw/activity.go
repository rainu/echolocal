package openclaw

import (
	"context"
	"errors"
)

// SessionActivity is a metadata-only projection. No run IDs, transcripts,
// reasoning, tool arguments or results leave this adapter.
type SessionActivity struct {
	SessionID string
	Working   bool
}

// ReadSessionActivity uses the existing operator read grant on the exact Talk
// socket. It cannot create a session, widen permissions, or follow a reconnect.
func (b *TalkBinding) ReadSessionActivity(ctx context.Context, target string) (SessionActivity, error) {
	if !b.current() {
		return SessionActivity{}, errors.New("talk_connection_changed")
	}
	var result struct {
		Session struct {
			Key      string `json:"key"`
			ID       string `json:"sessionId"`
			Active   *bool  `json:"hasActiveRun"`
			Children bool   `json:"hasActiveSubagentRun"`
		} `json:"session"`
	}
	app := &AppBinding{client: b.client, link: b.link}
	if err := app.Call(ctx, "sessions.describe", map[string]any{"key": target}, &result); err != nil {
		return SessionActivity{}, err
	}
	row := result.Session
	if !b.current() || row.Key != target || row.ID == "" || row.Active == nil {
		return SessionActivity{}, errors.New("talk_activity_unavailable")
	}
	return SessionActivity{SessionID: row.ID, Working: *row.Active || row.Children}, nil
}
