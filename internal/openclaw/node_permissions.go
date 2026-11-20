package openclaw

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

var PermissionGroups = []string{"screen", "input", "display", "settings", "audio", "alarms", "intercom", "camera", "state", "diagnostics", "maintenance", "tones", "talk"}

type NodeOperation struct {
	Command, Group string
	Description    string
	Parameters     map[string]any
	Run            func(context.Context, json.RawMessage) (any, error)
}
type NodeAccess struct {
	mu               sync.Mutex
	path             string
	grants           map[string]bool
	operations       map[string]NodeOperation
	active           map[uint64]context.CancelFunc
	next, generation uint64
	storageErr       bool
	cleanup          []func()
	changed          chan struct{}
}

func NewNodeAccess(path string) *NodeAccess {
	a := &NodeAccess{changed: make(chan struct{}, 1), path: path, grants: map[string]bool{}, operations: map[string]NodeOperation{}, active: map[uint64]context.CancelFunc{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return a
	}
	a.storageErr = err != nil || json.Unmarshal(data, &a.grants) != nil || a.grants == nil
	return a
}
func permissionGroup(g string) bool {
	for _, name := range PermissionGroups {
		if name == g {
			return true
		}
	}
	return false
}
func (a *NodeAccess) granted(g string) bool {
	v, ok := a.grants[g]
	return permissionGroup(g) && !a.storageErr && (!ok || v)
}
func (a *NodeAccess) Granted(g string) bool { a.mu.Lock(); defer a.mu.Unlock(); return a.granted(g) }

// SetGrants must remain local/admin-owned, never a Node command. Policy edits
// cancel active work and fence results. Missing fields default on after upgrade.
func (a *NodeAccess) SetGrants(changes map[string]bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	for g := range changes {
		if !permissionGroup(g) {
			return errors.New("permission_unknown")
		}
	}
	next := map[string]bool{}
	for _, g := range PermissionGroups {
		next[g] = a.granted(g)
	}
	for g, on := range changes {
		next[g] = on
	}
	data, _ := json.Marshal(next)
	if os.MkdirAll(filepath.Dir(a.path), 0700) != nil {
		return errors.New("permissions_storage")
	}
	f, err := os.CreateTemp(filepath.Dir(a.path), ".node-permissions-*")
	if err != nil {
		return errors.New("permissions_storage")
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	ce := f.Close()
	if err == nil {
		err = ce
	}
	if err == nil {
		err = os.Rename(f.Name(), a.path)
	}
	if err != nil {
		return errors.New("permissions_storage")
	}
	a.grants = next
	a.storageErr = false
	a.cancel()
	a.changedLocked()
	return nil
}
func (a *NodeAccess) OnCleanup(fn func()) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cleanup = append(a.cleanup, fn)
}
func (a *NodeAccess) cancel() {
	for _, fn := range a.cleanup {
		fn()
	}
	a.generation++
	for _, cancel := range a.active {
		cancel()
	}
}
func (a *NodeAccess) Disconnect() { a.mu.Lock(); defer a.mu.Unlock(); a.cancel() }
func (a *NodeAccess) Register(op NodeOperation) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if op.Command == "" || !permissionGroup(op.Group) || op.Run == nil {
		return errors.New("operation_invalid")
	}
	if _, ok := a.operations[op.Command]; ok {
		return errors.New("operation_duplicate")
	}
	if op.Parameters != nil {
		var detached map[string]any
		data, err := json.Marshal(op.Parameters)
		if err != nil || len(data) > 16384 || json.Unmarshal(data, &detached) != nil || (op.Description == "" || len(op.Description) > 1024) || !safeToolName.MatchString(toolName(op.Command)) {
			return errors.New("operation_metadata_invalid")
		}
		op.Parameters = detached
		for command := range a.operations {
			if toolName(command) == toolName(op.Command) {
				return errors.New("operation_tool_name_collision")
			}
		}
	}
	a.operations[op.Command] = op
	a.changedLocked()
	return nil
}
func (a *NodeAccess) Commands() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := []string{}
	for name, op := range a.operations {
		if a.granted(op.Group) {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}
func (a *NodeAccess) Execute(ctx context.Context, name string, params json.RawMessage) (any, error) {
	if len(params) > 128*1024 || (len(params) > 0 && !json.Valid(params)) {
		return nil, errors.New("invalid_params")
	}
	a.mu.Lock()
	op, ok := a.operations[name]
	if !ok {
		a.mu.Unlock()
		return nil, errors.New("unsupported_command")
	}
	if !a.granted(op.Group) {
		a.mu.Unlock()
		return nil, errors.New("permission_denied")
	}
	if len(a.active) >= 4 {
		a.mu.Unlock()
		return nil, errors.New("busy")
	}
	ctx, cancel := context.WithCancel(ctx)
	a.next++
	id, gen := a.next, a.generation
	a.active[id] = cancel
	a.mu.Unlock()
	defer func() { cancel(); a.mu.Lock(); delete(a.active, id); a.mu.Unlock() }()
	if ctx.Err() != nil {
		return nil, errors.New("cancelled")
	}
	if op.Parameters != nil {
		if err := validateNodeParameters(params, op.Parameters); err != nil {
			return nil, err
		}
	}
	result, err := op.Run(ctx, params)
	if err != nil && err.Error() == "invalid_params" && op.Parameters != nil {
		err = invalidParameter("$", "expected the documented parameter shape and field constraints")
	}
	a.mu.Lock()
	stale := gen != a.generation || !a.granted(op.Group)
	a.mu.Unlock()
	if stale || ctx.Err() != nil {
		return nil, errors.New("cancelled")
	}
	return result, err
}
