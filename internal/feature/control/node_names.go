//go:build !board_biscuit

package control

import (
	connection "github.com/ygelfand/echolocal/internal/openclaw"
	"strings"
)

// Only reviewed OpenClaw wire contracts are unprefixed.
func nodeCommandName(name string) string {
	switch name {
	case "screen.snapshot", "screen.record", "camera.snap", "device.info", "device.status", "device.health", "device.permissions", "system.notify":
		return name
	}
	if strings.HasPrefix(name, "echolocal.") {
		return name
	}
	return "echolocal." + name
}
func nodeRegister(a *connection.NodeAccess, op connection.NodeOperation) error {
	op.Command = nodeCommandName(op.Command)
	return a.Register(op)
}
