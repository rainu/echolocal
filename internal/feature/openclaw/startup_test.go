//go:build !board_biscuit

package openclaw

import (
	"github.com/ygelfand/libcountertop/pkg/runtime/startup"
	"testing"
)

func TestConnectionStartupIncludesNode(t *testing.T) {
	connected := startup.Progress{Done: true, Doing: "connected"}
	pending := startup.Progress{Doing: "connecting"}
	failed := startup.Progress{Failed: true, Doing: "offline"}
	for _, tt := range []struct {
		ui, node startup.Progress
		enabled  bool
		want     startup.Progress
	}{
		{connected, pending, true, pending},
		{connected, failed, true, failed},
		{pending, connected, true, pending},
		{failed, connected, true, failed},
		{connected, connected, true, connected},
		{startup.Progress{Done: true, Doing: "disabled"}, connected, false, connected},
	} {
		if got := connectionStartup(tt.ui, tt.node, tt.enabled); got != tt.want {
			t.Fatalf("got %+v want %+v", got, tt.want)
		}
	}
}
