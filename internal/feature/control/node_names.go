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
	if op.Parameters == nil {
		op.Description, op.Parameters = nodeContract(strings.TrimPrefix(op.Command, "echolocal."))
	}
	if op.Parameters == nil || op.Description == "" {
		panic("Node command lacks reviewed metadata: " + op.Command)
	}
	op.Command = nodeCommandName(op.Command)
	if err := a.Register(op); err != nil {
		panic("Node command registration failed: " + op.Command)
	}
	return nil
}
