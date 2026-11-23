package openclaw

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	"io"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const desktopFrameLimit = 64 * 1024

// DesktopStatus deliberately contains no bearer capabilities or destinations.
type DesktopStatus struct {
	State  string `json:"state"`
	Frames uint64 `json:"frames"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}
type desktopSession struct {
	mu     sync.Mutex
	cancel context.CancelFunc
	status DesktopStatus
}

func (c *Client) DesktopStatus() DesktopStatus {
	c.desktop.mu.Lock()
	defer c.desktop.mu.Unlock()
	s := c.desktop.status
	if s.State == "" {
		s.State = "stopped"
	}
	return s
}
func (c *Client) StopDesktop() {
	c.desktop.mu.Lock()
	defer c.desktop.mu.Unlock()
	if c.desktop.cancel != nil {
		c.desktop.cancel()
	}
}

// The attach target is derived only from the currently authenticated Node link,
// never from operator state or a caller-provided host. Match the ticket exactly.
func desktopAttachURL(origin, ticket, attachPath string) (string, error) {
	bad := errors.New("invalid_params")
	if ValidateEndpoint(origin) != nil || len(ticket) < 16 || len(ticket) > 512 || strings.ContainsAny(ticket, "\x00\r\n") {
		return "", bad
	}
	a, err := url.Parse(attachPath)
	if err != nil || a.IsAbs() || a.Host != "" || a.User != nil || a.Fragment != "" || a.Path != "/node-desktop/attach" || a.RawPath != "" {
		return "", bad
	}
	query, err := url.ParseQuery(a.RawQuery)
	if err != nil || len(query) != 1 || len(query["ticket"]) != 1 || query.Get("ticket") != ticket {
		return "", bad
	}
	u, _ := url.Parse(origin)
	u.Path = strings.TrimRight(u.Path, "/") + a.Path
	u.RawPath = ""
	u.RawQuery = url.Values{"ticket": {ticket}}.Encode()
	return u.String(), nil
}

// StreamDesktop owns exactly one outbound carrier, capture operation and RFB
// state machine. Called only by the permission-gated desktop.stream operation.
func (c *Client) StreamDesktop(ctx context.Context, raw json.RawMessage, w, h int, capture func(context.Context) (image.Image, error)) (any, error) {
	var p struct {
		Ticket     string `json:"ticket"`
		AttachPath string `json:"attachPath"`
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&p) != nil || d.Decode(new(any)) != io.EOF || w < 1 || h < 1 || w > 1280 || h > 1280 {
		return nil, errors.New("invalid_params")
	}
	c.mu.Lock()
	link := c.rpc
	c.mu.Unlock()
	if c.node == nil || link == nil {
		return nil, errors.New("stream_unavailable")
	}
	target, err := desktopAttachURL(link.origin, p.Ticket, p.AttachPath)
	if err != nil {
		return nil, err
	}
	if c.access == nil || !c.access.Granted("desktop") {
		return nil, errors.New("permission_denied")
	}
	ctx, cancel := context.WithCancel(ctx)
	c.desktop.mu.Lock()
	if c.desktop.cancel != nil {
		c.desktop.mu.Unlock()
		cancel()
		return nil, errors.New("busy")
	}
	c.desktop.cancel = cancel
	c.desktop.status = DesktopStatus{State: "starting", Width: w, Height: h}
	c.desktop.mu.Unlock()
	defer func() {
		cancel()
		c.desktop.mu.Lock()
		c.desktop.cancel = nil
		c.desktop.status.State = "stopped"
		c.desktop.mu.Unlock()
	}()
	conn, resp, err := c.dialer.DialContext(ctx, target, nil)
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	if err != nil {
		return nil, errors.New("stream_attach_failed")
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	conn.SetReadLimit(desktopFrameLimit)
	conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	secret := make([]byte, 4)
	if _, err = rand.Read(secret); err != nil {
		return nil, errors.New("stream_unavailable")
	}
	password := hex.EncodeToString(secret)
	peer := &desktopPeer{conn: conn, timeout: 15 * time.Second}
	meta, _ := json.Marshal(map[string]string{"auth": "vnc-password", "vncPassword": password})
	if _, err = peer.Write(meta); err != nil {
		return nil, errors.New("stream_attach_failed")
	}
	err = serveDesktop(ctx, peer, password, w, h, capture, func() {
		c.desktop.mu.Lock()
		c.desktop.status.State = "streaming"
		c.desktop.status.Frames++
		c.desktop.mu.Unlock()
	})
	if ctx.Err() != nil {
		return nil, errors.New("cancelled")
	}
	if err != nil {
		return nil, errors.New("stream_ended")
	}
	return map[string]string{"status": "stopped"}, nil
}

// One reader and one writer; frames are binary, ordered and bounded. The RFB
// parser can read across arbitrary carrier fragmentation without buffering it all.
type desktopPeer struct {
	conn    *websocket.Conn
	reader  io.Reader
	timeout time.Duration
}

func (p *desktopPeer) Read(b []byte) (int, error) {
	for {
		if p.reader == nil {
			p.conn.SetReadDeadline(time.Now().Add(p.timeout))
			typ, r, err := p.conn.NextReader()
			if err != nil {
				return 0, err
			}
			if typ != websocket.BinaryMessage {
				return 0, errors.New("binary_required")
			}
			p.reader = r
		}
		n, err := p.reader.Read(b)
		if err == io.EOF {
			p.reader = nil
			if n > 0 {
				return n, nil
			}
			continue
		}
		return n, err
	}
}
func (p *desktopPeer) Write(b []byte) (int, error) {
	total := 0
	for len(b) > 0 {
		chunk := b[:min(len(b), desktopFrameLimit)]
		p.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := p.conn.WriteMessage(websocket.BinaryMessage, chunk); err != nil {
			return total, err
		}
		total += len(chunk)
		b = b[len(chunk):]
	}
	return total, nil
}
func (p *desktopPeer) ready() { p.timeout = 45 * time.Second }
