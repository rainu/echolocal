// Package chatapp owns the transient chat view, not connection configuration.
package chatapp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type RPC interface {
	Call(context.Context, string, any, any) error
	Updates() (<-chan struct{}, func(), error)
}
type Session struct {
	Key           string `json:"key"`
	SessionID     string `json:"sessionId"`
	AgentID       string `json:"agentId"`
	Label         string `json:"label"`
	DerivedTitle  string `json:"derivedTitle"`
	DisplayName   string `json:"displayName"`
	Category      string `json:"category"`
	Model         string `json:"model"`
	ModelProvider string `json:"modelProvider"`
}

func (s Session) Title() string {
	for _, v := range []string{s.Label, s.DerivedTitle, s.DisplayName, s.Key} {
		if v != "" {
			return v
		}
	}
	return ""
}

type Message struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

func (m Message) Text() string {
	var s string
	if json.Unmarshal(m.Content, &s) == nil {
		return s
	}
	var blocks []struct{ Type, Text string }
	if json.Unmarshal(m.Content, &blocks) != nil {
		return ""
	}
	var out []string
	for _, b := range blocks {
		if b.Type == "text" {
			out = append(out, b.Text)
		}
	}
	return strings.Join(out, "\n\n")
}

type Model struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	Name     string `json:"name"`
}

func (m Model) Key() string {
	if strings.HasPrefix(m.ID, m.Provider+"/") || m.Provider == "" {
		return m.ID
	}
	return m.Provider + "/" + m.ID
}

type State struct {
	Sessions                                                      []Session
	Groups                                                        []string
	Selected                                                      Session
	Messages                                                      []Message
	Models                                                        []Model
	Draft, Notice, Older                                          string
	Connected, Loading, Busy, Talk, Stopping, ModelsReady, Active bool
	Generation                                                    uint64
	TalkGeneration                                                uint64
}
type pending struct{ Session, ID, Text string }
type App struct {
	mu            sync.Mutex
	state         State
	drafts        map[string]string
	pending       map[string]*pending
	subscribed    string
	olderMessages []Message
	bind          func() (RPC, error)
	redraw        func()
	startTalk     func(Session) (uint64, error)
	stopTalk      func(uint64) bool
	talkBusy      func() bool
	talkID        uint64
	rpc           RPC
	ctx           context.Context
	cancel        context.CancelFunc
	refresh       chan struct{}
	epoch         uint64
}

