//go:build !board_biscuit

package openclaw

import (
	"context"
	"github.com/ygelfand/echolocal/internal/feature/shell"
	"sync"
	"time"

	"github.com/ygelfand/echolocal/internal/board"
	"github.com/ygelfand/echolocal/internal/component"
	"github.com/ygelfand/echolocal/internal/layout"
	connection "github.com/ygelfand/echolocal/internal/openclaw"
	sharedcomponent "github.com/ygelfand/libcountertop/pkg/runtime/component"
)

func init() {
	component.Register(sharedcomponent.Network, Get, sharedcomponent.Order(25), component.Needs(board.Panel))
}

type Service struct {
	*connection.Client
	Node        *connection.Client
	Connections *connection.Connections
}

var NodeSetup func(*connection.NodeAccess)

var get = sync.OnceValue(func() *Service {
	b := board.Detect()
	s := &Service{Client: connection.New(layout.StateDir+"/openclaw.json", layout.Version), Node: connection.NewNode(layout.StateDir+"/openclaw-node.json", layout.Version, connection.NodeMetadata{Name: b.DefaultName, Model: b.Model})}
	s.Connections = connection.NewConnections(layout.StateDir+"/openclaw-connection.json", s.Client, s.Node, b.DefaultName+" · UI", b.DefaultName+" · Node")
	s.Node.AttachNodeAccess(connection.NewNodeAccess(layout.StateDir + "/openclaw-node-permissions.json"))
	return s
})

func Get() *Service             { return get() }
func (s *Service) Name() string { return "openclaw" }
func (s *Service) Run(ctx context.Context) error {
	if ctx.Err() != nil {
		return nil
	}
	s.Client.Start()
	if NodeSetup != nil {
		NodeSetup(s.Node.NodeAccess())
	}
	s.Node.Start()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	ui, node := s.Client.Status(), s.Node.Status()
	for {
		select {
		case <-ctx.Done():
			s.Node.Close()
			return s.Client.Close()
		case <-tick.C:
			nextUI, nextNode := s.Client.Status(), s.Node.Status()
			if nextUI != ui || nextNode != node {
				ui, node = nextUI, nextNode
				shell.Get().Redraw()
			}
		}
	}
}
