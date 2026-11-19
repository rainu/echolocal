//go:build !board_biscuit

package control

import (
	"github.com/ygelfand/echolocal/internal/feature/call"
	connection "github.com/ygelfand/echolocal/internal/openclaw"
	"sync"
)

var nodeCallOwner struct {
	sync.Mutex
	id string
}

func nodeTrackCall(before string) {
	now, ok := call.Get().Now()
	if ok && now.ID != before {
		nodeCallOwner.Lock()
		nodeCallOwner.id = now.ID
		nodeCallOwner.Unlock()
	}
}
func nodeCallID() string {
	c, ok := call.Get().Now()
	if !ok {
		return ""
	}
	return c.ID
}
func nodeInstallCleanup(a *connection.NodeAccess) {
	a.OnCleanup(func() {
		nodeCallOwner.Lock()
		defer nodeCallOwner.Unlock()
		if nodeCallOwner.id != "" && nodeCallID() == nodeCallOwner.id {
			call.Get().Hangup()
		}
		nodeCallOwner.id = ""
	})
}
