package openclaw

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// NodeDeliveredResult defers disruptive work until the Gateway acknowledges the
// tool result. Failure/uncertainty must not execute the deferred operation.
type NodeDeliveredResult interface {
	NodeResultDelivered(bool)
}

// AttachNodeAccess is startup-only; grants are edited through SetNodeGrants.
func (c *Client) AttachNodeAccess(a *NodeAccess) { c.access = a }
func (c *Client) NodeAccess() *NodeAccess        { return c.access }
func (c *Client) SetNodeGrants(g map[string]bool) error {
	if c.access == nil {
		return errors.New("unsupported_command")
	}
	inputOnly, err := c.access.setGrants(g)
	if err != nil {
		return err
	}
	c.op.Lock()
	defer c.op.Unlock()
	inputOn := c.access.Granted("input")
	c.mu.Lock()
	hot := inputOnly && c.rpc != nil && (!inputOn || c.rpc.nodeInputDeclared)
	c.mu.Unlock()
	if hot {
		return nil
	}
	c.stopLocked()
	if c.saved.Enabled {
		c.startLocked()
	}
	return nil
}
func (c *Client) nodeEvent(parent context.Context, r *rpcLink, f frame) {
	if c.node == nil {
		return
	}
	if f.Event == "node.invoke.cancel" {
		var p struct {
			InvokeID string `json:"invokeId"`
		}
		if json.Unmarshal(f.Payload, &p) != nil {
			return
		}
		r.mu.Lock()
		cancel := r.nodeCalls[p.InvokeID]
		r.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		return
	}
	if f.Event != "node.invoke.request" {
		return
	}
	var p struct {
		ID         string `json:"id"`
		NodeID     string `json:"nodeId"`
		Command    string `json:"command"`
		ParamsJSON string `json:"paramsJSON"`
		TimeoutMS  int    `json:"timeoutMs"`
	}
	if json.Unmarshal(f.Payload, &p) != nil || p.ID == "" || p.NodeID == "" {
		return
	}
	timeout := 30 * time.Second
	if p.TimeoutMS > 0 {
		timeout = min(timeout, time.Duration(p.TimeoutMS)*time.Millisecond)
	}
	ctx, cancel := nodeInvocationContext(parent, p.Command, p.TimeoutMS, timeout)
	r.mu.Lock()
	if r.nodeCalls == nil {
		r.nodeCalls = map[string]context.CancelFunc{}
	}
	if r.closed || len(r.nodeCalls) >= 4 || r.nodeCalls[p.ID] != nil {
		r.mu.Unlock()
		cancel()
		return
	}
	r.nodeCalls[p.ID] = cancel
	r.mu.Unlock()
	go func() {
		defer cancel()
		defer func() { r.mu.Lock(); delete(r.nodeCalls, p.ID); r.mu.Unlock() }()
		result, err := c.access.Execute(ctx, p.Command, json.RawMessage(p.ParamsJSON))
		delivered := false
		defer func() {
			if r, ok := result.(NodeDeliveredResult); ok {
				r.NodeResultDelivered(delivered)
			}
		}()
		reply := map[string]any{"id": p.ID, "nodeId": p.NodeID, "ok": err == nil}
		if err != nil {
			code := "UNAVAILABLE"
			var parameterError *ParameterError
			if errors.As(err, &parameterError) {
				code = "INVALID_PARAMS"
			}
			reply["error"] = map[string]string{"code": code, "message": err.Error()}
		} else {
			raw, e := json.Marshal(result)
			if e != nil || len(raw) > 8*1024*1024 {
				reply["ok"] = false
				reply["error"] = map[string]string{"code": "UNAVAILABLE", "message": "result_too_large"}
			} else {
				reply["payloadJSON"] = string(raw)
			}
		}
		delivery, done := context.WithTimeout(parent, 10*time.Second)
		defer done()
		deliveryErr := r.nodeResult(delivery, reply)
		delivered = deliveryErr == nil && reply["ok"] == true && ctx.Err() == nil
		if deliveryErr != nil && deliveryErr.Error() == "rpc_invalid_params" {
			// Return a bounded error instead of silently leaving the invoke pending.
			reply = map[string]any{"id": p.ID, "nodeId": p.NodeID, "ok": false,
				"error": map[string]string{"code": "UNAVAILABLE", "message": "result_too_large"}}
			_ = r.nodeResult(delivery, reply)
		}
	}()
}

// Only desktop.stream uses the Gateway cancellation-owned zero-timeout contract.
// All other commands keep their existing finite cap.
func nodeInvocationContext(parent context.Context, command string, timeoutMS int, bounded time.Duration) (context.Context, context.CancelFunc) {
	if command == "desktop.stream" && timeoutMS == 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, bounded)
}
