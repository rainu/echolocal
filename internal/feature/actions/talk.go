//go:build !board_biscuit

package actions

import (
	"context"
	"errors"
	"time"

	"github.com/ygelfand/echolocal/internal/board"
	"github.com/ygelfand/echolocal/internal/feature/clawtalk"
	claw "github.com/ygelfand/echolocal/internal/feature/openclaw"
	"github.com/ygelfand/echolocal/internal/feature/voice"
	esphome "github.com/ygelfand/go-esphome-device"
)

func talkActions() []*esphome.Action {
	if !board.Detect().Has(board.Panel) {
		return nil
	}
	return []*esphome.Action{
		{Name: "openclaw_talk_start", Description: "Start Talk; empty arguments use assistant 1 defaults for this call only.", Answers: true,
			Args: []esphome.Arg{{Name: "session_id", Type: esphome.ArgString}, {Name: "startup_prompt", Type: esphome.ArgString}},
			Run: func(c esphome.Call) (any, error) {
				ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
				defer cancel()
				v := voice.Get()
				if err := clawtalk.Get().StartAutomation(ctx, c.String("session_id"), c.String("startup_prompt"),
					v.Turn().Busy() || v.Turn().Pending()); err != nil {
					return nil, err
				}
				return map[string]any{"success": true, "active": true}, nil
			}},
		{Name: "openclaw_talk_stop", Description: "Stop only the voice conversation; succeeds when already stopped.", Answers: true,
			Run: func(esphome.Call) (any, error) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := clawtalk.Get().StopAutomation(ctx); err != nil {
					return nil, err
				}
				return map[string]any{"success": true, "active": false}, nil
			}},
		{Name: "openclaw_sessions", Description: "List available sessions as routing IDs and human-readable names.", Answers: true,
			Run: func(esphome.Call) (any, error) {
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				bound, err := claw.Get().BindTalk()
				if err != nil {
					return nil, errors.New("talk_connection_unavailable")
				}
				rows, err := clawtalk.AutomationSessions(ctx, bound)
				if err != nil {
					return nil, err
				}
				return map[string]any{"sessions": rows}, nil
			}},
	}
}
