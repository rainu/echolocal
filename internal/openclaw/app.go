package openclaw

import (
	"context"
	"errors"
)

// AppBinding is an operator-only, connection-fenced chat adapter. It never
// borrows Node authority and never retries a mutation on a replacement socket.
type AppBinding struct {
	client *Client
	link   *rpcLink
}

func (c *Client) BindApp() (*AppBinding, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.node != nil || c.rpc == nil {
		return nil, errors.New("app_disconnected")
	}
	return &AppBinding{c, c.rpc}, nil
}

func appScope(method string) string {
	switch method {
	case "sessions.list", "sessions.subscribe", "sessions.groups.list", "sessions.messages.subscribe", "sessions.messages.unsubscribe", "chat.history", "models.list":
		return "read"
	case "chat.send", "sessions.patch":
		return "write"
	}
	return ""
}

func (b *AppBinding) Call(ctx context.Context, method string, params, result any) error {
	scope := appScope(method)
	if scope == "" {
		return errors.New("rpc_method_not_allowed")
	}
	b.client.mu.Lock()
	current := b.client.rpc == b.link && b.client.node == nil
	allowed := false
	for _, s := range b.client.granted {
		if s == "operator.admin" || s == "operator."+scope || s == "operator.sessions."+scope {
			allowed = true
		}
	}
	b.client.mu.Unlock()
	if !current {
		return errors.New("app_connection_changed")
	}
	if !allowed {
		return errors.New("app_scope_required")
	}
	return b.link.call(ctx, method, params, result)
}

// Updates are content-free invalidations. A full history read remains the
// authoritative source, so reconnects/deltas cannot duplicate rendered messages.
func (b *AppBinding) Updates() (<-chan struct{}, func(), error) {
	b.link.mu.Lock()
	defer b.link.mu.Unlock()
	if b.link.closed {
		return nil, nil, errors.New("app_disconnected")
	}
	if b.link.appSubs == nil {
		b.link.appSubs = make(map[chan struct{}]bool)
	}
	if len(b.link.appSubs) >= 4 {
		return nil, nil, errors.New("app_busy")
	}
	ch := make(chan struct{}, 1)
	b.link.appSubs[ch] = true
	return ch, func() {
		b.link.mu.Lock()
		defer b.link.mu.Unlock()
		if b.link.appSubs[ch] {
			delete(b.link.appSubs, ch)
			close(ch)
		}
	}, nil
}

func (r *rpcLink) appEvent(f frame) {
	switch f.Event {
	case "sessions.changed", "session.message", "session.messages", "chat", "chat.status", "agent", "session.status":
	default:
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for ch := range r.appSubs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
