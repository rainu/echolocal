// Package openclaw implements separately authenticated operator and Node connections.
package openclaw

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const Protocol = 4

type credentials struct {
	OperatorScopes *[]string `json:"operatorScopes,omitempty"`
	DisplayName    string    `json:"displayName,omitempty"`
	Talk           bool      `json:"talk,omitempty"`
	BootstrapOnly  bool      `json:"bootstrapOnly,omitempty"`
	URL            string    `json:"url"`
	Bootstrap      string    `json:"bootstrap,omitempty"`
	Private        []byte    `json:"private,omitempty"`
	Token          string    `json:"deviceToken,omitempty"`
	Enabled        bool      `json:"enabled"`
}

type Event struct {
	At                      time.Time
	Direction, Type, Result string
}
type Status struct {
	State, Reason               string
	Configured, Paired, Enabled bool
}

type Client struct {
	operatorSharedRetry bool
	shared              *Connections
	access              *NodeAccess
	node                *NodeMetadata
	rpc                 *rpcLink
	op                  sync.Mutex
	mu                  sync.Mutex
	path, version       string
	born                time.Time
	saved               credentials
	state, reason       string
	granted             []string
	events              []Event
	cancel              context.CancelFunc
	done                chan struct{}
	dialer              *websocket.Dialer
}

func New(path, version string) *Client {
	c := &Client{path: path, version: version, born: time.Now(), state: "disconnected", dialer: &websocket.Dialer{HandshakeTimeout: 10 * time.Second, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}}
	data, err := os.ReadFile(path)
	if err == nil {
		if json.Unmarshal(data, &c.saved) != nil {
			c.state = "error"
			c.reason = "storage"
			c.saved = credentials{}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		c.state = "error"
		c.reason = "storage"
	}
	return c
}

// Start resumes only a previously saved user preference. No secret is returned.
func (c *Client) Start() {
	c.op.Lock()
	defer c.op.Unlock()
	if c.Status().Enabled {
		c.startLocked()
	}
}
func (c *Client) Status() Status {
	s := c.snapshot()
	c.mu.Lock()
	defer c.mu.Unlock()
	return Status{c.state, c.reason, s.URL != "", s.Token != "", s.Enabled}
}
func (c *Client) Events() []Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Event(nil), c.events...)
}
func (c *Client) ClearEvents() { c.mu.Lock(); defer c.mu.Unlock(); c.events = nil }
func (c *Client) mark(state, reason, direction, kind string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state, c.reason = state, reason
	c.events = append(c.events, Event{time.Now(), direction, kind, state + ":" + reason})
	if len(c.events) > 64 {
		c.events = append([]Event(nil), c.events[len(c.events)-64:]...)
	}
}
func ValidateEndpoint(raw string) error {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "wss" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.TrimSpace(raw) != raw {
		return errors.New("invalid_endpoint")
	}
	if p := u.Port(); p != "" {
		if _, e := net.LookupPort("tcp", p); e != nil {
			return errors.New("invalid_endpoint")
		}
	}
	return nil
}
func (c *Client) persist(s credentials) error {
	if c.shared != nil {
		common := c.shared.Editable()
		// The shared Gateway credential has one persistent source; never duplicate
		// it into per-role identity files or confuse it with a device token.
		if common.Token != "" && !s.BootstrapOnly {
			s.Bootstrap = ""
		}
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0700); err != nil {
		return errors.New("storage")
	}
	b, e := json.Marshal(s)
	if e != nil {
		return errors.New("storage")
	}
	f, e := os.CreateTemp(filepath.Dir(c.path), ".openclaw-*")
	if e != nil {
		return errors.New("storage")
	}
	name := f.Name()
	defer os.Remove(name)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e == nil {
		e = closeErr
	}
	if e == nil {
		e = os.Rename(name, c.path)
	}
	if e != nil {
		return errors.New("storage")
	}
	c.mu.Lock()
	c.saved = s
	c.mu.Unlock()
	return nil
}
func (c *Client) stopLocked() {
	if c.cancel != nil {
		c.cancel()
		<-c.done
		c.cancel = nil
		c.mark("disconnected", "stopped", "local", "stop")
	}
}
func (c *Client) startLocked() {
	c.stopLocked()
	c.mu.Lock()
	c.operatorSharedRetry = false
	c.mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.done = make(chan struct{})
	go func() { defer close(c.done); c.run(ctx) }()
}

