package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/angerer/claude_git/internal/domain"
	"github.com/angerer/claude_git/internal/environment"
	"github.com/stretchr/testify/require"
)

func TestPersonaTemplate_CoderAndReviewer(t *testing.T) {
	coder, ok := personaTemplate("coder")
	require.True(t, ok)
	require.Contains(t, coder.TOML, `name        = "coder"`)
	require.Contains(t, coder.TOML, `setting_sources = ["user", "project", "local"]`)
	require.Contains(t, coder.TOML, `"Write"`)
	require.NotEmpty(t, coder.ClaudeMD)

	rev, ok := personaTemplate("reviewer")
	require.True(t, ok)
	require.Contains(t, rev.TOML, `name        = "reviewer"`)
	require.Contains(t, rev.TOML, `permission_mode = "read-only"`)
	require.Contains(t, rev.TOML, `setting_sources = ["user", "project"]`)
	// reviewer denies write tools
	deny := rev.TOML[strings.Index(rev.TOML, "deny"):]
	require.Contains(t, deny, `"Write"`)
	require.Contains(t, deny, `"Edit"`)
	require.Contains(t, deny, `"NotebookEdit"`)

	_, ok = personaTemplate("nope")
	require.False(t, ok)
}

func newTestEnv(t *testing.T) *environment.Environment {
	t.Helper()
	t.Setenv("CLAUDE_GIT_HOME", t.TempDir())
	env, err := environment.Create(t.TempDir())
	require.NoError(t, err)
	return env
}

func TestScaffoldPersona_WritesTOMLandMD(t *testing.T) {
	env := newTestEnv(t)
	sc, _ := personaTemplate("reviewer")

	err := scaffoldPersona(env, "rev1", sc)
	require.NoError(t, err)

	dir := filepath.Join(environment.RepoDir(env.Hash), "personas", "rev1")
	md, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	require.NoError(t, err)
	require.Equal(t, sc.ClaudeMD, string(md))

	p, err := env.LoadPersona("rev1")
	require.NoError(t, err)
	require.Equal(t, "rev1", p.Name) // name overridden to the new name
	require.Equal(t, "read-only", p.Enforcement.PermissionMode)
	require.ElementsMatch(t, []string{"Write", "Edit", "NotebookEdit", "Bash(git commit:*)", "Bash(git push:*)"}, p.Enforcement.ToolsDeny)

	// second scaffold with the same name is rejected
	err = scaffoldPersona(env, "rev1", sc)
	require.ErrorIs(t, err, domain.ErrPersonaExists)
}
