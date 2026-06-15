package cli

// personaScaffold is the file content a persona template renders into a new
// persona directory: a persona.toml body and a starter CLAUDE.md body.
type personaScaffold struct {
	TOML     string
	ClaudeMD string
}

const coderTOML = `name        = "coder"
description = "Full build persona. Curated skill set, Write/Edit allowed."
extends     = "_base"

[config]
claude_md       = "CLAUDE.md"
setting_sources = ["user", "project", "local"]

[config.skills]
mode    = "allowlist"
include = ["superpowers", "writing-plans", "executing-plans", "test", "debug"]

[config.subagents]
include = ["code-architect", "tdd-guide"]

[config.mcp]
config = "mcp.json"
strict = false

[enforcement]
permission_mode = "default"

[enforcement.tools]
allow = ["Read", "Grep", "Glob", "Write", "Edit", "Bash"]
deny  = []

[metadata]
version = "0.1.0"
author  = ""
`

const coderMD = `# coder

Full build environment. You may create and edit files, run the build, and use
the curated build skill set. Follow the project's TDD workflow.
`

const reviewerTOML = `name        = "reviewer"
description = "Uncontaminated reviewer. Sees the diff, not the build skills."
extends     = "_base"

[config]
claude_md       = "CLAUDE.md"
setting_sources = ["user", "project"]

[config.skills]
mode    = "replace"
include = ["security-review", "silent-failure-hunter", "type-design-analyzer"]

[config.subagents]
include = ["code-reviewer", "security-reviewer"]

[config.mcp]
config = "mcp.json"
strict = true

[enforcement]
permission_mode = "read-only"

[enforcement.tools]
allow = ["Read", "Grep", "Glob", "Bash(git diff:*)", "Bash(git log:*)"]
deny  = ["Write", "Edit", "NotebookEdit", "Bash(git commit:*)", "Bash(git push:*)"]

[metadata]
version = "0.1.0"
author  = ""
`

const reviewerMD = `# reviewer

Read-only review environment. You cannot write or edit files. Review the diff
on its own merits; you do not have the builder's skills and must not assume its
intent.
`

// personaTemplate returns the embedded scaffold for a known template name.
func personaTemplate(name string) (personaScaffold, bool) {
	switch name {
	case "coder":
		return personaScaffold{TOML: coderTOML, ClaudeMD: coderMD}, true
	case "reviewer":
		return personaScaffold{TOML: reviewerTOML, ClaudeMD: reviewerMD}, true
	default:
		return personaScaffold{}, false
	}
}