// Configure never returns endpoints or credentials in errors. An empty token
// preserves the existing bootstrap credential only for the same endpoint.
func (c *Client) Configure(endpoint, bootstrap string) error {
	if c.node != nil {
		return errors.New("credentials_missing")
	}
	return c.configure(endpoint, bootstrap, false, false)
}

// ConfigureBootstrap provisions a short-lived, explicitly issued pairing
// credential. It never substitutes a bootstrap bearer for a shared auth token.
func (c *Client) ConfigureBootstrap(endpoint, bootstrap string) error {
	return c.configure(endpoint, bootstrap, true, false)
}

// EditableConnection is exclusively for the private on-device editor.
// Never serialize its result into diagnostics, exports or public artifacts.
func (c *Client) EditableConnection() (string, string) {
	if c.shared != nil {
		s := c.shared.Editable()
		return s.URL, s.Token
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	token := c.saved.Bootstrap
	if token == "" {
		token = c.saved.Token
	}
	return c.saved.URL, token
}

// UpdateConnection saves the visible editor values. An edited credential must
// replace cached auth, otherwise the old device token would silently win.
func (c *Client) UpdateConnection(endpoint, token string) error {
	return c.configure(endpoint, token, c.node != nil, true)
}
func (c *Client) configure(endpoint, bootstrap string, bootstrapOnly, editing bool) error {
	if err := ValidateEndpoint(endpoint); err != nil {
		return err
	}
	c.op.Lock()
	defer c.op.Unlock()
	c.stopLocked()
	s := c.saved
	if endpoint != s.URL {
		s = credentials{URL: endpoint}
	}
	if editing {
		current := s.Bootstrap
		if current == "" {
			current = s.Token
		}
		if bootstrap == "" {
			s.Bootstrap, s.Token, s.BootstrapOnly = "", "", false
		} else if bootstrap != current {
			s.Bootstrap, s.Token, s.BootstrapOnly = bootstrap, "", bootstrapOnly
		}
	} else if bootstrap != "" {
		s.Bootstrap = bootstrap
		s.BootstrapOnly = bootstrapOnly
	}
	if len(s.Private) == 0 {
		_, p, e := ed25519.GenerateKey(rand.Reader)
		if e != nil {
			return errors.New("identity")
		}
		s.Private = p
	}
	s.Enabled = false
	if e := c.persist(s); e != nil {
		return e
	}
	c.mark("disconnected", "", "local", "settings_saved")
	return nil
}
func (c *Client) Connect() error {
	c.op.Lock()
	defer c.op.Unlock()
	c.stopLocked()
	s := c.snapshot()
	if ValidateEndpoint(s.URL) != nil {
		return errors.New("invalid_endpoint")
	}
	if len(s.Private) != ed25519.PrivateKeySize {
		return errors.New("identity")
	}
	if s.Token == "" && s.Bootstrap == "" {
		return errors.New("credentials_missing")
	}
	s.Enabled = true
	if e := c.persist(s); e != nil {
		return e
	}
	c.startLocked()
	return nil
}
func (c *Client) Disconnect() error {
	c.op.Lock()
	defer c.op.Unlock()
	c.stopLocked()
	s := c.saved
	s.Enabled = false
	e := c.persist(s)
	c.mark("disconnected", "manual", "local", "disconnect")
	return e
}

// Forget is local only; it does not revoke Gateway authorization.
func (c *Client) Forget() error {
	c.op.Lock()
	defer c.op.Unlock()
	c.stopLocked()
	if e := c.persist(credentials{}); e != nil {
		return e
	}
	c.ClearEvents()
	c.mark("disconnected", "forgotten", "local", "forget")
	return nil
}

// PairAgain keeps the stable device identity and removes only the cached token.
func (c *Client) PairAgain() error {
	c.op.Lock()
	defer c.op.Unlock()
	c.stopLocked()
	s := c.saved
	s.Token = ""
	s.Enabled = false
	if e := c.persist(s); e != nil {
		return e
	}
	c.mark("disconnected", "pair_again", "local", "pair_again")
	return nil
}

type frame struct {
	Type, ID, Event string
	OK              bool
	Payload         json.RawMessage
	Error           struct {
		Code      string
		Retryable bool
		Details   struct{ Code string }
	}
}

func connectParams(s credentials, nonce string, at int64, version string) map[string]any {
	displayName := s.DisplayName
	if displayName == "" {
		displayName = "EchoLocal"
	}
	private := ed25519.PrivateKey(s.Private)
	public := private.Public().(ed25519.PublicKey)
	hash := sha256.Sum256(public)
	id := hex.EncodeToString(hash[:])
	token := s.Token
	if token == "" {
		token = s.Bootstrap
	}
	auth := map[string]any{"token": token}
	if s.Token == "" && s.BootstrapOnly {
		auth = map[string]any{"bootstrapToken": token}
	}
	scopes := selectedOperatorScopes(s)
	payload := strings.Join([]string{"v3", id, "openclaw-linux", "ui", "operator", strings.Join(scopes, ","), fmt.Sprint(at), token, nonce, "android", "tablet"}, "|")
	return map[string]any{"minProtocol": Protocol, "maxProtocol": Protocol, "client": map[string]any{"id": "openclaw-linux", "displayName": displayName, "version": version, "platform": "android", "deviceFamily": "tablet", "mode": "ui"}, "role": "operator", "scopes": scopes, "auth": auth, "device": map[string]any{"id": id, "publicKey": base64.RawURLEncoding.EncodeToString(public), "signature": base64.RawURLEncoding.EncodeToString(ed25519.Sign(private, []byte(payload))), "signedAt": at, "nonce": nonce}}
}
func classify(err error) string {
	var dns *net.DNSError
	var cert *tls.CertificateVerificationError
	var unknown x509.UnknownAuthorityError
	switch {
	case errors.As(err, &dns):
		return "dns"
	case errors.As(err, &cert), errors.As(err, &unknown):
		return "tls"
	}
	var n net.Error
	if errors.As(err, &n) && n.Timeout() {
		return "timeout"
	}
	return "network"
}
func (c *Client) run(ctx context.Context) {
	delay := time.Second
	for ctx.Err() == nil {
		retry := c.attempt(ctx)
		if ctx.Err() != nil || !retry {
			return
		}
		c.mark("reconnecting", "network", "local", "backoff")
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if delay < 30*time.Second {
			delay *= 2
			if delay > 30*time.Second {
				delay = 30 * time.Second
			}
		}
	}
}
func (c *Client) attempt(ctx context.Context) bool {
	c.mu.Lock()
	c.granted = nil
	c.mu.Unlock()
	c.mark("connecting", "", "local", "connect")
	s := c.snapshot()
	c.mu.Lock()
	sharedRetry := c.operatorSharedRetry
	c.mu.Unlock()
	if sharedRetry {
		s.Token = ""
	}
	if ValidateEndpoint(s.URL) != nil || len(s.Private) != ed25519.PrivateKeySize || (s.Token == "" && s.Bootstrap == "") {
		c.mark("error", "settings", "local", "connect")
		return false
	}
	conn, resp, e := c.dialer.DialContext(ctx, s.URL, http.Header{})
	if e != nil {
		reason := classify(e)
		if resp != nil {
			if resp.Body != nil {
				resp.Body.Close()
			}
			if resp.StatusCode == 401 || resp.StatusCode == 403 {
				reason = "authentication"
			}
			if resp.StatusCode != 429 && resp.StatusCode < 500 {
				c.mark("error", reason, "in", "upgrade")
				return false
			}
		}
		c.mark("error", reason, "in", "transport")
		return reason != "tls"
	}
	defer conn.Close()
	conn.SetReadLimit(256 * 1024)
	stopped := make(chan struct{})
	defer close(stopped)
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-stopped:
		}
	}()
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	var ch frame
	if conn.ReadJSON(&ch) != nil || ch.Type != "event" || ch.Event != "connect.challenge" {
		c.mark("error", "protocol", "in", "challenge")
		return false
	}
	var challenge struct {
		Nonce string
		TS    int64
	}
	if json.Unmarshal(ch.Payload, &challenge) != nil || challenge.Nonce == "" {
		c.mark("error", "protocol", "in", "challenge")
		return false
	}
	id := make([]byte, 16)
	if _, e = rand.Read(id); e != nil {
		c.mark("error", "identity", "local", "connect")
		return false
	}
	rid := hex.EncodeToString(id)
	// No handshake fields or raw Gateway errors enter the diagnostic event buffer.
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if e = conn.WriteJSON(map[string]any{"type": "req", "id": rid, "method": "connect", "params": c.connectParams(s, challenge.Nonce, time.Now().UnixMilli())}); e != nil {
		c.mark("error", "network", "out", "connect")
		return true
	}
	c.mark("connecting", "", "out", "connect")
	var res frame
	// Gateway registration can broadcast events before the correlated hello.
	// Keep the original handshake deadline and bound pre-hello event traffic.
	for skipped := 0; ; skipped++ {
		if conn.ReadJSON(&res) != nil {
			c.mark("error", "protocol", "in", "connect")
			return false
		}
		if res.Type == "res" && res.ID == rid {
			break
		}
		if res.Type != "event" || skipped >= 15 {
			c.mark("error", "protocol", "in", "connect")
			return false
		}
	}
	if !res.OK {
		// The cached token may be valid but too narrow for a scope expansion.
		// Retry at most once using an already configured shared credential.
		// Do not erase the identity/token, request new credentials or approve pairing.
		if c.node == nil && !sharedRetry && s.Token != "" && s.Bootstrap != "" && !s.BootstrapOnly &&
			(res.Error.Details.Code == "AUTH_DEVICE_TOKEN_MISMATCH" || res.Error.Details.Code == "AUTH_TOKEN_MISMATCH") {
			c.mu.Lock()
			c.operatorSharedRetry = true
			c.mu.Unlock()
			c.mark("reconnecting", "authentication", "in", "scope_reauthenticate")
			return true
		}
		state, reason := "error", "authentication"
		switch res.Error.Details.Code {
		case "PAIRING_REQUIRED":
			state, reason = "pairing", "approval_required"
		case "AUTH_TOKEN_MISMATCH", "AUTH_TOKEN_MISSING", "AUTH_DEVICE_TOKEN_MISMATCH":
			reason = "authentication"
		case "PROTOCOL_MISMATCH":
			reason = "protocol"
		}
		if res.Error.Code == "UNAVAILABLE" && res.Error.Retryable && res.Error.Details.Code == "" {
			c.mark("error", "unavailable", "in", "connect")
			return true
		}
		c.mark(state, reason, "in", "connect")
		return false
	}
	var hello struct {
		Type     string
		Protocol int
		Auth     struct {
			DeviceToken, Role string
			Scopes            []string
		}
	}
	if json.Unmarshal(res.Payload, &hello) != nil || hello.Type != "hello-ok" || hello.Protocol != Protocol {
		c.mark("error", "protocol", "in", "hello")
		return false
	}
	if c.node == nil && hello.Auth.Role != "" && hello.Auth.Role != "operator" {
		c.mark("error", "authentication", "in", "hello")
		return false
	}
	if c.node != nil && (hello.Auth.Role != "node" || len(hello.Auth.Scopes) != 0) {
		c.mark("error", "authentication", "in", "hello")
		return false
	}
	if hello.Auth.DeviceToken != "" {
		s.Token = hello.Auth.DeviceToken
		if s.BootstrapOnly {
			s.Bootstrap = ""
			s.BootstrapOnly = false
		}
		if c.persist(s) != nil {
			c.mark("error", "storage", "local", "credentials")
			return false
		}
	}
	link := newRPCLink(conn)
	link.origin = s.URL
	c.mu.Lock()
	c.rpc = link
	if c.node == nil {
		c.granted = append([]string(nil), hello.Auth.Scopes...)
	}
	c.mu.Unlock()
	defer func() {
		link.close()
		if c.access != nil {
			c.access.Disconnect()
		}
		c.mu.Lock()
		if c.rpc == link {
			c.rpc = nil
			c.granted = nil
		}
		c.mu.Unlock()
	}()
	c.mark("connected", "", "in", "hello")
	conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	for {
		var msg frame
		if conn.ReadJSON(&msg) != nil {
			if ctx.Err() == nil {
				c.mark("disconnected", "network", "in", "close")
			}
			return true
		}
		conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		if msg.Type == "event" {
			link.event(msg)
			if c.access != nil {
				c.nodeEvent(ctx, link, msg)
			}
		}
		if msg.Type == "res" {
			link.deliver(msg)
			continue
		}
		if msg.Type == "event" && msg.Event == "shutdown" {
			c.mark("disconnected", "unavailable", "in", "shutdown")
			return true
		} // Never retain arbitrary event names or payloads.
		c.mu.Lock()
		c.events = append(c.events, Event{time.Now(), "in", "application_event", "received"})
		if len(c.events) > 64 {
			c.events = append([]Event(nil), c.events[len(c.events)-64:]...)
		}
		c.mu.Unlock()
	}
}
