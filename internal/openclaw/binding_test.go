package openclaw

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestTalkBindingFencesReconnectAndSecrets(t *testing.T) {
	c := New(t.TempDir()+"/settings.json", "test")
	old := newRPCLink(nil)
	old.origin = "wss://synthetic-old.example"
	c.saved.Talk = true
	c.rpc = old
	binding, err := c.BindTalk()
	if err != nil {
		t.Fatal(err)
	}
	events, stop, err := binding.SubscribeTalk()
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	var f frame
	f.Event = "talk.event"
	f.Payload = json.RawMessage(`{"voiceSessionId":"synthetic","talkEvent":{"type":"tool.call","payload":{"args":"synthetic-secret"}}}`)
	old.event(f)
	e := <-events
	if len(e.Event.Payload) != 0 {
		t.Fatal("tool payload forwarded")
	}
	c.rpc = newRPCLink(nil)
	if err = binding.CallTalk(context.Background(), "talk.catalog", map[string]any{}, nil); err == nil || err.Error() != "talk_connection_changed" {
		t.Fatal("RPC transferred to another connection")
	}
	launch := TalkLaunch{origin: old.origin, owner: old}
	launch.Transport = "webrtc"
	launch.ClientControl.Owner = "gateway"
	launch.ClientSecret = "synthetic-secret"
	launch.OfferURL = "/plugins/test"
	if _, err = binding.ExchangeOffer(context.Background(), launch, "synthetic-sdp"); err == nil || err.Error() != "talk_connection_changed" {
		t.Fatal("offer authorization crossed connections")
	}
	old.close()
	select {
	case _, ok := <-events:
		if ok {
			t.Fatal("old event stream still active")
		}
	case <-time.After(time.Second):
		t.Fatal("event stream not closed")
	}
}
func TestTalkEventOverflowClosesInsteadOfDroppingLifecycle(t *testing.T) {
	r := newRPCLink(nil)
	ch, _, err := r.subscribe()
	if err != nil {
		t.Fatal(err)
	}
	var f frame
	f.Event = "talk.event"
	f.Payload = json.RawMessage(`{"voiceSessionId":"synthetic","talkEvent":{"type":"session.ready"}}`)
	for i := 0; i < 65; i++ {
		r.event(f)
	}
	for range ch {
	}
	if len(r.subs) != 0 {
		t.Fatal("overflowed event subscriber retained")
	}
}
