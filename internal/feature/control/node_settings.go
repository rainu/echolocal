//go:build !board_biscuit

package control

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/ygelfand/echolocal/internal/component"
	connection "github.com/ygelfand/echolocal/internal/openclaw"
	"github.com/ygelfand/libcountertop/pkg/media/sponsorblock"
	setting "github.com/ygelfand/libcountertop/pkg/settings"
	"sort"
	"strconv"
	"strings"
)

const nodeSettingsDiscoveryHint = "use settings.get with domain=\"\" and name=\"\" to discover safe identifiers"

func nodeSettingGroup(domain string) string { return nodeSettingsPolicies[domain].Group }
func nodeReviewedSetting(domain, name string) bool {
	p, ok := nodeSettingsPolicies[domain]
	if !ok {
		return false
	}
	for _, n := range strings.Fields(p.Names) {
		if n == name {
			return true
		}
	}
	return false
}

// No Fields() calls here: that UI interface eagerly reads unrelated values.
func nodeSetting(a *connection.NodeAccess, domain, name string) (setting.Bound, setting.Field, error) {
	if !a.Granted("settings") {
		return nil, setting.Field{}, errors.New("permission_denied")
	}
	if _, excluded := nodeSettingsExclusions[domain+"/"+name]; excluded {
		return nil, setting.Field{}, errors.New("setting_excluded: local administration only")
	}
	if _, ok := nodeSettingsPolicies[domain]; !ok {
		return nil, setting.Field{}, errors.New("unsupported_domain: " + nodeSettingsDiscoveryHint)
	}
	if !nodeReviewedSetting(domain, name) {
		if _, ok := nodeSettingsExclusions[domain+"/"+name]; ok {
			return nil, setting.Field{}, errors.New("setting_excluded: local administration only")
		}
		return nil, setting.Field{}, errors.New("unknown_setting: " + nodeSettingsDiscoveryHint)
	}
	if !a.Granted(nodeSettingGroup(domain)) {
		return nil, setting.Field{}, errors.New("permission_denied")
	}
	if !NodeSettingAvailable(domain, name) {
		return nil, setting.Field{}, errors.New("setting_unavailable: hardware does not provide this setting")
	}
	for _, b := range component.Settings.All() {
		if b.Domain() != domain {
			continue
		}
		d, ok := b.(setting.Inspectable)
		if !ok {
			continue
		}
		// A domain may have several independent bound tables.
		for _, n := range b.Names() {
			if n != name {
				continue
			}
			f, ok := d.Metadata(name)
			if !ok {
				continue
			}
			if !nodeSupportedKind(f.Kind) {
				return nil, setting.Field{}, errors.New("setting_excluded: unsupported field type")
			}
			return b, f, nil
		}
	}
	return nil, setting.Field{}, errors.New("setting_unavailable: not registered on this hardware or build")
}
func nodeSupportedKind(k string) bool {
	switch k {
	case "toggle", "number", "choice", "text", "list", "pair", "triple":
		return true
	}
	return false
}
func nodeSettingValue(b setting.Bound, name string) (string, error) {
	v, err := b.(setting.Inspectable).RawValue(name)
	if err != nil {
		return "", errors.New("operation_failed")
	}
	return v, nil
}
func nodeValidateSetting(b setting.Bound, f setting.Field, value string) error {
	if len(value) > 4096 || strings.ContainsRune(value, 0) {
		return errors.New("invalid_value: maximum 4096 bytes; NUL is not allowed")
	}
	switch f.Kind {
	case "toggle":
		if value != "on" && value != "off" {
			return errors.New("invalid_value: toggle requires on or off")
		}
	case "number":
		n, err := strconv.Atoi(value)
		if err != nil || ((f.Min != 0 || f.Max != 0) && (n < f.Min || n > f.Max)) {
			return errors.New("invalid_value: use a decimal integer in the discovered range")
		}
	case "choice":
		if len(f.Choices) == 0 {
			return errors.New("setting_unavailable: no choices currently configured")
		}
		if len(f.Choices) > 0 {
			ok := false
			for _, c := range f.Choices {
				if c.Value == value {
					ok = true
					break
				}
			}
			if !ok {
				return errors.New("invalid_value: use a discovered choice value, not its label")
			}
		}
	}
	if b.Domain() == "cast" && f.Name == "youtube.skip" && value != "" {
		for _, token := range strings.Split(value, ",") {
			ok := false
			for _, category := range sponsorblock.Categories {
				if strings.TrimSpace(token) == category {
					ok = true
					break
				}
			}
			if !ok {
				return errors.New("invalid_value: use discovered category entries")
			}
		}
	}
	if b.(setting.Inspectable).Validate(f.Name, value) != nil {
		return errors.New("invalid_value: rejected by the setting parser")
	}
	return nil
}

