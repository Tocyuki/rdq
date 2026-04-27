// Package skills exposes the embedded Agent Skills shipped with the rdq
// binary. The embed.FS is consumed by skillsmith.New (see cmd/rdq/skills.go)
// so the binary can list, install, update, and uninstall skills under
// ~/.agents/skills/ without any external assets.
//
// Layout:
//
//	skills/
//	├── embed.go        <- this file; //go:embed lives here
//	└── rdq-exec/
//	    ├── SKILL.md    <- required by agentskills.io spec
//	    └── references/...
//
// To add a new skill, create a sibling directory (e.g. rdq-ask/) and append
// it to the //go:embed directive below. Skill directory names must match the
// `name` field in their SKILL.md frontmatter (agentskills.io requirement).
package skills

import "embed"

//go:embed all:rdq-exec
var FS embed.FS
