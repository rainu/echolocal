//go:build !board_biscuit

package openclaw

import (
	"context"
	"sync"
	"time"

	"github.com/ygelfand/echolocal/internal/feature/dhcp"
	"github.com/ygelfand/echolocal/internal/feature/shell"
	"github.com/ygelfand/echolocal/internal/feature/timesync"
	"github.com/ygelfand/libcountertop/pkg/runtime/startup"

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
	ready := func() string {
		if b.Has(board.Wifi) && !dhcp.Get().Startup().Done {
			return "network"
		}
		if !timesync.Get().Valid() {
			return "clock"
		}
		return ""
	}
	s.Client.SetReadiness(ready)
	s.Node.SetReadiness(ready)
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
	desktop := s.Node.DesktopStatus()
	for {
		select {
		case <-ctx.Done():
			s.Node.Close()
			return s.Client.Close()
		case <-tick.C:
			nextUI, nextNode := s.Client.Status(), s.Node.Status()
			nextDesktop := s.Node.DesktopStatus()
			if nextUI != ui || nextNode != node || nextDesktop.State != desktop.State {
				desktop = nextDesktop
				ui, node = nextUI, nextNode
				shell.Get().Redraw()
			}
		}
	}
}

// Both configured roles participate in boot readiness; UI alone is not enough.
func (s *Service) Startup() startup.Progress {
	return connectionStartup(s.Client.Startup(), s.Node.Startup(), s.Client.Status().Enabled)
}
func connectionStartup(ui, node startup.Progress, uiEnabled bool) startup.Progress {
	for _, p := range []startup.Progress{ui, node} {
		if p.Failed {
			return p
		}
	}
	for _, p := range []startup.Progress{ui, node} {
		if !p.Done {
			return p
		}
	}
	if uiEnabled {
		return ui
	}
	return node
}
