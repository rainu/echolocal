package openclaw

import (
	"context"
	"errors"
	"strings"
	"unicode"
)

// SessionActivity is a metadata-only projection. No run IDs, transcripts,
// reasoning, tool arguments or results leave this adapter.
type SessionActivity struct {
	SessionID string
	Title     string
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
			Key                              string `json:"key"`
			Label, DisplayName, DerivedTitle string
			ID                               string `json:"sessionId"`
			Active                           *bool  `json:"hasActiveRun"`
			Children                         bool   `json:"hasActiveSubagentRun"`
			BackgroundProcesses              int    `json:"backgroundProcessCount"`
		} `json:"session"`
	}
	app := &AppBinding{client: b.client, link: b.link}
	if err := app.Call(ctx, "sessions.describe", map[string]any{"key": target, "includeDerivedTitles": true}, &result); err != nil {
		return SessionActivity{}, err
	}
	row := result.Session
	if !b.current() || row.Key != target || row.ID == "" || (row.Active == nil && !row.Children && row.BackgroundProcesses <= 0) {
		return SessionActivity{}, errors.New("talk_activity_unavailable")
	}
	return SessionActivity{SessionID: row.ID, Title: sessionTitle(target, row.Label, row.DisplayName, row.DerivedTitle), Working: (row.Active != nil && *row.Active) || row.Children || row.BackgroundProcesses > 0}, nil
}

// Titles are metadata of the exact routing target, never guessed from a key,
// voice slot or agent name. Flatten whitespace and bound untrusted UI input.
func sessionTitle(target string, candidates ...string) string {
	for _, title := range candidates {
		title = strings.Join(strings.FieldsFunc(title, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }), " ")
		if title == "" || title == target || strings.Contains(title, "agent:") {
			continue
		}
		r := []rune(title)
		if len(r) > 512 {
			title = string(r[:511]) + "…"
		}
		return title
	}
	return ""
}
