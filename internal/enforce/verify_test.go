package enforce

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/angerer/claude_git/internal/compose"
	"github.com/angerer/claude_git/internal/domain"
	"github.com/stretchr/testify/require"
)

// buildMaterializedDir writes a persona source tree and a config dir that
// mirrors exactly what materialize.Materialize would produce for rm: the
// allowlisted skills/subagents, a CLAUDE.md, a settings.json with the expected
// allow/deny/permissionMode, and an mcp.json iff the persona is MCP-isolated.
// Verify must accept this clean pair and reject anything that deviates from it.
// Keeping this helper in lockstep with Materialize is the Materialize<->Verify
// consistency contract under test. It returns (personaDir, destDir).
func buildMaterializedDir(t *testing.T, rm compose.ResolvedManifest) (string, string) {
	t.Helper()
	root := t.TempDir()
	personaDir := filepath.Join(root, "persona")
	dir := filepath.Join(root, "cfg")
	require.NoError(t, os.MkdirAll(personaDir, 0o755))
	require.NoError(t, os.MkdirAll(dir, 0o755))

	for _, s := range rm.Skills {
		// source skill tree (reference for the whitelist)
		require.NoError(t, os.MkdirAll(filepath.Join(personaDir, "skills", s), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(personaDir, "skills", s, "SKILL.md"), []byte("x"), 0o644))
		// materialized copy
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "skills", s), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "skills", s, "SKILL.md"), []byte("x"), 0o644))
	}
	for _, a := range rm.Subagents {
		require.NoError(t, os.MkdirAll(filepath.Join(personaDir, "agents"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(personaDir, "agents", a+".md"), []byte("x"), 0o644))
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "agents"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "agents", a+".md"), []byte("x"), 0o644))
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(rm.ClaudeMD), 0o644))

	ps := BuildPermissions(rm.Enforcement)
	sf := map[string]any{
		"permissions":    map[string]any{"allow": ps.Allow, "deny": ps.Deny},
		"permissionMode": ps.Mode,
	}
	data, err := json.MarshalIndent(sf, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "settings.json"), data, 0o644))

	if rm.MCP.Config != "" || domain.MCPIsolated(rm.Enforcement, rm.MCP) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "mcp.json"), []byte(`{"mcpServers":{}}`), 0o644))
	}
	return personaDir, dir
}

func reviewerRM() compose.ResolvedManifest {
	return compose.ResolvedManifest{
		Persona:    domain.Persona{Name: "reviewer", Metadata: domain.Metadata{Version: "1.2.0"}},
		Skills:     []string{"security-review"},
		Subagents:  []string{"code-reviewer"},
		SettingSrc: []string{"user", "project"},
		Enforcement: domain.Enforcement{
			PermissionMode: "read-only",
			ToolsAllow:     []string{"Read", "Grep"},
			ToolsDeny:      []string{"Bash(git commit:*)"},
		},
	}
}

func TestVerify_Clean(t *testing.T) {
	rm := reviewerRM()
	personaDir, dir := buildMaterializedDir(t, rm)

	att, err := Verify(rm, personaDir, dir)
	require.NoError(t, err)
	require.True(t, att.Clean)
	require.Equal(t, "reviewer", att.Persona)
	require.Equal(t, "1.2.0", att.Version)
	require.Equal(t, []string{"user", "project"}, att.SettingSrc)
	require.Contains(t, att.Denied, "Write")
	require.Contains(t, att.Denied, "Bash(git commit:*)")

	// Included carries the skill + subagent names.
	var gotSkills, gotSubagents []string
	for _, line := range att.Included {
		switch line.Kind {
		case "skill":
			gotSkills = line.Names
		case "subagent":
			gotSubagents = line.Names
		}
	}
	require.Equal(t, []string{"security-review"}, gotSkills)
	require.Equal(t, []string{"code-reviewer"}, gotSubagents)
}

func TestVerify_SmuggledSkillMismatch(t *testing.T) {
	rm := reviewerRM()
	personaDir, dir := buildMaterializedDir(t, rm)
	// Smuggle a build skill into the materialized dir.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "skills", "writing-plans"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "skills", "writing-plans", "SKILL.md"), []byte("x"), 0o644))

	att, err := Verify(rm, personaDir, dir)
	require.ErrorIs(t, err, domain.ErrVerifyMismatch)
	require.False(t, att.Clean)
}

func TestVerify_MissingDenyMismatch(t *testing.T) {
	rm := reviewerRM()
	personaDir, dir := buildMaterializedDir(t, rm)
	// Rewrite settings.json without the mandatory Write deny.
	sf := map[string]any{
		"permissions":    map[string]any{"allow": []string{"Read", "Grep"}, "deny": []string{"Edit", "NotebookEdit"}},
		"permissionMode": "read-only",
	}
	data, _ := json.MarshalIndent(sf, "", "  ")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "settings.json"), data, 0o644))

	att, err := Verify(rm, personaDir, dir)
	require.True(t, errors.Is(err, domain.ErrVerifyMismatch))
	require.False(t, att.Clean)
}

// rewriteSettings overwrites settings.json with the given allow/deny/mode.
func rewriteSettings(t *testing.T, dir string, allow, deny []string, mode string) {
	t.Helper()
	sf := map[string]any{
		"permissions":    map[string]any{"allow": allow, "deny": deny},
		"permissionMode": mode,
	}
	data, err := json.MarshalIndent(sf, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "settings.json"), data, 0o644))
}

// C3 — Verify must whitelist the destDir: any file or directory that
// Materialize would not have written is a mismatch. The following cases each
// smuggle one extra artefact past the old (blind) checks.

