//go:build !board_biscuit

package clawtalk

import (
	"context"
	"testing"
)

func TestNodeStopCannotCancelAnotherConversation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := &Manager{active: true, generation: 9, cancel: cancel}
	if m.StopOwned(8) || ctx.Err() != nil {
		t.Fatal("stale node owner cancelled current conversation")
	}
	if !m.StopOwned(9) || ctx.Err() == nil {
		t.Fatal("node did not cancel its own generation")
	}
}
func TestNodeStartRejectsInvalidAndBusyWithoutConfiguration(t *testing.T) {
	m := &Manager{}
	for _, slot := range []int{-1, 2, 99} {
		if _, err := m.StartOwned(slot, false); err == nil {
			t.Fatal("invalid slot accepted")
		}
	}
	m.active = true
	if _, err := m.StartOwned(0, false); err == nil {
		t.Fatal("busy Talk accepted")
	}
	m.active = false
	if _, err := m.StartOwned(0, true); err == nil {
		t.Fatal("busy assistant accepted")
	}
}
