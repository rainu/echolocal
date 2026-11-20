package openclaw

import (
	_ "embed"
	"strings"
)

//go:embed skills/echolocal-device-tools/SKILL.md
var nodeUsageContent string

// NodeUsageSkill reads its descriptor from the canonical embedded frontmatter.
// The Gateway requires exact name/description agreement, not just valid fields.
func NodeUsageSkill() NodeSkill {
	fields := map[string]string{}
	for _, line := range strings.Split(strings.SplitN(nodeUsageContent, "---", 3)[1], "\n") {
		key, value, ok := strings.Cut(line, ": ")
		if ok {
			fields[key] = strings.TrimSpace(value)
		}
	}
	return NodeSkill{Name: fields["name"], Description: fields["description"], Content: nodeUsageContent}
}
