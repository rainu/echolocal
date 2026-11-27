//go:build !board_biscuit

package actions

import (
	"github.com/ygelfand/echolocal/internal/board"
	esphome "github.com/ygelfand/go-esphome-device"
	"testing"
)

func TestNativeTalkActionsAreDiscoverableAndAnswer(t *testing.T) {
	all := Get().Actions()
	names := map[string]*esphome.Action{}
	for _, a := range all {
		if names[a.Name] != nil {
			t.Fatal("duplicate", a.Name)
		}
		names[a.Name] = a
	}
	if !board.Detect().Has(board.Panel) {
		for _, name := range []string{"openclaw_talk_start", "openclaw_talk_stop", "openclaw_sessions", "openclaw_talk_transfer", "openclaw_talk_status"} {
			if names[name] != nil {
				t.Fatal("Talk on non-panel board")
			}
		}
		return
	}
	for _, name := range []string{"openclaw_talk_start", "openclaw_talk_stop", "openclaw_sessions", "openclaw_talk_transfer", "openclaw_talk_status"} {
		a := names[name]
		if a == nil || !a.Answers || a.Run == nil {
			t.Fatal("missing answering action", name)
		}
	}
	for _, old := range []string{"talk_start", "talk_stop"} {
		if names[old] != nil {
			t.Fatal("unexpected unprefixed Talk action", old)
		}
	}
	transfer := names["openclaw_talk_transfer"]
	if len(transfer.Args) != 1 || transfer.Args[0].Name != "session_id" || transfer.Args[0].Type != esphome.ArgString {
		t.Fatal(transfer.Args)
	}
	start := names["openclaw_talk_start"]
	if len(start.Args) != 2 || start.Args[0].Name != "session_id" || start.Args[1].Name != "startup_prompt" {
		t.Fatal(start.Args)
	}
	for _, a := range start.Args {
		if a.Type != esphome.ArgString {
			t.Fatal(a)
		}
	}
}
