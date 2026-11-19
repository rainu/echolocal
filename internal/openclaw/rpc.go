package openclaw

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gorilla/websocket"
	"strconv"
	"sync"
	"time"
)

// rpcLink belongs to one authenticated socket. Lost or uncertain requests are
// never transferred to a replacement connection.
type rpcLink struct {
	conn      *websocket.Conn
	origin    string
	write     sync.Mutex
	mu        sync.Mutex
	next      uint64
	pending   map[string]chan frame
	closed    bool
	nodeCalls map[string]context.CancelFunc
	subs      map[*eventSubscription]bool
}

func newRPCLink(conn *websocket.Conn) *rpcLink {
	return &rpcLink{conn: conn, pending: make(map[string]chan frame), subs: make(map[*eventSubscription]bool)}
}
func (r *rpcLink) close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	r.closed = true
	for _, cancel := range r.nodeCalls {
		cancel()
	}
	for sub := range r.subs {
		close(sub.ch)
		delete(r.subs, sub)
	}
	for id, ch := range r.pending {
		close(ch)
		delete(r.pending, id)
	}
}
func (r *rpcLink) deliver(f frame) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ch, ok := r.pending[f.ID]; ok {
		delete(r.pending, f.ID)
		ch <- f
	}
}
func talkMethodAllowed(method string) bool {
	switch method {
	case "talk.catalog", "agents.list", "sessions.list", "talk.client.create", "talk.client.close", "talk.client.toolCall", "talk.client.steer", "talk.client.transcript":
		return true
	}
	return false
}

// CallTalk is backend-only: results can contain ephemeral provider credentials.
// Never put raw results or Gateway error payloads into presentation or logs.
func (c *Client) CallTalk(ctx context.Context, method string, params any, result any) error {
	if !talkMethodAllowed(method) {
		return errors.New("rpc_method_not_allowed")
	}
	c.mu.Lock()
	r := c.rpc
	enabled := c.saved.Talk
	c.mu.Unlock()
	if !enabled {
		return errors.New("talk_authorization_required")
	}
	if r == nil {
		return errors.New("rpc_disconnected")
	}
	err := r.call(ctx, method, params, result)
	if err == nil && method == "talk.client.create" {
		captureLaunch(r, params, result)
	}
	return err
}
func (r *rpcLink) call(ctx context.Context, method string, params any, result any) error {
	if ctx.Err() != nil {
		return errors.New("rpc_cancelled")
	}
	data, err := json.Marshal(params)
	if err != nil || len(data) > 128*1024 {
		return errors.New("rpc_invalid_params")
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return errors.New("rpc_disconnected")
	}
	if len(r.pending) >= 16 {
		r.mu.Unlock()
		return errors.New("rpc_busy")
	}
	r.next++
	id := "rpc-" + strconv.FormatUint(r.next, 10)
	ch := make(chan frame, 1)
	r.pending[id] = ch
	r.mu.Unlock()
	defer func() { r.mu.Lock(); delete(r.pending, id); r.mu.Unlock() }()
	r.write.Lock()
	deadline := time.Now().Add(10 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	r.conn.SetWriteDeadline(deadline)
	err = r.conn.WriteJSON(map[string]any{"type": "req", "id": id, "method": method, "params": json.RawMessage(data)})
	r.write.Unlock()
	if err != nil {
		r.conn.Close()
		return errors.New("rpc_delivery_uncertain")
	}
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return errors.New("rpc_cancelled")
	case <-timer.C:
		return errors.New("rpc_timeout")
	case f, ok := <-ch:
		if !ok {
			return errors.New("rpc_delivery_uncertain")
		}
		if !f.OK {
			return &RPCRejection{Retryable: f.Error.Code == "UNAVAILABLE" && f.Error.Retryable}
		}
		if result != nil && json.Unmarshal(f.Payload, result) != nil {
			return errors.New("rpc_invalid_response")
		}
		return nil
	}
}

// EnableTalk must only follow explicit user authorization. Connection-only
// clients continue to request no scopes until this preference is enabled.
func (c *Client) EnableTalk() error {
	if c.node != nil {
		return errors.New("talk_authorization_required")
	}
	c.op.Lock()
	defer c.op.Unlock()
	if c.saved.Talk {
		return nil
	}
	c.stopLocked()
	s := c.saved
	s.Talk = true
	if err := c.persist(s); err != nil {
		return err
	}
	if s.Enabled {
		c.startLocked()
	}
	return nil
}

// RPCRejection contains no Gateway text or identifiers. Retryable is only set
// for an explicit retryable UNAVAILABLE response, never an uncertain delivery.
type RPCRejection struct{ Retryable bool }

func (e *RPCRejection) Error() string { return "rpc_rejected" }
