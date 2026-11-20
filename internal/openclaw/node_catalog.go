package openclaw

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
)

// NodeTool is metadata, never an execution grant. Command remains the wire name.
type NodeTool struct {
	PluginID    string         `json:"pluginId"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
	Command     string         `json:"command"`
}
type NodeSkill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content"`
}
type NodePublicationStatus struct {
	Tools     string `json:"tools"`
	Skill     string `json:"skill"`
	ToolCount int    `json:"toolCount"`
}

func (c *Client) NodePublication() NodePublicationStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.publication
}
func toolName(command string) string {
	name := strings.NewReplacer(".", "_", "-", "_").Replace(command)
	if !strings.HasPrefix(name, "echolocal_") {
		name = "echolocal_" + name
	}
	return name
}

var safeToolName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)

// Tools returns a detached, sorted snapshot of the currently granted registry.
func (a *NodeAccess) Tools() []NodeTool {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := []NodeTool{}
	for _, op := range a.operations {
		if !a.granted(op.Group) || op.Description == "" || op.Parameters == nil {
			continue
		}
		out = append(out, NodeTool{"echolocal", toolName(op.Command), op.Description, op.Parameters, op.Command})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Command < out[j].Command })
	raw, _ := json.Marshal(out)
	var snapshot []NodeTool
	_ = json.Unmarshal(raw, &snapshot)
	return snapshot
}
func (a *NodeAccess) changedLocked() {
	select {
	case a.changed <- struct{}{}:
	default:
	}
}

// Each authenticated socket owns one publisher. It never waits on the receive
// loop, retries a rejected method, changes policy, or reuses a replacement link.
func (c *Client) nodeCatalog(ctx context.Context, r *rpcLink) {
	if c.node == nil || c.access == nil {
		return
	}
	for {
		// Coalesce all edits before taking the snapshot.
		select {
		case <-c.access.changed:
		default:
		}
		tools := c.access.Tools()
		toolParams, oversized := boundedNodeToolPayload(tools)
		skill := NodeUsageSkill()
		for _, item := range []struct {
			method string
			params any
		}{
			{"node.pluginTools.update", toolParams},
			{"node.skills.update", map[string]any{"skills": []NodeSkill{skill}}},
		} {
			if ctx.Err() != nil {
				return
			}
			data, err := json.Marshal(item.params)
			status := "acknowledged"
			var reply struct {
				Tools  []NodeTool  `json:"tools"`
				Skills []NodeSkill `json:"skills"`
			}
			if err != nil || len(data) > 96*1024 {
				status = "payload_too_large"
			} else {
				callCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
				err = r.call(callCtx, item.method, item.params, &reply)
				cancel()
				if err != nil {
					status = "unavailable"
				} else if item.method == "node.pluginTools.update" && oversized {
					status = "payload_too_large"
				} else if item.method == "node.pluginTools.update" && len(reply.Tools) != len(tools) {
					status = "filtered"
				} else if item.method == "node.skills.update" && len(reply.Skills) != 1 {
					status = "filtered"
				}
			}
			c.mu.Lock()
			if c.rpc != r || ctx.Err() != nil {
				c.mu.Unlock()
				return
			}
			if item.method == "node.pluginTools.update" {
				c.publication.Tools = status
				c.publication.ToolCount = len(reply.Tools)
			} else {
				c.publication.Skill = status
			}
			c.mu.Unlock()
		}
		select {
		case <-ctx.Done():
			return
		case <-c.access.changed:
		}
	}
}

// Oversized replacements withdraw the prior catalog rather than leave an
// incomplete or stale live surface. Execution permission checks are unchanged.
func boundedNodeToolPayload(tools []NodeTool) (map[string]any, bool) {
	payload := map[string]any{"tools": tools}
	raw, err := json.Marshal(payload)
	if err != nil || len(raw) > 96*1024 || len(tools) > 128 {
		return map[string]any{"tools": []NodeTool{}}, true
	}
	return payload, false
}

// ParameterError contains only schema-owned field names and fixed diagnostics,
// never supplied keys, values, decoder messages or personal content.
type ParameterError struct{ Field, Expected string }

