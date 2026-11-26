package openclaw

import (
	"bytes"
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

func TestClientMediaContractRequiresExplicitOptInOnExactConnection(t *testing.T) {
	r := newRPCLink(nil)
	r.origin = "wss://synthetic.example"
	var launch TalkLaunch
	captureLaunch(r, map[string]any{"capabilities": []string{"voice-transcript", "voice-selection"}}, &launch)
	if !launch.ClientOwned() || launch.owner != r || launch.origin != r.origin {
		t.Fatal("client media was not fenced to requested connection")
	}
	captureLaunch(r, map[string]any{"capabilities": []string{"voice-transcript", "gateway-control-v1"}}, &launch)
	if launch.ClientOwned() {
		t.Fatal("gateway-owned contract accepted as client media")
	}
	captureLaunch(r, map[string]any{}, &launch)
	if launch.ClientOwned() {
		t.Fatal("client media enabled without explicit opt-in")
	}
	captureLaunch(r, map[string]any{"capabilities": []string{"voice-transcript"}}, &launch)
	launch.ClientControl.Owner = "gateway"
	if launch.ClientOwned() {
		t.Fatal("Gateway ownership silently replaced")
	}
}

func TestTalkInputUnconfirmedCrossesRPCPrivacyBoundaryWithoutPayload(t *testing.T) {
	r := newRPCLink(nil)
	ch, stop, err := r.subscribe()
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	const message = "OpenClaw could not confirm that input. It was not sent again; check the conversation before retrying."
	for _, tc := range []struct{ name, kind, message, claimed, want string }{
		{"known", "session.error", message, "", "provider_input_unconfirmed"},
		{"unknown", "session.error", "synthetic-private-error", "", ""},
		{"forged", "session.error", "Unauthorized", "provider_input_unconfirmed", ""},
		{"replaced", "session.replaced", message, "", ""},
		{"tool", "tool.result", message, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]any{"voiceSessionId": "synthetic", "FailureReason": tc.claimed, "failureReason": tc.claimed, "talkEvent": map[string]any{"type": tc.kind, "payload": map[string]any{"message": tc.message, "arguments": "synthetic-private-arguments", "token": "synthetic-private-token"}}})
			r.event(frame{Event: "talk.event", Payload: body})
			e := <-ch
			if e.FailureReason != tc.want {
				t.Fatalf("safe reason=%q want=%q", e.FailureReason, tc.want)
			}
			if len(e.Event.Payload) != 0 {
				t.Fatal("raw provider/tool payload escaped privacy boundary")
			}
			serialized, _ := json.Marshal(e)
			if bytes.Contains(serialized, []byte("synthetic-private")) {
				t.Fatal("private error details entered serialized event")
			}
		})
	}
}
