//go:build !board_biscuit

package control

import (
	"encoding/json"
	"errors"
	"github.com/spf13/cobra"
	featureclaw "github.com/ygelfand/echolocal/internal/feature/openclaw"
	connection "github.com/ygelfand/echolocal/internal/openclaw"
	harness "github.com/ygelfand/libcountertop/pkg/runtime/control"
)

func nodePermissionsCommand() *cobra.Command {
	return harness.Says(&cobra.Command{Use: "node-permissions [GROUP|all on|off]", Short: "Local admin only: read or edit Node grants", Args: cobra.MaximumNArgs(2)}, func(args []string) (string, error) {
		c := featureclaw.Get().Node
		if len(args) > 0 {
			if len(args) != 2 || (args[1] != "on" && args[1] != "off") {
				return "", errors.New("use GROUP|all on|off")
			}
			values := map[string]bool{}
			if args[0] == "all" {
				for _, g := range connection.PermissionGroups {
					values[g] = args[1] == "on"
				}
			} else {
				values[args[0]] = args[1] == "on"
			}
			if e := c.SetNodeGrants(values); e != nil {
				return "", e
			}
		}
		values := map[string]bool{}
		for _, g := range connection.PermissionGroups {
			values[g] = c.NodeAccess().Granted(g)
		}
		b, _ := json.Marshal(values)
		return string(b), nil
	})
}
