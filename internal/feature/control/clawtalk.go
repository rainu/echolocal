//go:build !board_biscuit

package control

import (
	"encoding/json"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/ygelfand/echolocal/internal/feature/clawtalk"
	"github.com/ygelfand/echolocal/internal/feature/voice"
	harness "github.com/ygelfand/libcountertop/pkg/runtime/control"
	"strconv"
)

func talkCommands(t *harness.Tree) {
	root := &cobra.Command{Use: "talk", Short: "Bounded Talk lifecycle diagnostics"}
	root.AddCommand(&cobra.Command{Use: "show", Args: cobra.NoArgs, Short: "Show current or last Talk without starting capture", Run: func(cmd *cobra.Command, _ []string) {
		clawtalk.Get().Show()
		fmt.Fprintln(cmd.OutOrStdout(), "shown without starting capture")
	}})
	root.AddCommand(&cobra.Command{Use: "status", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(clawtalk.Get().Status())
	}})
	root.AddCommand(&cobra.Command{Use: "stop", Args: cobra.NoArgs, Run: func(cmd *cobra.Command, _ []string) {
		clawtalk.Get().Stop()
		fmt.Fprintln(cmd.OutOrStdout(), "stopped")
	}})
	root.AddCommand(&cobra.Command{Use: "start [slot]", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		slot, e := strconv.Atoi(args[0])
		if e != nil || slot < 1 || slot > 2 {
			return fmt.Errorf("invalid slot")
		}
		v := voice.Get()
		if !clawtalk.Get().Store.Get(slot - 1).OpenClaw {
			return fmt.Errorf("target is not OpenClaw")
		}
		clawtalk.Get().Route(slot-1, v.Turn().Busy() || v.Turn().Pending())
		fmt.Fprintln(cmd.OutOrStdout(), "activation requested")
		return nil
	}})
	t.Top(root)
}
