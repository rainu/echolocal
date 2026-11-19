package openclaw

import "errors"

// SupportedOperatorScopes is the closed Gateway protocol scope set (2026.11.4).
// Return a copy so UI callers cannot change the handshake defaults.
func SupportedOperatorScopes() []string {
	return []string{"operator.admin", "operator.read", "operator.write", "operator.sessions.read", "operator.sessions.write", "operator.approvals", "operator.questions", "operator.pairing", "operator.talk", "operator.talk.secrets"}
}
func selectedOperatorScopes(s credentials) []string {
	if s.OperatorScopes == nil {
		return SupportedOperatorScopes()
	}
	return append([]string{}, (*s.OperatorScopes)...)
}
func (c *Client) RequestedOperatorScopes() []string {
	if c.node != nil {
		return []string{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return selectedOperatorScopes(c.saved)
}

// GrantedOperatorScopes reports only the current authenticated connection,
// never cached device-token authority or the user's requested selection.
func (c *Client) GrantedOperatorScopes() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string{}, c.granted...)
}

// SetOperatorScopes disconnects first, retains identity and credentials, saves
// exactly the supported selection and reauthenticates if previously enabled.
// The Gateway alone approves upgrades; stored grants may need revoking there.
func (c *Client) SetOperatorScopes(scopes []string) error {
	if c.node != nil {
		return errors.New("operator_only")
	}
	known := map[string]bool{}
	for _, v := range SupportedOperatorScopes() {
		known[v] = true
	}
	chosen := map[string]bool{}
	for _, v := range scopes {
		if !known[v] || chosen[v] {
			return errors.New("invalid_scopes")
		}
		chosen[v] = true
	}
	next := []string{}
	for _, v := range SupportedOperatorScopes() {
		if chosen[v] {
			next = append(next, v)
		}
	}
	c.op.Lock()
	defer c.op.Unlock()
	c.stopLocked()
	s := c.saved
	s.OperatorScopes = &next
	if err := c.persist(s); err != nil {
		return err
	}
	c.mark("disconnected", "permissions_changed", "local", "permissions")
	if s.Enabled {
		c.startLocked()
	}
	return nil
}
