//go:build !board_biscuit

package openclaw

import (
	"context"
	"sync"

	"github.com/ygelfand/echolocal/internal/board"
	"github.com/ygelfand/echolocal/internal/component"
	"github.com/ygelfand/echolocal/internal/layout"
	connection "github.com/ygelfand/echolocal/internal/openclaw"
	sharedcomponent "github.com/ygelfand/libcountertop/pkg/runtime/component"
)

func init() {
	component.Register(sharedcomponent.Network, Get, sharedcomponent.Order(25), component.Needs(board.Panel))
}

type Service struct{ *connection.Client }

var get = sync.OnceValue(func() *Service { return &Service{connection.New(layout.StateDir+"/openclaw.json", layout.Version)} })

func Get() *Service             { return get() }
func (s *Service) Name() string { return "openclaw" }
func (s *Service) Run(ctx context.Context) error {
	if ctx.Err() != nil {
		return nil
	}
	s.Client.Start()
	<-ctx.Done()
	return s.Client.Close()
}
