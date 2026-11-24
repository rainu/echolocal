package openclaw

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type desktopInputSpy struct {
	keys, pointers, closed int
	symbol                 uint32
	x, y                   int
}

func (s *desktopInputSpy) Key(_ context.Context, down bool, k uint32) { s.keys++; s.symbol = k }
func (s *desktopInputSpy) Pointer(_ context.Context, b byte, x, y int) {
	s.pointers++
	s.x = x
	s.y = y
}
func (s *desktopInputSpy) Close() { s.closed++ }
func TestDesktopRFBInputDispatchAndClipboardIsolation(t *testing.T) {
	spy := &desktopInputSpy{}
	wire := bytes.NewBuffer([]byte{4, 1, 0, 0, 0, 0, 0, 65, 5, 1, 0, 100, 0, 50, 6, 0, 0, 0, 0, 0, 0, 3, 'x', 'y', 'z'})
	_ = readRFBEvents(context.Background(), wire, make(chan rfbEvent, 8), spy)
	if spy.keys != 1 || spy.symbol != 65 || spy.pointers != 1 || spy.x != 100 || spy.y != 50 {
		t.Fatal("wire input not dispatched")
	}
}
func TestInputGrantReductionKeepsDesktopAndCancelsInput(t *testing.T) {
	a := NewNodeAccess(filepath.Join(t.TempDir(), "grants"))
	a.SetGrants(map[string]bool{"desktop": true})
	started := make(chan string, 2)
	done := make(chan string, 2)
	for _, group := range []string{"desktop", "input"} {
		a.Register(NodeOperation{Command: group, Group: group, Run: func(ctx context.Context, _ json.RawMessage) (any, error) {
			started <- group
			<-ctx.Done()
			done <- group
			return nil, nil
		}})
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go a.Execute(ctx, "desktop", nil)
	go a.Execute(ctx, "input", nil)
	<-started
	<-started
	full := map[string]bool{}
	for _, g := range PermissionGroups {
		full[g] = a.Granted(g)
	}
	full["input"] = false
	only, err := a.setGrants(full)
	if err != nil || !only {
		t.Fatal("not input-only")
	}
	select {
	case group := <-done:
		if group != "input" {
			t.Fatal("capture cancelled by input grant")
		}
	case <-time.After(time.Second):
		t.Fatal("input not cancelled")
	}
	if _, err = a.Execute(ctx, "input", nil); err == nil {
		t.Fatal("revoked input accepted")
	}
	select {
	case <-done:
		t.Fatal("desktop did not remain active")
	default:
	}
	a.Disconnect()
	select {
	case group := <-done:
		if group != "desktop" {
			t.Fatal("wrong operation")
		}
	case <-time.After(time.Second):
		t.Fatal("disconnect did not cancel capture")
	}
}

func TestClientInputOnlyEditKeepsCurrentConnection(t *testing.T) {
	c := NewNode(filepath.Join(t.TempDir(), "node"), "test", NodeMetadata{})
	a := NewNodeAccess(filepath.Join(t.TempDir(), "grants"))
	c.AttachNodeAccess(a)
	link := &rpcLink{nodeInputDeclared: true}
	c.rpc = link
	epoch := a.InputGeneration()
	if err := c.SetNodeGrants(map[string]bool{"input": false}); err != nil {
		t.Fatal(err)
	}
	if c.rpc != link || a.Granted("input") || a.InputGeneration() != epoch+1 {
		t.Fatal("revocation reconnected or lost epoch")
	}
	if err := c.SetNodeGrants(map[string]bool{"input": true}); err != nil {
		t.Fatal(err)
	}
	if c.rpc != link || !a.Granted("input") || a.InputGeneration() != epoch+2 {
		t.Fatal("restoration reconnected or lost epoch")
	}
}

type closingDesktopInput struct {
	once   sync.Once
	closed chan struct{}
}

func (s *closingDesktopInput) Key(context.Context, bool, uint32)       {}
func (s *closingDesktopInput) Pointer(context.Context, byte, int, int) {}
func (s *closingDesktopInput) Close()                                  { s.once.Do(func() { close(s.closed) }) }
func TestReaderDisconnectCancelsInputBeforeBlockedCaptureReturns(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()
	client.SetDeadline(time.Now().Add(3 * time.Second))
	captured, release := make(chan struct{}), make(chan struct{})
	input := &closingDesktopInput{closed: make(chan struct{})}
	done := make(chan error, 1)
	go func() {
		defer server.Close()
		done <- serveDesktop(context.Background(), server, "12345678", 4, 2, func(context.Context) (image.Image, error) {
			close(captured)
			<-release
			return image.NewRGBA(image.Rect(0, 0, 4, 2)), nil
		}, func() {}, input)
	}()
	clientRFBHandshake(t, client, "12345678", 4, 2)
	client.Write(updateRequest())
	<-captured
	client.Close()
	select {
	case <-input.closed:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("capture blocked contact cleanup")
	}
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("capture did not stop")
	}
}