func (e *ParameterError) Error() string             { return "invalid_params: " + e.Field + ": " + e.Expected }
func invalidParameter(field, expected string) error { return &ParameterError{field, expected} }
func validateNodeParameters(raw json.RawMessage, schema map[string]any) error {
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return invalidParameter("$", "expected JSON object")
	}
	return validateNodeValue(value, schema, "$", 0)
}
func schemaNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	}
	return 0, false
}
func validateNodeValue(v any, s map[string]any, path string, depth int) error {
	if depth > 12 {
		return invalidParameter("$", "object nesting limit exceeded")
	}
	typ, _ := s["type"].(string)
	bad := func() error { return invalidParameter(path, "expected "+typ) }
	switch typ {
	case "object":
		obj, ok := v.(map[string]any)
		if !ok {
			return bad()
		}
		props, _ := s["properties"].(map[string]any)
		required, _ := s["required"].([]any)
		for _, key := range required {
			k, _ := key.(string)
			if _, ok := obj[k]; !ok {
				return invalidParameter(path+"."+k, "required field missing")
			}
		}
		// Unknown user-provided keys are deliberately not repeated.
		for k, value := range obj {
			sub, ok := props[k].(map[string]any)
			if !ok {
				if extra, ok := s["additionalProperties"].(map[string]any); ok {
					sub = extra
				} else if s["additionalProperties"] == false {
					keys := make([]string, 0, len(props))
					for key := range props {
						keys = append(keys, key)
					}
					sort.Strings(keys)
					expected := "unknown field; expected properties: " + strings.Join(keys, ", ")
					if len(expected) > 240 {
						expected = "unknown field; use only documented properties"
					}
					return invalidParameter(path, expected)
				} else {
					continue
				}
			}
			child := path + ".*"
			if _, ok := props[k]; ok {
				child = path + "." + k
			}
			if err := validateNodeValue(value, sub, child, depth+1); err != nil {
				return err
			}
		}
		if n, ok := schemaNumber(s["minProperties"]); ok && float64(len(obj)) < n {
			return invalidParameter(path, "object has too few fields")
		}
	case "array":
		arr, ok := v.([]any)
		if !ok {
			return bad()
		}
		if n, ok := schemaNumber(s["maxItems"]); ok && float64(len(arr)) > n {
			return invalidParameter(path, "too many items")
		}
		if n, ok := schemaNumber(s["minItems"]); ok && float64(len(arr)) < n {
			return invalidParameter(path, "too few items")
		}
		if sub, ok := s["items"].(map[string]any); ok {
			for _, value := range arr {
				if err := validateNodeValue(value, sub, path+"[]", depth+1); err != nil {
					return err
				}
			}
		}
	case "string":
		str, ok := v.(string)
		if !ok {
			return bad()
		}
		// Runtime adapters bound UTF-8 bytes, not Unicode code points.
		if n, ok := schemaNumber(s["x-maxBytes"]); ok && float64(len(str)) > n {
			return invalidParameter(path, "string exceeds byte limit")
		}
		if n, ok := schemaNumber(s["minLength"]); ok && float64(len([]rune(str))) < n {
			return invalidParameter(path, "string is empty")
		}
		if pattern, ok := s["pattern"].(string); ok {
			re, err := regexp.Compile(pattern)
			if err != nil || !re.MatchString(str) {
				return invalidParameter(path, "string does not match documented format")
			}
		}
	case "number", "integer":
		n, ok := v.(float64)
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) || (typ == "integer" && n != math.Trunc(n)) {
			return bad()
		}
		if min, ok := schemaNumber(s["minimum"]); ok && n < min {
			return invalidParameter(path, "below minimum")
		}
		if max, ok := schemaNumber(s["maximum"]); ok && n > max {
			return invalidParameter(path, "above maximum")
		}
	case "boolean":
		if _, ok := v.(bool); !ok {
			return bad()
		}
	default:
		return errors.New("schema_invalid")
	}
	if values, ok := s["enum"].([]any); ok {
		found := false
		for _, candidate := range values {
			a, _ := json.Marshal(v)
			b, _ := json.Marshal(candidate)
			if string(a) == string(b) {
				found = true
				break
			}
		}
		if !found {
			return invalidParameter(path, "expected a documented allowed value")
		}
	}
	return nil
}
