package openclaw

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestDesktopAttachOriginAndTicket(t *testing.T) {
	good, err := desktopAttachURL("wss://gateway.invalid/mount/", "0123456789abcdef", "/node-desktop/attach?ticket=0123456789abcdef")
	if err != nil || good != "wss://gateway.invalid/mount/node-desktop/attach?ticket=0123456789abcdef" {
		t.Fatal("mount path not retained")
	}
	for _, path := range []string{"wss://other.invalid/node-desktop/attach?ticket=0123456789abcdef", "//other.invalid/node-desktop/attach?ticket=0123456789abcdef", "/node-desktop/attach?ticket=wrong", "/node-desktop/attach?ticket=0123456789abcdef&ticket=0123456789abcdef", "/node-desktop/attach?ticket=0123456789abcdef&x=1", "/node-desktop/../attach?ticket=0123456789abcdef", "/node-desktop/%61ttach?ticket=0123456789abcdef", "/node-desktop/attach?ticket=0123456789abcdef#x"} {
		if _, err := desktopAttachURL("wss://gateway.invalid", "0123456789abcdef", path); err == nil {
			t.Fatal("unsafe attach accepted")
		}
	}
}
func TestDesktopDefaultOptIn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "grants.json")
	a := NewNodeAccess(path)
	a.Register(NodeOperation{Command: "desktop.stream", Group: "desktop", Run: func(context.Context, json.RawMessage) (any, error) { t.Fatal("denied capture ran"); return nil, nil }})
	if a.Granted("desktop") {
		t.Fatal("sharing default on")
	}
	if _, err := a.Execute(context.Background(), "desktop.stream", nil); err == nil {
		t.Fatal("denied start")
	}
	if !a.Granted("screen") {
		t.Fatal("snapshot grant changed")
	}
	a.SetGrants(map[string]bool{"screen": true})
	if NewNodeAccess(path).Granted("desktop") {
		t.Fatal("old screen grant enabled stream")
	}
}
func TestDesktopInvocationLifetime(t *testing.T) {
	parent, stop := context.WithCancel(context.Background())
	stream, cancel := nodeInvocationContext(parent, "desktop.stream", 0, time.Nanosecond)
	defer cancel()
	if _, ok := stream.Deadline(); ok {
		t.Fatal("zero-timeout stream was capped")
	}
	bounded, done := nodeInvocationContext(parent, "screen.snapshot", 0, time.Second)
	defer done()
	if _, ok := bounded.Deadline(); !ok {
		t.Fatal("snapshot became unbounded")
	}
	timed, finish := nodeInvocationContext(parent, "desktop.stream", 1000, time.Second)
	defer finish()
	if _, ok := timed.Deadline(); !ok {
		t.Fatal("explicit timeout lost")
	}
	stop()
	if stream.Err() == nil {
		t.Fatal("disconnect did not cancel")
	}
}
func clientRFBHandshake(t *testing.T, p desktopWire, password string, w, h int) {
	t.Helper()
	b, err := rfbRead(p, 12)
	if err != nil || string(b) != "RFB 003.008\n" {
		t.Fatal("banner", err)
	}
	p.Write(b)
	b, err = rfbRead(p, 2)
	if err != nil || !bytes.Equal(b, []byte{1, 2}) {
		t.Fatal("authentication offer", err)
	}
	p.Write([]byte{2})
	b, err = rfbRead(p, 16)
	if err != nil {
		t.Fatal(err)
	}
	p.Write(vncResponse(password, b))
	b, err = rfbRead(p, 4)
	if err != nil || !bytes.Equal(b, make([]byte, 4)) {
		t.Fatal("authentication failed", err)
	}
	p.Write([]byte{1})
	b, err = rfbRead(p, 24)
	if err != nil {
		t.Fatal("server init", err)
	}
	if int(binary.BigEndian.Uint16(b)) != w || int(binary.BigEndian.Uint16(b[2:])) != h {
		t.Fatal("size")
	}
	if n := binary.BigEndian.Uint32(b[20:]); n > 4096 {
		t.Fatal("name limit")
	} else {
		if _, err = rfbRead(p, int(n)); err != nil {
			t.Fatal(err)
		}
	}
}
func updateRequest() []byte { return []byte{3, 0, 0, 0, 0, 0, 0, 4, 0, 2} }
func readUpdate(t *testing.T, p desktopWire, tight bool) {
	t.Helper()
	b, err := rfbRead(p, 16)
	if err != nil {
		t.Fatal("update", err)
	}
	if b[3] != 1 || binary.BigEndian.Uint16(b[8:]) != 4 || binary.BigEndian.Uint16(b[10:]) != 2 {
		t.Fatal("rectangle")
	}
	encoding := binary.BigEndian.Uint32(b[12:])
	if tight {
		if encoding != 7 {
			t.Fatal("not Tight")
		}
		b, err = rfbRead(p, 1)
		if err != nil || b[0] != 0x90 {
			t.Fatal("JPEG control")
		}
		n := 0
		for i := 0; i < 3; i++ {
			b, err = rfbRead(p, 1)
			if err != nil {
				t.Fatal(err)
			}
			mask := byte(127)
			if i == 2 {
				mask = 255
			}
			n |= int(b[0]&mask) << uint(i*7)
			if b[0]&128 == 0 || i == 2 {
				break
			}
		}
		b, err = rfbRead(p, n)
		if err != nil {
			t.Fatal(err)
		}
		img, err := jpeg.Decode(bytes.NewReader(b))
		if err != nil || img.Bounds().Dx() != 4 {
			t.Fatal("JPEG", err)
		}
	} else {
		if encoding != 0 {
			t.Fatal("not Raw")
		}
		b, err = rfbRead(p, 32)
		if err != nil || !bytes.Equal(b[:4], []byte{30, 20, 10, 0}) {
			t.Fatal("raw pixel", err)
		}
	}
}
func TestRFBViewOnlyRawAndTight(t *testing.T) {
	for _, tight := range []bool{false, true} {
		t.Run(map[bool]string{false: "raw", true: "tight"}[tight], func(t *testing.T) {
			server, client := net.Pipe()
			defer client.Close()
			client.SetDeadline(time.Now().Add(5 * time.Second))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				defer server.Close()
				done <- serveDesktop(ctx, server, "12345678", 4, 2, func(context.Context) (image.Image, error) {
					img := image.NewRGBA(image.Rect(0, 0, 4, 2))
					for y := 0; y < 2; y++ {
						for x := 0; x < 4; x++ {
							img.SetRGBA(x, y, color.RGBA{10, 20, 30, 255})
						}
					}
					return img, nil
				}, func() {})
			}()
			clientRFBHandshake(t, client, "12345678", 4, 2)
			if tight {
				client.Write([]byte{2, 0, 0, 1, 0, 0, 0, 7})
			}
			// Real key, pointer and clipboard messages are consumed without side effects.
			client.Write([]byte{4, 1, 0, 0, 0, 0, 0, 65, 5, 1, 0, 1, 0, 1, 6, 0, 0, 0, 0, 0, 0, 3, 'b', 'a', 'd'})
			client.Write(updateRequest())
			readUpdate(t, client, tight)
			// Malformed clipboard length must terminate rather than allocate.
			client.Write([]byte{6, 0, 0, 0, 127, 255, 255, 255})
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("bad clipboard accepted")
				}
			case <-time.After(time.Second):
				t.Fatal("RFB did not stop")
			}
		})
	}
}
func TestRFBPixelFormats(t *testing.T) {
	if !validRFBFormat(rfbFormat) {
		t.Fatal("default format")
	}
	for _, change := range []func([]byte){func(f []byte) { f[0] = 64 }, func(f []byte) { f[10] = 32 }, func(f []byte) { f[10] = 8 }, func(f []byte) { f[4] = 1; f[5] = 4 }, func(f []byte) { f[3] = 0 }} {
		f := append([]byte(nil), rfbFormat...)
		change(f)
		if validRFBFormat(f) {
			t.Fatal("bad pixel layout")
		}
	}
}
func TestDesktopCarrierCleanupAndFraming(t *testing.T) {
	for _, cause := range []string{"local-stop", "revocation", "disconnect", "cancel", "peer-close", "timeout"} {
		t.Run(cause, func(t *testing.T) {
			a := NewNodeAccess(filepath.Join(t.TempDir(), "grants.json"))
			a.SetGrants(map[string]bool{"desktop": true})
			peerReady := make(chan *desktopPeer, 1)
			bad := make(chan string, 1)
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
				if err != nil {
					return
				}
				conn.SetReadLimit(desktopFrameLimit)
				typ, data, err := conn.ReadMessage()
				var meta map[string]string
				if err != nil || typ != websocket.BinaryMessage || json.Unmarshal(data, &meta) != nil || meta["auth"] != "vnc-password" {
					bad <- "metadata"
					return
				}
				p := &desktopPeer{conn: conn, timeout: 5 * time.Second}
				// Fragment each RFB client write into at most three-byte binary messages.
				peerReady <- p
				clientRFBHandshake(t, &fragmentedPeer{p}, meta["vncPassword"], 4, 2)
				p.Write(updateRequest())
				readUpdate(t, p, false)
				if cause == "peer-close" {
					conn.Close()
				}
			}))
			defer srv.Close()
			c := NewNode(filepath.Join(t.TempDir(), "node.json"), "test", NodeMetadata{})
			c.AttachNodeAccess(a)
			pool := x509.NewCertPool()
			pool.AddCert(srv.Certificate())
			c.dialer.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
			c.rpc = &rpcLink{origin: strings.Replace(srv.URL, "https:", "wss:", 1)}
			a.Register(NodeOperation{Command: "desktop.stream", Group: "desktop", Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
				return c.StreamDesktop(ctx, raw, 4, 2, func(context.Context) (image.Image, error) { return image.NewUniform(color.RGBA{10, 20, 30, 255}), nil })
			}})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if cause == "timeout" {
				ctx, cancel = context.WithTimeout(ctx, 800*time.Millisecond)
				defer cancel()
			}
			result := make(chan error, 1)
			go func() {
				_, err := a.Execute(ctx, "desktop.stream", json.RawMessage(`{"ticket":"0123456789abcdef","attachPath":"/node-desktop/attach?ticket=0123456789abcdef"}`))
				result <- err
			}()
			var peer *desktopPeer
			select {
			case peer = <-peerReady:
				defer peer.conn.Close()
			case <-time.After(3 * time.Second):
				t.Fatal("no attach")
			}
			// Wait for actual framebuffer delivery, never a fixed startup sleep.
			deadline := time.Now().Add(3 * time.Second)
			for c.DesktopStatus().Frames == 0 && time.Now().Before(deadline) {
				time.Sleep(5 * time.Millisecond)
			}
			if c.DesktopStatus().Frames == 0 {
				t.Fatal("no frames")
			}
			// Outbound frame chunking independently checked for payloads above 64 KiB.
			// The real Raw framebuffer tests exercise bytes spanning arbitrary frames.
			switch cause {
			case "local-stop":
				c.StopDesktop()
			case "revocation":
				a.SetGrants(map[string]bool{"desktop": false})
			case "disconnect":
				a.Disconnect()
			case "cancel":
				cancel()
			}
			select {
			case <-result:
			case <-time.After(3 * time.Second):
				t.Fatal("carrier leaked")
			}
			if c.DesktopStatus().State != "stopped" {
				t.Fatal("stale state")
			}
			select {
			case msg := <-bad:
				t.Fatal(msg)
			default:
			}
		})
	}
}

type fragmentedPeer struct{ *desktopPeer }

func (p *fragmentedPeer) Write(b []byte) (int, error) {
	total := len(b)
	for len(b) > 0 {
		n := min(3, len(b))
		if _, err := p.desktopPeer.Write(b[:n]); err != nil {
			return 0, err
		}
		b = b[n:]
	}
	return total, nil
}

var _ io.Reader = (*desktopPeer)(nil)
