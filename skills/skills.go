// Package skills embeds the agent Skill that init places where coding agents
// read skills. The folder beside this file is the Skill itself, so people can
// also read it here or copy it by hand.
package skills

import "embed"

// Files holds the Skill under ponchimap/: SKILL.md and its references.
//
//go:embed ponchimap
var Files embed.FS