func TestVerify_SmuggledSettingsLocalMismatch(t *testing.T) {
	rm := reviewerRM()
	personaDir, dir := buildMaterializedDir(t, rm)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "settings.local.json"), []byte("{}"), 0o644))

	att, err := Verify(rm, personaDir, dir)
	require.ErrorIs(t, err, domain.ErrVerifyMismatch)
	require.False(t, att.Clean)
}

func TestVerify_SmuggledHookMismatch(t *testing.T) {
	rm := reviewerRM()
	personaDir, dir := buildMaterializedDir(t, rm)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "hooks"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "hooks", "x.sh"), []byte("#!/bin/sh\n"), 0o755))

	att, err := Verify(rm, personaDir, dir)
	require.ErrorIs(t, err, domain.ErrVerifyMismatch)
	require.False(t, att.Clean)
}

func TestVerify_SmuggledCommandMismatch(t *testing.T) {
	rm := reviewerRM()
	personaDir, dir := buildMaterializedDir(t, rm)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "commands"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "commands", "x.md"), []byte("x"), 0o644))

	att, err := Verify(rm, personaDir, dir)
	require.ErrorIs(t, err, domain.ErrVerifyMismatch)
	require.False(t, att.Clean)
}

func TestVerify_SmuggledDotMCPMismatch(t *testing.T) {
	rm := reviewerRM()
	personaDir, dir := buildMaterializedDir(t, rm)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte(`{"mcpServers":{"x":{}}}`), 0o644))

	att, err := Verify(rm, personaDir, dir)
	require.ErrorIs(t, err, domain.ErrVerifyMismatch)
	require.False(t, att.Clean)
}

func TestVerify_UnexpectedMCPWhenNoneMismatch(t *testing.T) {
	rm := reviewerRM()
	rm.Enforcement.PermissionMode = "default" // not isolated -> no mcp.json expected
	rm.MCP = domain.MCPConfig{Config: "", Strict: false}
	personaDir, dir := buildMaterializedDir(t, rm)
	// Smuggle an mcp.json although none is expected.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "mcp.json"), []byte(`{"mcpServers":{}}`), 0o644))

	att, err := Verify(rm, personaDir, dir)
	require.ErrorIs(t, err, domain.ErrVerifyMismatch)
	require.False(t, att.Clean)
}

func TestVerify_InjectedFileInAllowedSkillMismatch(t *testing.T) {
	rm := reviewerRM()
	personaDir, dir := buildMaterializedDir(t, rm)
	// An extra file inside an allowed skill tree must be caught (recursive whitelist).
	require.NoError(t, os.WriteFile(filepath.Join(dir, "skills", "security-review", "INJECTED.md"), []byte("evil"), 0o644))

	att, err := Verify(rm, personaDir, dir)
	require.ErrorIs(t, err, domain.ErrVerifyMismatch)
	require.False(t, att.Clean)
}

func TestVerify_NestedAgentMismatch(t *testing.T) {
	rm := reviewerRM()
	personaDir, dir := buildMaterializedDir(t, rm)
	// A nested file under agents/ that is not an allowed top-level <name>.md.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "agents", "nested"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "agents", "nested", "evil.md"), []byte("x"), 0o644))

	att, err := Verify(rm, personaDir, dir)
	require.ErrorIs(t, err, domain.ErrVerifyMismatch)
	require.False(t, att.Clean)
}

// I1 — permissionMode in settings.json must equal the expected mode. Flipping a
// read-only persona to "default" is a mismatch even if deny rules survive.
func TestVerify_PermissionModeTamperedMismatch(t *testing.T) {
	rm := reviewerRM()
	personaDir, dir := buildMaterializedDir(t, rm)
	ps := BuildPermissions(rm.Enforcement)
	rewriteSettings(t, dir, ps.Allow, ps.Deny, "default") // was read-only

	att, err := Verify(rm, personaDir, dir)
	require.ErrorIs(t, err, domain.ErrVerifyMismatch)
	require.False(t, att.Clean)
}

// I2 — permissions.allow must equal the expected allow set exactly. An extra
// allowed tool (here Bash) is a mismatch.
func TestVerify_AllowListTamperedMismatch(t *testing.T) {
	rm := reviewerRM()
	personaDir, dir := buildMaterializedDir(t, rm)
	ps := BuildPermissions(rm.Enforcement)
	rewriteSettings(t, dir, append(append([]string{}, ps.Allow...), "Bash"), ps.Deny, ps.Mode)

	att, err := Verify(rm, personaDir, dir)
	require.ErrorIs(t, err, domain.ErrVerifyMismatch)
	require.False(t, att.Clean)
}

// I3 — an isolated persona whose mcp.json is missing on disk is a mismatch
// (the launch would pass --mcp-config to a nonexistent file).
func TestVerify_MissingExpectedMCPMismatch(t *testing.T) {
	rm := reviewerRM() // read-only -> isolated -> mcp.json expected
	personaDir, dir := buildMaterializedDir(t, rm)
	require.NoError(t, os.Remove(filepath.Join(dir, "mcp.json")))

	att, err := Verify(rm, personaDir, dir)
	require.ErrorIs(t, err, domain.ErrVerifyMismatch)
	require.False(t, att.Clean)
}

// I4 — an isolated persona must declare its SettingSrc; an empty SettingSrc is
// a mismatch (it would let every settings source leak in at launch).
func TestVerify_EmptySettingSrcMismatch(t *testing.T) {
	rm := reviewerRM()
	rm.SettingSrc = nil
	personaDir, dir := buildMaterializedDir(t, rm)

	att, err := Verify(rm, personaDir, dir)
	require.ErrorIs(t, err, domain.ErrVerifyMismatch)
	require.False(t, att.Clean)
}
