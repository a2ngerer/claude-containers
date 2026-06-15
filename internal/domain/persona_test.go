package domain

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const reviewerTOML = `name = "reviewer"
description = "Uncontaminated reviewer."
extends = "_base"

[config]
claude_md = "CLAUDE.md"
setting_sources = ["user", "project"]

[config.skills]
mode = "allowlist"
include = ["security-review", "silent-failure-hunter"]

[config.subagents]
include = ["code-reviewer", "security-reviewer"]

[config.mcp]
config = "mcp.json"
strict = true
requires = ["github"]

[enforcement]
permission_mode = "read-only"

[enforcement.tools]
allow = ["Read", "Grep", "Bash(git diff:*)"]
deny = ["Write", "Edit", "Bash(git commit:*)"]

[metadata]
version = "1.2.0"
author = "alexander.angerer"
`

func TestIsLayer(t *testing.T) {
	require.True(t, Persona{Name: "_base"}.IsLayer())
	require.False(t, Persona{Name: "reviewer"}.IsLayer())
	require.False(t, Persona{Name: ""}.IsLayer())
}

func TestLoadPersonaTOML_MapsEnforcementTools(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "persona.toml")
	require.NoError(t, os.WriteFile(path, []byte(reviewerTOML), 0o644))

	p, err := LoadPersonaTOML(path)
	require.NoError(t, err)

	require.Equal(t, "reviewer", p.Name)
	require.Equal(t, "_base", p.Extends)
	require.Equal(t, "CLAUDE.md", p.Config.ClaudeMD)
	require.Equal(t, []string{"user", "project"}, p.Config.SettingSources)
	require.Equal(t, "allowlist", p.Config.Skills.Mode)
	require.Equal(t, []string{"security-review", "silent-failure-hunter"}, p.Config.Skills.Include)
	require.Equal(t, []string{"code-reviewer", "security-reviewer"}, p.Config.Subagents.Include)
	require.Equal(t, "mcp.json", p.Config.MCP.Config)
	require.True(t, p.Config.MCP.Strict)
	require.Equal(t, []string{"github"}, p.Config.MCP.Requires)
	require.Equal(t, "read-only", p.Enforcement.PermissionMode)
	// the critical mapping: [enforcement.tools] allow/deny -> ToolsAllow/ToolsDeny
	require.Equal(t, []string{"Read", "Grep", "Bash(git diff:*)"}, p.Enforcement.ToolsAllow)
	require.Equal(t, []string{"Write", "Edit", "Bash(git commit:*)"}, p.Enforcement.ToolsDeny)
	require.Equal(t, "1.2.0", p.Metadata.Version)
	require.Equal(t, "alexander.angerer", p.Metadata.Author)
}

func TestSavePersonaTOML_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "persona.toml")
	require.NoError(t, os.WriteFile(path, []byte(reviewerTOML), 0o644))

	orig, err := LoadPersonaTOML(path)
	require.NoError(t, err)

	out := filepath.Join(dir, "out.toml")
	require.NoError(t, SavePersonaTOML(orig, out))

	again, err := LoadPersonaTOML(out)
	require.NoError(t, err)

	require.Equal(t, orig, again)
	require.Equal(t, []string{"Read", "Grep", "Bash(git diff:*)"}, again.Enforcement.ToolsAllow)
	require.Equal(t, []string{"Write", "Edit", "Bash(git commit:*)"}, again.Enforcement.ToolsDeny)
}
