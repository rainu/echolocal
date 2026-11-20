package openclaw

import _ "embed"

//go:embed skills/echolocal-device-tools/SKILL.md
var nodeUsageContent string

// NodeUsageSkill is the canonical shipped guide. Example contract tests validate
// its calls against the same metadata used for validation and publication.
func NodeUsageSkill() NodeSkill {
	return NodeSkill{
		Name:        "echolocal-device-tools",
		Description: "Use the current EchoLocal node tool catalog for device audio, display, alarms/timers, YouTube TV and Talk; discover permissions and parameter schemas before calling.",
		Content:     nodeUsageContent,
	}
}
