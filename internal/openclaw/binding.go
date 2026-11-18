package openclaw

import (
	"context"
	"errors"
)

// TalkBinding fences all operations to the exact authenticated connection which
// owns this activation. Reconnects and endpoint edits cannot transfer a call.
type TalkBinding struct {
	client *Client
	link   *rpcLink
}

func (c *Client) BindTalk() (*TalkBinding, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.saved.Talk || c.rpc == nil {
		return nil, errors.New("talk_control_unavailable")
	}
	return &TalkBinding{c, c.rpc}, nil
}
func (b *TalkBinding) current() bool {
	b.client.mu.Lock()
	defer b.client.mu.Unlock()
	return b.client.rpc == b.link && b.client.saved.Talk
}
func (b *TalkBinding) CallTalk(ctx context.Context, method string, params, result any) error {
	if !b.current() {
		return errors.New("talk_connection_changed")
	}
	if !talkMethodAllowed(method) {
		return errors.New("rpc_method_not_allowed")
	}
	err := b.link.call(ctx, method, params, result)
	if err == nil && method == "talk.client.create" {
		if launch, ok := result.(*TalkLaunch); ok {
			launch.owner = b.link
			launch.origin = b.link.origin
		}
	}
	return err
}
func (b *TalkBinding) SubscribeTalk() (<-chan TalkEvent, func(), error) {
	if !b.current() {
		return nil, nil, errors.New("talk_connection_changed")
	}
	return b.link.subscribe()
}
func (b *TalkBinding) ExchangeOffer(ctx context.Context, launch TalkLaunch, offer string) (string, error) {
	if !b.current() || launch.owner != b.link {
		return "", errors.New("talk_connection_changed")
	}
	return b.client.ExchangeOffer(ctx, launch, offer)
}
