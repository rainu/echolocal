//go:build !board_biscuit

package clawtalk

import (
	"context"
	"errors"
	"github.com/ygelfand/echolocal/internal/openclaw/talk"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTransferReservationBlocksEveryStartAndStopsInGap(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := &transferRequest{ctx: ctx, cancel: cancel}
	m := &Manager{transfer: r}
	if !m.Busy() || !m.Route(0, false) {
		t.Fatal("reservation lost")
	}
	if _, err := m.StartOwned(0, false); err == nil {
		t.Fatal("node start entered gap")
	}
	if _, err := m.StartSession("agent:test:new", false); err == nil {
		t.Fatal("UI start entered gap")
	}
	if err := m.StartAutomation(context.Background(), "agent:test:new", "", false); err == nil {
		t.Fatal("automation entered gap")
	}
	if _, err := m.PrepareTransfer(context.Background(), "agent:test:new"); err == nil || err.Error() != "talk_transfer_busy" {
		t.Fatal(err)
	}
	if !m.Stop() || ctx.Err() == nil {
		t.Fatal("stop did not cancel gap")
	}
	if err := m.openTransferred(r, Slot{}); err == nil || m.active {
		t.Fatal("cancelled transfer resurrected")
	}
}
func TestTransferRejectsInvalidAndInactiveWithoutChangingTalk(t *testing.T) {
	m := &Manager{}
	for _, id := range []string{"", "bad", "agent::x", "agent:bad agent:x"} {
		if _, err := m.PrepareTransfer(context.Background(), id); err == nil || err.Error() != "talk_invalid_session" {
			t.Fatal(id, err)
		}
	}
	if _, err := m.PrepareTransfer(context.Background(), "agent:test:next"); err == nil || err.Error() != "talk_not_ready" {
		t.Fatal(err)
	}
	if m.active || m.transfer != nil {
		t.Fatal("rejection changed state")
	}
}
func TestTransferReceiptStartsAtMostOnce(t *testing.T) {
	var calls atomic.Int32
	r := &TransferResult{delivered: func(bool) { calls.Add(1) }}
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); r.NodeResultDelivered(true) }()
	}
	wg.Wait()
	r.NodeResultDelivered(false)
	if calls.Load() != 1 {
		t.Fatal(calls.Load())
	}
}
func TestTransferFlowDeliveryConfirmationRecoveryAndCancellation(t *testing.T) {
	for _, tc := range []struct {
		name          string
		delivered     bool
		confirmErr    bool
		openFailures  int
		cancelConfirm bool
		state         string
		opens         int
	}{
		{"delivery_failed", false, false, 0, false, "cancelled", 0},
		{"confirmation_failed", true, true, 0, false, "failed", 0},
		{"completed", true, false, 0, false, "completed", 1},
		{"recovered", true, false, 1, false, "recovered", 2},
		{"recovery_failed", true, false, 2, false, "failed", 2},
		{"stop_confirmation", true, false, 0, true, "cancelled", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			b := talk.NewSession(nil, "agent:test:old", nil)
			r := &transferRequest{ctx: ctx, cancel: cancel, generation: 5, backend: b, previous: Slot{Context: "agent:test:old"}, target: Slot{Context: "agent:test:next"}, delivered: make(chan bool, 1)}
			m := &Manager{ctx: context.Background(), active: true, generation: 5, lastBackend: b, transfer: r}
			var retired atomic.Bool
			m.cancel = func() { retired.Store(true); go func() { m.mu.Lock(); m.active = false; m.mu.Unlock() }() }
			confirmed := false
			r.announce = func(context.Context, string) error {
				if retired.Load() {
					t.Fatal("closed before confirmation")
				}
				if tc.cancelConfirm {
					m.Stop()
					return errors.New("cancelled")
				}
				if tc.confirmErr {
					return errors.New("talk_confirmation_failed")
				}
				confirmed = true
				return nil
			}
			opens := 0
			r.open = func(_ *transferRequest, v Slot) error {
				if !retired.Load() || !confirmed {
					t.Fatal("opened before audible confirmation/retirement")
				}
				m.mu.Lock()
				active := m.active
				m.mu.Unlock()
				if active {
					t.Fatal("overlapping activation")
				}
				opens++
				if opens == 1 && v.Context != "agent:test:next" || opens == 2 && v.Context != "agent:test:old" {
					t.Fatal("wrong target")
				}
				if opens <= tc.openFailures {
					return errors.New("synthetic failure")
				}
				return nil
			}
			r.delivered <- tc.delivered
			m.runTransfer(r)
			if m.transferState != tc.state || opens != tc.opens || m.transfer != nil {
				t.Fatal(m.transferState, opens)
			}
			if (!tc.delivered || tc.confirmErr) && retired.Load() {
				t.Fatal("unaccepted transfer ended current voice")
			}
			if tc.state == "recovered" && m.transferReason != "talk_target_open_failed" {
				t.Fatal("false success")
			}
		})
	}
}
func TestAcceptedTransferCannotRetireReplacement(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	old := talk.NewSession(nil, "agent:test:old", nil)
	newer := talk.NewSession(nil, "agent:test:new", nil)
	r := &transferRequest{ctx: ctx, cancel: cancel, generation: 1, backend: old, delivered: make(chan bool, 1)}
	var retired atomic.Bool
	m := &Manager{active: true, generation: 2, lastBackend: newer, transfer: r, cancel: func() { retired.Store(true) }}
	r.delivered <- true
	m.runTransfer(r)
	if retired.Load() || m.generation != 2 || m.lastBackend != newer {
		t.Fatal("retired replacement")
	}
}
func TestTransferCleanupCannotCancelNextRequest(t *testing.T) {
	first, c1 := context.WithCancel(context.Background())
	defer c1()
	next, c2 := context.WithCancel(context.Background())
	defer c2()
	old := &transferRequest{ctx: first, cancel: c1}
	current := &transferRequest{ctx: next, cancel: c2}
	m := &Manager{transfer: current, transferState: "accepted"}
	m.finishTransfer(old, "failed", "old")
	if m.transfer != current || next.Err() != nil || m.transferState != "accepted" {
		t.Fatal("old cleanup touched new request")
	}
	m.CancelTransfer()
	if next.Err() == nil {
		t.Fatal("disconnect did not cancel pending transfer")
	}
}
func TestTransferStopOwnedRespectsGeneration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := &transferRequest{ctx: ctx, cancel: cancel, generation: 9}
	m := &Manager{active: true, generation: 9, transfer: r, cancel: func() {}}
	if m.StopOwned(8) || ctx.Err() != nil {
		t.Fatal("old owner cancelled transfer")
	}
	if !m.StopOwned(9) || ctx.Err() == nil {
		t.Fatal("owner did not cancel transfer")
	}
	wait, done := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer done()
	if err := m.waitTransferRelease(wait); err == nil {
		t.Fatal("close timeout ignored")
	}
}

func TestTransferCompletionDetachesCancellationOnlyAtCommit(t *testing.T) {
	for _, cancelBefore := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		triggered := make(chan struct{})
		r := &transferRequest{ctx: ctx, cancel: cancel}
		r.stopSetup = context.AfterFunc(ctx, func() { close(triggered) })
		m := &Manager{transfer: r}
		if cancelBefore {
			cancel()
			<-triggered
		}
		m.finishTransfer(r, "completed", "")
		if cancelBefore && m.transferState != "cancelled" {
			t.Fatal("cancelled setup reported success")
		}
		if !cancelBefore {
			if m.transferState != "completed" {
				t.Fatal(m.transferState)
			}
			select {
			case <-triggered:
				t.Fatal("completed Talk cancelled by request cleanup")
			case <-time.After(20 * time.Millisecond):
			}
		}
	}
}
