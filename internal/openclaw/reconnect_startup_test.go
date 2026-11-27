package openclaw

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestStartupRetriesInterruptedHandshake(t *testing.T) {
	for _, role := range []string{"operator", "node"} {
		for _, stage := range []string{"challenge", "hello"} {
			t.Run(role+"/"+stage, func(t *testing.T) {
				var attempts atomic.Int32
				srv, endpoint := server(t, func(conn *websocket.Conn) {
					n := attempts.Add(1)
					if n == 1 && stage == "challenge" {
						return
					}
					req := challenge(conn)
					if req == nil || n == 1 {
						return
					}
					if role == "node" {
						nodeHello(conn, req)
					} else {
						hello(conn, req)
					}
					var msg any
					conn.ReadJSON(&msg)
				})
				c := New(filepath.Join(t.TempDir(), "identity.json"), "test")
				if role == "node" {
					c = NewNode(c.path, "test", NodeMetadata{})
				}
				c.dialer.TLSClientConfig = srv.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
				if err := c.ConfigureBootstrap(endpoint, "synthetic-startup"); err != nil {
					t.Fatal(err)
				}
				if err := c.Connect(); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { c.Close() })
				waitState(t, c, "connected")
				if attempts.Load() != 2 {
					t.Fatalf("attempt count: %d", attempts.Load())
				}
				raw, _ := json.Marshal(c.Events())
				if strings.Contains(string(raw), "synthetic") || strings.Contains(string(raw), endpoint) {
					t.Fatal("private diagnostics")
				}
			})
		}
	}
}

// Keep the process clock untouched. Only the test TLS verifier sees a cold clock.
func TestStartupRetriesCertificateDateUntilClockReady(t *testing.T) {
	var corrected atomic.Bool
	srv, endpoint := server(t, func(conn *websocket.Conn) {
		req := challenge(conn)
		if req == nil {
			return
		}
		hello(conn, req)
		var msg any
		conn.ReadJSON(&msg)
	})
	c := client(t, srv, endpoint)
	c.dialer.TLSClientConfig.Time = func() time.Time {
		if corrected.Load() {
			return time.Now()
		}
		return time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	c.Connect()
	waitState(t, c, "reconnecting")
	if c.Status().Reason != "tls" {
		t.Fatal(c.Status())
	}
	corrected.Store(true)
	waitState(t, c, "connected")
}

func TestSavedRolesWaitForReadinessAndDoNotDuplicateStart(t *testing.T) {
	for _, role := range []string{"operator", "node"} {
		t.Run(role, func(t *testing.T) {
			var attempts atomic.Int32
			srv, endpoint := server(t, func(conn *websocket.Conn) {
				attempts.Add(1)
				req := challenge(conn)
				if req == nil {
					return
				}
				if role == "node" {
					nodeHello(conn, req)
				} else {
					hello(conn, req)
				}
				var msg any
				conn.ReadJSON(&msg)
			})
			path := filepath.Join(t.TempDir(), "role.json")
			makeClient := func() *Client {
				c := New(path, "test")
				if role == "node" {
					c = NewNode(path, "test", NodeMetadata{})
				}
				c.dialer.TLSClientConfig = srv.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
				return c
			}
			initial := makeClient()
			initial.ConfigureBootstrap(endpoint, "synthetic-bootstrap")
			initial.Connect()
			waitState(t, initial, "connected")
			identity := string(initial.snapshot().Private)
			initial.Close()
			resumed := makeClient()
			t.Cleanup(func() { resumed.Close() })
			var ready atomic.Bool
			resumed.SetReadiness(func() string {
				if !ready.Load() {
					return "network"
				}
				return ""
			})
			resumed.Start()
			waitState(t, resumed, "reconnecting")
			time.Sleep(120 * time.Millisecond)
			if attempts.Load() != 1 {
				t.Fatal("dialed before prerequisites")
			}
			ready.Store(true)
			waitState(t, resumed, "connected")
			for range 5 {
				resumed.Start()
			}
			time.Sleep(120 * time.Millisecond)
			if attempts.Load() != 2 {
				t.Fatal("duplicate lifecycle connection", attempts.Load())
			}
			if string(resumed.snapshot().Private) != identity || resumed.snapshot().Token != "synthetic-device-token" {
				t.Fatal("lost authorization")
			}
			resumed.Disconnect()
			disabled := makeClient()
			disabled.SetReadiness(func() string { return "" })
			disabled.Start()
			time.Sleep(120 * time.Millisecond)
			if disabled.Status().Enabled || attempts.Load() != 2 {
				t.Fatal("explicit disconnect ignored")
			}
		})
	}
}

func TestPendingReadinessCanBeCancelledWithoutReconnecting(t *testing.T) {
	c := New(filepath.Join(t.TempDir(), "role.json"), "test")
	c.Configure("wss://gateway.example.test", "synthetic")
	c.SetReadiness(func() string { return "clock" })
	c.Connect()
	waitState(t, c, "reconnecting")
	if c.Status().Reason != "clock" {
		t.Fatal(c.Status())
	}
	c.Disconnect()
	time.Sleep(1100 * time.Millisecond)
	if c.Status().Enabled || c.Status().State != "disconnected" {
		t.Fatal(c.Status())
	}
}

func TestMalformedHandshakeIsTerminalAndManualRetryStillWorks(t *testing.T) {
	var attempts atomic.Int32
	srv, endpoint := server(t, func(conn *websocket.Conn) {
		if attempts.Add(1) == 1 {
			conn.WriteMessage(websocket.TextMessage, []byte("{"))
			return
		}
		req := challenge(conn)
		if req == nil {
			return
		}
		hello(conn, req)
		var msg any
		conn.ReadJSON(&msg)
	})
	c := client(t, srv, endpoint)
	c.Connect()
	waitState(t, c, "error")
	if c.Status().Reason != "protocol" {
		t.Fatal(c.Status())
	}
	time.Sleep(1100 * time.Millisecond)
	if attempts.Load() != 1 {
		t.Fatal("malformed handshake retried")
	}
	c.Connect()
	waitState(t, c, "connected")
}

func TestHandshakeFailureClassification(t *testing.T) {
	for _, tt := range []struct {
		err    error
		retry  bool
		reason string
	}{
		{io.EOF, true, "network"},
		{io.ErrUnexpectedEOF, true, "network"},
		{os.ErrDeadlineExceeded, true, "timeout"},
		{&websocket.CloseError{Code: websocket.CloseServiceRestart}, true, "network"},
		{&websocket.CloseError{Code: websocket.ClosePolicyViolation}, false, "protocol"},
		{websocket.ErrReadLimit, false, "protocol"},
		{errors.New("synthetic-private-error"), false, "protocol"},
	} {
		c := New(filepath.Join(t.TempDir(), "role"), "test")
		if got := c.handshakeReadFailure(tt.err, "challenge"); got != tt.retry || c.Status().Reason != tt.reason {
			t.Fatal("classification", c.Status())
		}
	}
}
