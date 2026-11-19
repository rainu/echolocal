package openclaw

import (
	"context"
	"encoding/json"
	"time"
)

// nodeHealth reuses the diagnostics executor, including revocation cancellation
// and result fencing. No collector runs for an operator-only connection.
func (c *Client) nodeHealth(ctx context.Context, r *rpcLink) {
	if c.node == nil || c.access == nil {
		return
	}
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		c.access.mu.Lock()
		generation := c.access.generation
		c.access.mu.Unlock()
		result, err := c.access.Execute(ctx, "echolocal.diagnostics.status", json.RawMessage("{}"))
		if err == nil {
			if status, ok := result.(map[string]any); ok {
				if stats, ok := status["hostStats"]; ok && stats != nil {
					// Hold the policy lock through delivery so a completed revocation cannot
					// race a final outbound metric. Requests have a strict short timeout.
					c.access.mu.Lock()
					if c.access.granted("diagnostics") && generation == c.access.generation && ctx.Err() == nil {
						raw, e := json.Marshal(stats)
						if e == nil {
							send, done := context.WithTimeout(ctx, 3*time.Second)
							_ = r.call(send, "node.event", map[string]any{"event": "node.host.stats", "payloadJSON": string(raw)}, nil)
							done()
						}
					}
					c.access.mu.Unlock()
				}
			}
		}
		timer.Reset(time.Minute)
	}
}