type nodeSettingInfo struct {
	Domain         string   `json:"domain"`
	Name           string   `json:"name"`
	Kind           string   `json:"kind"`
	Example        string   `json:"example,omitempty"`
	Unit           string   `json:"unit,omitempty"`
	Format         string   `json:"format"`
	Min            *int     `json:"min,omitempty"`
	Max            *int     `json:"max,omitempty"`
	Readable       bool     `json:"readable"`
	Writable       bool     `json:"writable"`
	Entries        []string `json:"entries,omitempty"`
	Choices        []string `json:"choices,omitempty"`
	RequiredGrants []string `json:"requiredGrants"`
}

func nodeSettingDescription(domain string, f setting.Field) nodeSettingInfo {
	v := nodeSettingInfo{Domain: domain, Name: f.Name, Kind: f.Kind, Example: f.Example, Unit: f.Unit, Readable: true, Writable: true, RequiredGrants: []string{"settings"}}
	if g := nodeSettingGroup(domain); g != "settings" {
		v.RequiredGrants = append(v.RequiredGrants, g)
	}

	switch f.Kind {
	case "toggle":
		v.Format = "on | off (JSON string)"
	case "number":
		if f.Min != 0 || f.Max != 0 {
			v.Min = &f.Min
			v.Max = &f.Max
		}
		v.Format = "decimal integer (JSON string)"
	case "choice":
		v.Writable = len(f.Choices) > 0
		v.Format = "exact choice value (JSON string)"
		for _, c := range f.Choices {
			v.Choices = append(v.Choices, c.Value)
		}
	case "text", "list":
		v.Format = "setting-specific text (JSON string), at most 4096 bytes"
	case "pair":
		v.Format = "setting-specific pair (JSON string)"
	case "triple":
		v.Format = "setting-specific triple (JSON string)"
	}
	if domain == "cast" && f.Name == "youtube.skip" {
		v.Format = "comma-separated category entries; empty clears"
		v.Entries = append([]string{}, sponsorblock.Categories...)
	}
	if domain == "apps" {
		v.Format = "comma-separated application keys; surrounding whitespace trimmed; empty clears"
	}
	if domain == "home" && strings.HasPrefix(f.Name, "rings.hidden_") || domain == "home" && f.Name == "rings.labels" {
		v.Format = "comma-separated IDs/kinds/labels; surrounding whitespace trimmed, duplicates removed; empty clears"
	}
	if domain == "visual" && f.Name == "label" || domain == "doorbell" && strings.HasSuffix(f.Name, ".name") {
		v.Format = "display label text; table may trim/truncate, so read back after writing"
	}
	if domain == "clock" && f.Name == "date_pattern" {
		v.Format = "Unicode date pattern, e.g. dd.MM.yyyy"
	}
	if domain == "screen" && strings.HasPrefix(f.Name, "custom.") {
		v.Format = "theme color string, e.g. #112233"
	}
	return v
}
func nodeDiscoverSettings(a *connection.NodeAccess, domain string) []nodeSettingInfo {
	out := []nodeSettingInfo{}
	for _, b := range component.Settings.All() {
		if domain != "" && b.Domain() != domain {
			continue
		}
		for _, name := range b.Names() {
			if !nodeReviewedSetting(b.Domain(), name) {
				continue
			}
			_, f, err := nodeSetting(a, b.Domain(), name)
			if err != nil {
				continue
			}
			out = append(out, nodeSettingDescription(b.Domain(), f))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Domain != out[j].Domain {
			return out[i].Domain < out[j].Domain
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// The root index intentionally excludes choices/examples/ranges. A complete
// all-field metadata response exceeds the agent tool result budget on real
// devices. Select a domain for full details without guessing bound names.
func nodeSettingsDiscovery(a *connection.NodeAccess, domain string) map[string]any {
	const availability = "Only currently registered and permitted safe fields; no values or excluded fields."
	fields := nodeDiscoverSettings(a, domain)
	if domain != "" {
		return map[string]any{"settings": fields, "availability": availability}
	}
	type domainInfo struct {
		Domain         string   `json:"domain"`
		Names          []string `json:"names"`
		RequiredGrants []string `json:"requiredGrants"`
	}
	domains := []domainInfo{}
	for _, f := range fields {
		if len(domains) == 0 || domains[len(domains)-1].Domain != f.Domain {
			domains = append(domains, domainInfo{Domain: f.Domain, Names: []string{}, RequiredGrants: append([]string{}, f.RequiredGrants...)})
		}
		domains[len(domains)-1].Names = append(domains[len(domains)-1].Names, f.Name)
	}
	return map[string]any{"domains": domains, "availability": availability, "details": "Repeat settings.get with a selected domain and name=\"\" for kinds, formats, ranges, canonical choices, write availability and exact grants."}
}
func registerNodeSettings(a *connection.NodeAccess) {
	add := func(name string, run func(context.Context, json.RawMessage) (any, error)) {
		_ = nodeRegister(a, connection.NodeOperation{Command: name, Group: "settings", Run: run})
	}
	add("settings.discover", func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p struct {
			Domain string `json:"domain"`
		}
		if decodeNode(raw, &p) != nil {
			return nil, errors.New("invalid_params")
		}
		if p.Domain != "" {
			if _, ok := nodeSettingsPolicies[p.Domain]; !ok {
				return nil, errors.New("unsupported_domain: " + nodeSettingsDiscoveryHint)
			}
		}
		return nodeSettingsDiscovery(a, p.Domain), nil
	})
	add("settings.get", func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p struct {
			Domain string `json:"domain"`
			Name   string `json:"name"`
		}
		if decodeNode(raw, &p) != nil {
			return nil, errors.New("invalid_params")
		}
		if p.Name == "" {
			if p.Domain != "" {
				if _, ok := nodeSettingsPolicies[p.Domain]; !ok {
					return nil, errors.New("unsupported_domain: " + nodeSettingsDiscoveryHint)
				}
			}
			return nodeSettingsDiscovery(a, p.Domain), nil
		}
		b, f, err := nodeSetting(a, p.Domain, p.Name)
		if err != nil {
			return nil, err
		}
		value, err := nodeSettingValue(b, p.Name)
		if err != nil {
			return nil, err
		}
		return map[string]any{"domain": p.Domain, "name": p.Name, "value": value, "kind": f.Kind}, nil
	})
	add("settings.set", func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p struct {
			Domain string  `json:"domain"`
			Name   string  `json:"name"`
			Value  *string `json:"value"`
		}
		if decodeNode(raw, &p) != nil || p.Value == nil {
			return nil, errors.New("invalid_params")
		}
		b, f, err := nodeSetting(a, p.Domain, p.Name)
		if err != nil {
			return nil, err
		}
		if err = nodeValidateSetting(b, f, *p.Value); err != nil {
			return nil, err
		}
		if ctx.Err() != nil {
			return nil, errors.New("cancelled")
		}
		if _, _, err = nodeSetting(a, p.Domain, p.Name); err != nil {
			return nil, err
		}
		if b.(setting.Inspectable).ChangeRaw(p.Name, *p.Value) != nil {
			return nil, errors.New("operation_failed")
		}
		return map[string]bool{"done": true}, nil
	})
	add("settings.export", func(ctx context.Context, raw json.RawMessage) (any, error) {
		if nodeEmpty(raw) != nil {
			return nil, errors.New("invalid_params")
		}
		out := map[string]map[string]string{}
		for _, s := range nodeDiscoverSettings(a, "") {
			b, _, err := nodeSetting(a, s.Domain, s.Name)
			if err != nil {
				return nil, err
			}
			v, err := nodeSettingValue(b, s.Name)
			if err != nil {
				return nil, err
			}
			if out[s.Domain] == nil {
				out[s.Domain] = map[string]string{}
			}
			out[s.Domain][s.Name] = v
		}
		return out, nil
	})
	add("settings.import", func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p struct {
			Values map[string]map[string]string `json:"values"`
		}
		if decodeNode(raw, &p) != nil || len(p.Values) == 0 {
			return nil, errors.New("invalid_params")
		}
		type change struct {
			domain, name, value string
			b                   setting.Bound
		}
		changes := []change{}
		for domain, fields := range p.Values {
			if len(fields) == 0 {
				return nil, errors.New("invalid_params")
			}
			for name, value := range fields {
				b, f, err := nodeSetting(a, domain, name)
				if err != nil {
					return nil, err
				}
				if err := nodeValidateSetting(b, f, value); err != nil {
					return nil, err
				}
				changes = append(changes, change{domain, name, value, b})
			}
		}
		// Preflight every parser before the first write; saving is not transactional.
		sort.Slice(changes, func(i, j int) bool {
			return changes[i].domain+"/"+changes[i].name < changes[j].domain+"/"+changes[j].name
		})
		for _, c := range changes {
			if ctx.Err() != nil {
				return nil, errors.New("cancelled")
			}
			if _, _, err := nodeSetting(a, c.domain, c.name); err != nil {
				return nil, err
			}
			if c.b.(setting.Inspectable).ChangeRaw(c.name, c.value) != nil {
				return nil, errors.New("operation_failed: import may be partially applied")
			}
		}
		return map[string]int{"applied": len(changes)}, nil
	})
}