func New(bind func() (RPC, error), redraw func(), start func(Session) (uint64, error), stop func(uint64) bool, busy func() bool) *App {
	return &App{bind: bind, redraw: redraw, startTalk: start, stopTalk: stop, talkBusy: busy, drafts: map[string]string{}, refresh: make(chan struct{}, 1)}
}
func (a *App) Snapshot() State {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.state
	s.Sessions = append([]Session(nil), s.Sessions...)
	s.Groups = append([]string(nil), s.Groups...)
	s.Messages = append([]Message(nil), s.Messages...)
	s.Models = append([]Model(nil), s.Models...)
	return s
}
func (a *App) Open() {
	a.mu.Lock()
	if a.cancel != nil {
		a.mu.Unlock()
		return
	}
	a.epoch++
	epoch := a.epoch
	ctx, cancel := context.WithCancel(context.Background())
	a.ctx, a.cancel = ctx, cancel
	a.state.Loading = true
	a.state.Busy = false
	a.mu.Unlock()
	go a.loop(ctx, epoch)
}
func (a *App) Close() {
	a.mu.Lock()
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
	a.epoch++
	rpc, key := a.rpc, a.subscribed
	a.rpc = nil
	a.subscribed = ""
	a.state.Connected = false
	id := a.talkID
	a.state.Stopping = a.state.Talk
	a.mu.Unlock()
	if id != 0 {
		a.stopTalk(id)
	}
	if rpc != nil && key != "" {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = rpc.Call(ctx, "sessions.messages.unsubscribe", map[string]any{"key": key, "subscriptionId": "echolocal-chat"}, nil)
		}()
	}
}
func (a *App) Refresh() {
	select {
	case a.refresh <- struct{}{}:
	default:
	}
}
func (a *App) loop(ctx context.Context, epoch uint64) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	var updates <-chan struct{}
	var release func()
	defer func() {
		if release != nil {
			release()
		}
	}()
	dirty := true
	last := time.Time{}
	for {
		if dirty && time.Since(last) > 400*time.Millisecond {
			a.mu.Lock()
			rpc := a.rpc
			busy := a.state.Busy
			a.mu.Unlock()
			if !busy {
				if rpc == nil {
					if release != nil {
						release()
						release = nil
					}
					updates = nil
					var err error
					rpc, err = a.bind()
					if err == nil {
						updates, release, err = rpc.Updates()
					}
					if err == nil {
						err = rpc.Call(ctx, "sessions.subscribe", map[string]any{}, nil)
					}
					if err == nil {
						a.mu.Lock()
						if a.epoch == epoch {
							a.rpc = rpc
							a.subscribed = ""
						}
						a.mu.Unlock()
					} else {
						rpc = nil
					}
				}
				if rpc != nil {
					a.reload(ctx, epoch, rpc)
				} else {
					a.mu.Lock()
					if a.epoch == epoch {
						a.state.Notice = "disconnected"
						a.state.Connected = false
						a.state.Loading = false
					}
					a.mu.Unlock()
					a.redraw()
				}
				last = time.Now()
				dirty = false
			}
		}
		select {
		case <-ctx.Done():
			return
		case _, ok := <-updates:
			if !ok {
				updates = nil
				if release != nil {
					release()
					release = nil
				}
				a.mu.Lock()
				if a.epoch == epoch {
					a.rpc = nil
					a.state.Connected = false
				}
				a.mu.Unlock()
			}
			dirty = true
		case <-a.refresh:
			dirty = true
		case <-tick.C:
			a.mu.Lock()
			changed := a.state.Talk && !a.talkBusy()
			if changed {
				a.state.Talk = false
				a.state.Stopping = false
				a.talkID = 0
				a.state.Notice = "talk_ended"
			}
			a.mu.Unlock()
			if changed {
				a.redraw()
				dirty = true
			}
			if time.Since(last) > 10*time.Second {
				dirty = true
			}
		}
	}
}
func (a *App) reload(ctx context.Context, epoch uint64, rpc RPC) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var list struct {
		Sessions     []Session
		Count, Total int
	}
	params := map[string]any{"source": "sidebar", "rowMode": "sidebar", "includeDerivedTitles": true, "excludeCron": true, "excludeSystem": true, "excludeSubagents": true, "archived": false, "limit": 100, "offset": 0}
	var rows []Session
	var err error
	for offset := 0; offset < 5000; {
		params["offset"] = offset
		list.Sessions = nil
		if err = rpc.Call(ctx, "sessions.list", params, &list); err != nil {
			break
		}
		rows = append(rows, list.Sessions...)
		offset += len(list.Sessions)
		if len(list.Sessions) < 100 || list.Total > 0 && offset >= list.Total {
			break
		}
	}
	var groups struct {
		Groups []struct {
			Name     string
			Position int
		}
		SectionOrder []string
	}
	if err == nil {
		err = rpc.Call(ctx, "sessions.groups.list", map[string]any{}, &groups)
	}
	a.mu.Lock()
	if a.epoch != epoch {
		a.mu.Unlock()
		return
	}
	if err != nil {
		a.state.Connected = false
		a.state.Loading = false
		a.state.Notice = "unavailable"
		a.rpc = nil
		a.mu.Unlock()
		a.redraw()
		return
	}
	if a.state.Notice == "disconnected" || a.state.Notice == "unavailable" || a.state.Notice == "history_failed" {
		a.state.Notice = ""
	}
	a.state.Sessions = rows
	sort.SliceStable(groups.Groups, func(i, j int) bool { return groups.Groups[i].Position < groups.Groups[j].Position })
	a.state.Groups = nil
	for _, g := range groups.Groups {
		a.state.Groups = append(a.state.Groups, g.Name)
	}
	a.state.Connected = true
	a.state.Loading = false
	selected := a.state.Selected
	found := false
	for _, s := range rows {
		if s.Key == selected.Key && s.SessionID == selected.SessionID {
			selected = s
			found = true
			break
		}
	}
	if selected.Key != "" && !found {
		a.state.Notice = "session_changed"
		a.state.Connected = false
		a.mu.Unlock()
		a.redraw()
		return
	}
	a.state.Selected = selected
	gen := a.state.Generation
	oldKey := a.subscribed
	a.mu.Unlock()
	if oldKey != selected.Key {
		if oldKey != "" {
			_ = rpc.Call(ctx, "sessions.messages.unsubscribe", map[string]any{"key": oldKey, "subscriptionId": "echolocal-chat"}, nil)
		}
		if selected.Key != "" {
			if e := rpc.Call(ctx, "sessions.messages.subscribe", map[string]any{"key": selected.Key, "subscriptionId": "echolocal-chat"}, nil); e == nil {
				a.mu.Lock()
				if a.epoch == epoch {
					a.subscribed = selected.Key
				}
				a.mu.Unlock()
			}
		}
	}
	if selected.Key != "" {
		a.history(ctx, epoch, gen, rpc, selected, "")
	}
	a.redraw()
}
func (a *App) history(ctx context.Context, epoch, gen uint64, rpc RPC, s Session, cursor string) {
	params := map[string]any{"sessionKey": s.Key, "limit": 50, "maxBytes": 128 * 1024}
	if strings.HasPrefix(cursor, "offset:") {
		offset, err := strconv.Atoi(strings.TrimPrefix(cursor, "offset:"))
		if err != nil || offset < 0 {
			return
		}
		params["offset"] = offset
	} else if cursor != "" {
		params["cursor"] = cursor
	}
	var h struct {
		SessionID   string
		Messages    []Message
		OlderCursor string
		HasMore     bool
		NextOffset  int
		InFlightRun json.RawMessage
	}
	err := rpc.Call(ctx, "chat.history", params, &h)
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.epoch != epoch || a.state.Generation != gen {
		return
	}
	if err != nil {
		a.state.Notice = "history_failed"
		return
	}
	if h.SessionID != "" && h.SessionID != s.SessionID {
		a.state.Notice = "session_changed"
		a.state.Connected = false
		return
	}
	a.state.Active = len(h.InFlightRun) > 0 && string(h.InFlightRun) != "null"
	if h.OlderCursor == "" && h.HasMore && h.NextOffset > 0 {
		h.OlderCursor = "offset:" + strconv.Itoa(h.NextOffset)
	}
	// Keep an explicitly paged-back view stable while live tail updates arrive.
	if cursor != "" {
		a.olderMessages = append(h.Messages, a.olderMessages...)
		a.state.Messages = append(h.Messages, a.state.Messages...)
		a.state.Older = h.OlderCursor
	} else {
		a.state.Messages = append(append([]Message(nil), a.olderMessages...), h.Messages...)
		if len(a.olderMessages) == 0 {
			a.state.Older = h.OlderCursor
		}
	}
}
func (a *App) Select(key string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.state.Busy || a.state.Talk || a.talkBusy() {
		a.state.Notice = "stop_first"
		go a.redraw()
		return
	}
	for _, s := range a.state.Sessions {
		if s.Key == key {
			a.drafts[a.state.Selected.Key] = a.state.Draft
			a.state.Selected = s
			a.state.Draft = a.drafts[key]
			a.state.Messages = nil
			a.olderMessages = nil
			a.state.Older = ""
			a.state.Models = nil
			a.state.ModelsReady = false
			a.state.Generation++
			a.state.Notice = ""
			a.Refresh()
			go a.redraw()
			return
		}
	}
}
func (a *App) Draft(text string, generation ...uint64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.state.Busy && (len(generation) == 0 || generation[0] == a.state.Generation) {
		a.state.Draft = text
	}
}
func (a *App) operation(expected []uint64, fn func(context.Context, RPC, Session, uint64, uint64) error) {
	a.mu.Lock()
	if len(expected) > 0 && expected[0] != a.state.Generation {
		a.mu.Unlock()
		return
	}
	if a.ctx == nil || a.ctx.Err() != nil || !a.state.Connected || a.rpc == nil || a.state.Selected.Key == "" || a.state.Busy || a.state.Talk || a.talkBusy() {
		a.state.Notice = "unavailable"
		a.mu.Unlock()
		a.redraw()
		return
	}
	ctx, rpc, s, gen, epoch := a.ctx, a.rpc, a.state.Selected, a.state.Generation, a.epoch
	a.state.Busy = true
	a.mu.Unlock()
	a.redraw()
	go func() {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		err := fn(ctx, rpc, s, gen, epoch)
		a.mu.Lock()
		if a.epoch == epoch {
			a.state.Busy = false
			if err != nil {
				a.state.Notice = "action_failed"
			}
		}
		a.mu.Unlock()
		a.redraw()
		a.Refresh()
	}()
}
func (a *App) Send(generation ...uint64) {
	a.operation(generation, func(ctx context.Context, rpc RPC, s Session, gen, epoch uint64) error {
		a.mu.Lock()
		text := strings.TrimSpace(a.state.Draft)
		if text == "" || len(text) > 16000 {
			a.mu.Unlock()
			return errors.New("empty_or_large")
		}
		pendingKey := s.Key + "\x00" + s.SessionID
		if a.pending == nil {
			a.pending = map[string]*pending{}
		}
		if a.pending[pendingKey] == nil || a.pending[pendingKey].Text != text {
			raw := make([]byte, 16)
			if _, e := rand.Read(raw); e != nil {
				a.mu.Unlock()
				return e
			}
			a.pending[pendingKey] = &pending{s.Key, hex.EncodeToString(raw), text}
		}
		p := *a.pending[pendingKey]
		a.state.Notice = "sending"
		a.mu.Unlock()
		err := rpc.Call(ctx, "chat.send", map[string]any{"sessionKey": s.Key, "sessionId": s.SessionID, "message": text, "idempotencyKey": p.ID}, nil)
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.epoch != epoch || a.state.Generation != gen {
			return err
		}
		if err != nil {
			a.state.Notice = "delivery_uncertain"
			return nil
		}
		a.state.Draft = ""
		a.drafts[s.Key] = ""
		delete(a.pending, pendingKey)
		a.state.Notice = "sent"
		return nil
	})
}
func (a *App) Older() {
	a.operation(nil, func(ctx context.Context, rpc RPC, s Session, gen, epoch uint64) error {
		a.mu.Lock()
		cursor := a.state.Older
		a.mu.Unlock()
		if cursor != "" {
			a.history(ctx, epoch, gen, rpc, s, cursor)
		}
		return nil
	})
}
func (a *App) LoadModels() {
	a.operation(nil, func(ctx context.Context, rpc RPC, s Session, gen, epoch uint64) error {
		var out struct{ Models []Model }
		if e := rpc.Call(ctx, "models.list", map[string]any{"sessionKey": s.Key}, &out); e != nil {
			return e
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.epoch == epoch && a.state.Generation == gen {
			a.state.Models = out.Models
			a.state.ModelsReady = true
		}
		return nil
	})
}
func (a *App) SetModel(key string, generation ...uint64) {
	a.operation(generation, func(ctx context.Context, rpc RPC, s Session, gen, epoch uint64) error {
		a.mu.Lock()
		known := key == ""
		for _, m := range a.state.Models {
			if m.Key() == key {
				known = true
			}
		}
		a.mu.Unlock()
		if !known {
			return errors.New("unknown_model")
		}
		var model any = key
		if key == "" {
			model = nil
		}
		if e := rpc.Call(ctx, "sessions.patch", map[string]any{"key": s.Key, "expectedSessionId": s.SessionID, "model": model}, nil); e != nil {
			return e
		}
		a.mu.Lock()
		if a.epoch == epoch && a.state.Generation == gen {
			a.state.ModelsReady = false
			a.state.Models = nil
			a.state.Notice = "model_changed"
		}
		a.mu.Unlock()
		return nil
	})
}
func (a *App) Talk(generation ...uint64) {
	a.mu.Lock()
	if len(generation) > 0 && generation[0] != a.state.Generation {
		a.mu.Unlock()
		return
	}
	if a.state.Talk {
		id := a.talkID
		a.state.Stopping = true
		a.mu.Unlock()
		a.stopTalk(id)
		a.redraw()
		return
	}
	if !a.state.Connected || a.state.Busy || a.state.Selected.Key == "" || a.talkBusy() {
		a.state.Notice = "unavailable"
		a.mu.Unlock()
		a.redraw()
		return
	}
	id, err := a.startTalk(a.state.Selected)
	if err == nil {
		a.talkID = id
		a.state.TalkGeneration = id
		a.state.Talk = true
		a.state.Stopping = false
		a.state.Notice = "talk_active"
	} else {
		a.state.Notice = "talk_failed"
	}
	a.mu.Unlock()
	a.redraw()
}
