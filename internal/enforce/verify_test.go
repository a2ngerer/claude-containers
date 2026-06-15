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

// buildMaterializedDir writes a minimal materialized config dir that matches rm:
// the allowlisted skills/subagents plus a settings.json carrying the expected
// deny rules.
func buildMaterializedDir(t *testing.T, rm compose.ResolvedManifest) string {
	t.Helper()
	dir := t.TempDir()
	for _, s := range rm.Skills {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "skills", s), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "skills", s, "SKILL.md"), []byte("x"), 0o644))
	}
	for _, a := range rm.Subagents {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "agents"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "agents", a+".md"), []byte("x"), 0o644))
	}
	ps := BuildPermissions(rm.Enforcement)
	sf := map[string]any{
		"permissions":    map[string]any{"allow": ps.Allow, "deny": ps.Deny},
		"permissionMode": ps.Mode,
	}
	data, err := json.MarshalIndent(sf, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "settings.json"), data, 0o644))
	return dir
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
	dir := buildMaterializedDir(t, rm)

	att, err := Verify(rm, dir)
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
	dir := buildMaterializedDir(t, rm)
	// Smuggle a build skill into the materialized dir.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "skills", "writing-plans"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "skills", "writing-plans", "SKILL.md"), []byte("x"), 0o644))

	att, err := Verify(rm, dir)
	require.ErrorIs(t, err, domain.ErrVerifyMismatch)
	require.False(t, att.Clean)
}

func TestVerify_MissingDenyMismatch(t *testing.T) {
	rm := reviewerRM()
	dir := buildMaterializedDir(t, rm)
	// Rewrite settings.json without the mandatory Write deny.
	sf := map[string]any{
		"permissions":    map[string]any{"allow": []string{"Read"}, "deny": []string{"Edit", "NotebookEdit"}},
		"permissionMode": "read-only",
	}
	data, _ := json.MarshalIndent(sf, "", "  ")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "settings.json"), data, 0o644))

	att, err := Verify(rm, dir)
	require.True(t, errors.Is(err, domain.ErrVerifyMismatch))
	require.False(t, att.Clean)
}
