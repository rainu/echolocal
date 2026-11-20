package openclaw

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// TalkEvent is backend-only. Payloads are transient and must never enter logs,
// presentation models, or persistent diagnostics.
type TalkEvent struct {
	VoiceSessionID string `json:"voiceSessionId"`
	Event          struct {
		Type    string          `json:"type"`
		CallID  string          `json:"callId"`
		Payload json.RawMessage `json:"payload"`
	} `json:"talkEvent"`
}
type eventSubscription struct {
	ch   chan TalkEvent
	once sync.Once
}

// SubscribeTalk binds delivery to the current socket; a lost/overflowed stream
// closes rather than reconnecting or silently dropping lifecycle events.
func (c *Client) SubscribeTalk() (<-chan TalkEvent, func(), error) {
	c.mu.Lock()
	r := c.rpc
	c.mu.Unlock()
	if r == nil {
		return nil, nil, errors.New("rpc_disconnected")
	}
	return r.subscribe()
}
func (r *rpcLink) subscribe() (<-chan TalkEvent, func(), error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || len(r.subs) >= 4 {
		return nil, nil, errors.New("rpc_unavailable")
	}
	s := &eventSubscription{ch: make(chan TalkEvent, 64)}
	r.subs[s] = true
	return s.ch, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.subs[s] {
			delete(r.subs, s)
			close(s.ch)
		}
	}, nil
}
func (r *rpcLink) event(f frame) {
	if f.Event != "talk.event" {
		return
	}
	var e TalkEvent
	if json.Unmarshal(f.Payload, &e) != nil || e.VoiceSessionID == "" {
		return
	}
	// Retain only transcript payloads; never audio or tool arguments/results.
	switch e.Event.Type {
	case "session.ready", "session.closed", "session.error", "session.replaced",
		"turn.started", "turn.ended", "turn.cancelled", "transcript.done", "transcript.delta", "output.text.delta", "output.text.done",
		"output.audio.started", "output.audio.done", "tool.call", "tool.result", "tool.error":
	default:
		return
	}
	if e.Event.Type != "transcript.done" && e.Event.Type != "transcript.delta" && e.Event.Type != "output.text.delta" && e.Event.Type != "output.text.done" {
		e.Event.Payload = nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for s := range r.subs {
		select {
		case s.ch <- e:
		default:
			delete(r.subs, s)
			close(s.ch)
		}
	}
}

type TalkLaunch struct {
	Provider       string `json:"provider"`
	Model          string `json:"model"`
	clientOwned    bool
	origin         string
	owner          *rpcLink
	Transport      string            `json:"transport"`
	VoiceSessionID string            `json:"voiceSessionId"`
	ClientSecret   string            `json:"clientSecret"`
	OfferURL       string            `json:"offerUrl"`
	OfferHeaders   map[string]string `json:"offerHeaders"`
	ClientControl  struct {
		Owner string `json:"owner"`
	} `json:"clientControl"`
}

// ExchangeOffer confines ephemeral offer authorization to the paired origin.
// No redirect, proxy, URL-bearing error, or response body is exposed.
func (c *Client) ExchangeOffer(ctx context.Context, launch TalkLaunch, offer string) (string, error) {
	c.mu.Lock()
	current := c.rpc
	c.mu.Unlock()
	if launch.owner == nil || current != launch.owner {
		return "", errors.New("talk_connection_changed")
	}
	endpoint := launch.origin
	base, err := url.Parse(endpoint)
	if err != nil || base.Scheme != "wss" || launch.Transport != "webrtc" || !(launch.ClientOwned() || (launch.ClientControl.Owner == "gateway" && !launch.clientOwned)) || launch.ClientSecret == "" || len(offer) > 1024*1024 {
		return "", errors.New("talk_launch_invalid")
	}
	rel, err := url.Parse(launch.OfferURL)
	if err != nil || rel.IsAbs() || rel.Host != "" || !strings.HasPrefix(rel.Path, "/plugins/") || rel.RawQuery != "" || rel.Fragment != "" {
		return "", errors.New("talk_offer_origin")
	}
	base.Scheme = "https"
	target := base.ResolveReference(rel)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), strings.NewReader(offer))
	if err != nil {
		return "", errors.New("talk_offer_invalid")
	}
	req.Header.Set("Content-Type", "application/sdp")
	req.Header.Set("Authorization", "Bearer "+launch.ClientSecret)
	for k, v := range launch.OfferHeaders {
		if strings.EqualFold(k, "OpenAI-Beta") {
			req.Header.Set(k, v)
		}
	}
	tr := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := hc.Do(req)
	if err != nil {
		return "", errors.New("talk_offer_failed")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", errors.New("talk_offer_rejected")
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 1024*1024+1))
	if err != nil || len(b) > 1024*1024 {
		return "", errors.New("talk_answer_invalid")
	}
	return string(b), nil
}

// ClientOwned is true only when this exact request explicitly selected the
// transcript-capable client control contract and the Gateway agreed.
func (l TalkLaunch) ClientOwned() bool { return l.clientOwned && l.ClientControl.Owner == "" }
func captureLaunch(r *rpcLink, params, result any) {
	launch, ok := result.(*TalkLaunch)
	if !ok {
		return
	}
	launch.owner = r
	launch.origin = r.origin
	raw, err := json.Marshal(params)
	if err != nil {
		return
	}
	var p struct{ Capabilities []string }
	if json.Unmarshal(raw, &p) != nil {
		return
	}
	gateway, transcripts := false, false
	for _, cap := range p.Capabilities {
		if cap == "gateway-control-v1" {
			gateway = true
		}
		if cap == "voice-transcript" {
			transcripts = true
		}
	}
	launch.clientOwned = transcripts && !gateway
}
