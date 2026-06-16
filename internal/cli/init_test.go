package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/a2ngerer/agent-containers/internal/environment"
	"github.com/stretchr/testify/require"
)

func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func seedWorkspace(t *testing.T) string {
	t.Helper()
	ws := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(ws, ".claude", "skills", "demo"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ws, ".claude", "settings.json"), []byte(`{"x":1}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(ws, ".claude", "skills", "demo", "SKILL.md"), []byte("demo skill\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(ws, "CLAUDE.md"), []byte("# project rules\n"), 0o644))
	return ws
}

func TestInit_BindsWorkspaceAndImportsBase(t *testing.T) {
	t.Setenv("ACON_HOME", t.TempDir())
	ws := seedWorkspace(t)
	// EvalSymlinks is now applied inside Create/Open; resolve here too so that
	// expected hash values match the canonical path (macOS /var -> /private/var).
	wsReal, err := filepath.EvalSymlinks(ws)
	require.NoError(t, err)

	out, err := runCLI(t, "init", "--workspace", ws)
	require.NoError(t, err)
	require.Contains(t, out, "Initialized")

	// marker file written into the workspace, one line = hash
	marker, err := os.ReadFile(filepath.Join(ws, ".acon"))
	require.NoError(t, err)
	require.Equal(t, environment.WorkspaceHash(filepath.Clean(wsReal)), strings.TrimSpace(string(marker)))

	// _base persona created with imported content
	hash := environment.WorkspaceHash(filepath.Clean(wsReal))
	baseDir := filepath.Join(environment.RepoDir(hash), "personas", "_base")
	require.FileExists(t, filepath.Join(baseDir, "persona.toml"))
	require.FileExists(t, filepath.Join(baseDir, "CLAUDE.md"))
	require.FileExists(t, filepath.Join(baseDir, ".claude", "settings.json"))
	require.FileExists(t, filepath.Join(baseDir, ".claude", "skills", "demo", "SKILL.md"))

	// _base appears via the environment API
	env, err := environment.Open(ws)
	require.NoError(t, err)
	personas, err := env.ListPersonas()
	require.NoError(t, err)
	require.Len(t, personas, 1)
	require.Equal(t, "_base", personas[0].Name)
	require.True(t, personas[0].IsLayer())

	// workspace .claude/ left untouched (still exactly what we seeded)
	orig, err := os.ReadFile(filepath.Join(ws, ".claude", "settings.json"))
	require.NoError(t, err)
	require.Equal(t, `{"x":1}`, string(orig))
}

func TestInit_NoExistingClaude(t *testing.T) {
	t.Setenv("ACON_HOME", t.TempDir())
	ws := t.TempDir()
	wsReal, err := filepath.EvalSymlinks(ws)
	require.NoError(t, err)

	out, err := runCLI(t, "init", "--workspace", ws)
	require.NoError(t, err)
	require.Contains(t, out, "Initialized")

	hash := environment.WorkspaceHash(filepath.Clean(wsReal))
	baseDir := filepath.Join(environment.RepoDir(hash), "personas", "_base")
	require.FileExists(t, filepath.Join(baseDir, "persona.toml"))
}
