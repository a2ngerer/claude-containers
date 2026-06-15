package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/angerer/claude_git/internal/environment"
	"github.com/stretchr/testify/require"
)

// seedReviewer creates a workspace + reviewer persona and chdirs into the
// workspace so the command's os.Getwd() resolves to it.
// Both home and ws are symlink-resolved so that environment.Create and
// os.Getwd() (after Chdir) produce the same hash on macOS (/var -> /private/var).
func seedReviewer(t *testing.T) *environment.Environment {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	t.Setenv("CLAUDE_GIT_HOME", home)
	ws, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	e, err := environment.Create(ws)
	require.NoError(t, err)

	repo := environment.RepoDir(e.Hash)
	base := filepath.Join(repo, "personas", "_base")
	require.NoError(t, os.MkdirAll(base, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(base, "persona.toml"),
		[]byte("name = \"_base\"\n\n[config]\nclaude_md = \"CLAUDE.md\"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(base, "CLAUDE.md"), []byte("# base\n"), 0o644))

	rev := filepath.Join(repo, "personas", "reviewer")
	require.NoError(t, os.MkdirAll(filepath.Join(rev, "skills", "security-review"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(rev, "skills", "security-review", "SKILL.md"), []byte("# sr\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(rev, "CLAUDE.md"), []byte("# reviewer\n"), 0o644))
	manifest := `name = "reviewer"
extends = "_base"

[config]
claude_md = "CLAUDE.md"
setting_sources = ["user", "project"]

[config.skills]
mode = "allowlist"
include = ["security-review"]

[config.mcp]
config = ""
strict = true

[enforcement]
permission_mode = "read-only"
tools.allow = ["Read", "Grep"]
tools.deny = ["Bash(git commit:*)"]

[metadata]
version = "1.2.0"
author = "tester"
`
	require.NoError(t, os.WriteFile(filepath.Join(rev, "persona.toml"), []byte(manifest), 0o644))

	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(wd) })
	require.NoError(t, os.Chdir(ws))
	return e
}

func TestVerifyCmd_Clean(t *testing.T) {
	seedReviewer(t)

	cmd := newVerifyCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"reviewer"})

	require.NoError(t, cmd.Execute())
	require.Contains(t, out.String(), "reviewer")
	require.Contains(t, out.String(), "uncontaminated")
}

func TestVerifyCmd_UnknownPersonaErrors(t *testing.T) {
	seedReviewer(t)

	cmd := newVerifyCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs([]string{"ghost"})

	err := cmd.Execute()
	require.Error(t, err)
}
