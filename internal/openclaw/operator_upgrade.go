package openclaw

import (
	"context"
	"reflect"
)

// awaitOperatorUpgrade runs only on the same identity's zero-scope operator
// connection. These owner-bound protocol methods need no administrative scope.
// No request IDs, tokens, server messages or raw results enter diagnostics.
func (c *Client) awaitOperatorUpgrade(ctx context.Context, r *rpcLink, scopes []string) {
	var request struct{ RequestID string }
	err := r.call(ctx, "device.scopes.requestUpgrade", map[string]any{"scopes": upgradeRequestScopes(scopes)}, &request)
	reason := "scope_upgrade_rejected"
	var result struct {
		Status, DeviceToken string
		Scopes              []string
	}
	if err == nil && request.RequestID != "" {
		err = r.call(ctx, "device.scopes.waitUpgrade", map[string]any{"requestId": request.RequestID}, &result)
	}
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		reason = "scope_upgrade_failed"
	} else if result.Status == "expired" {
		reason = "scope_upgrade_expired"
	}
	c.op.Lock()
	defer c.op.Unlock()
	c.mu.Lock()
	current := c.rpc == r && reflect.DeepEqual(selectedOperatorScopes(c.saved), scopes)
	c.mu.Unlock()
	if !current || ctx.Err() != nil {
		return
	}
	if err == nil && result.Status == "approved" && result.DeviceToken != "" {
		s := c.saved
		s.Token = result.DeviceToken
		if c.persist(s) == nil {
			c.mu.Lock()
			c.operatorUpgradeRetry = false
			c.mu.Unlock()
			// A new signed handshake must still demonstrate the requested authority.
			r.conn.Close()
			return
		}
		reason = "storage"
	}
	c.mu.Lock()
	c.upgradeTerminal = reason
	c.mu.Unlock()
	r.conn.Close()
}

// Gateway 2026.11.4 knows ten scopes but requestUpgrade validates at most eight.
// Its verified roleScopesAllow rules cover session read/write through write or
// admin (session read also through read). Omit only those already implied scopes
// from the approval request; NEVER alter the selected/signed handshake scope set.
func upgradeRequestScopes(scopes []string) []string {
	out := append([]string{}, scopes...)
	has := func(scope string) bool {
		for _, s := range scopes {
			if s == scope {
				return true
			}
		}
		return false
	}
	for _, redundant := range []string{"operator.sessions.read", "operator.sessions.write"} {
		if len(out) <= 8 {
			break
		}
		covered := has("operator.admin") || has("operator.write") || (redundant == "operator.sessions.read" && has("operator.read"))
		if !covered {
			continue
		}
		next := []string{}
		for _, s := range out {
			if s != redundant {
				next = append(next, s)
			}
		}
		out = next
	}
	return out
}
