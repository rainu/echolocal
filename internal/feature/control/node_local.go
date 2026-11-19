//go:build !board_biscuit

package control

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/spf13/cobra"
	featureclaw "github.com/ygelfand/echolocal/internal/feature/openclaw"
	harness "github.com/ygelfand/libcountertop/pkg/runtime/control"
	"time"
)

// Local root/ADB diagnostics only; never registered as a Node operation.
// This deliberately uses the SAME executor and grants as an authenticated
// invocation, rather than a privileged path around Node policy.
func nodeExecuteCommand() *cobra.Command {
	return harness.ContextSays(&cobra.Command{Use: "node-execute COMMAND [JSON]", Short: "Local diagnostics through the Node permission executor", Args: cobra.RangeArgs(1, 2)}, func(parent context.Context, args []string) (string, error) {
		raw := json.RawMessage("{}")
		if len(args) == 2 {
			raw = json.RawMessage(args[1])
		}
		ctx, cancel := context.WithTimeout(parent, 30*time.Second)
		defer cancel()
		result, err := featureclaw.Get().Node.NodeAccess().Execute(ctx, args[0], raw)
		if err != nil {
			return "", err
		}
		out, err := json.Marshal(result)
		if err != nil || len(out) > 8*1024*1024 {
			return "", errors.New("result_too_large")
		}
		return string(out), nil
	})
}
func nodeCommandsCommand() *cobra.Command {
	return harness.Says(&cobra.Command{Use: "node-commands", Short: "Local diagnostics: currently granted Node commands", Args: cobra.NoArgs}, func([]string) (string, error) {
		raw, _ := json.Marshal(featureclaw.Get().Node.NodeAccess().Commands())
		return string(raw), nil
	})
}
