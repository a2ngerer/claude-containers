package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/a2ngerer/claude-containers/internal/environment"
	"github.com/stretchr/testify/require"
)

func TestStatus_AfterInit(t *testing.T) {
	t.Setenv("CLAUDE_GIT_HOME", t.TempDir())
	ws := t.TempDir()
	// EvalSymlinks resolves macOS /var -> /private/var for stable hashes.
	wsReal, err := filepath.EvalSymlinks(ws)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(ws, "CLAUDE.md"), []byte("# rules\n"), 0o644))

	_, err = runCLI(t, "init", "--workspace", ws)
	require.NoError(t, err)

	out, err := runCLI(t, "status", "--workspace", ws)
	require.NoError(t, err)
	require.Contains(t, out, filepath.Clean(wsReal))
	require.Contains(t, out, environment.WorkspaceHash(filepath.Clean(wsReal)))
	require.Contains(t, out, "none") // no active persona yet
}

func TestStatus_ReflectsActivePersona(t *testing.T) {
	t.Setenv("CLAUDE_GIT_HOME", t.TempDir())
	ws := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(ws, "CLAUDE.md"), []byte("# rules\n"), 0o644))
	_, err := runCLI(t, "init", "--workspace", ws)
	require.NoError(t, err)

	env, err := environment.Open(ws)
	require.NoError(t, err)
	require.NoError(t, env.SetActive("_base"))

	out, err := runCLI(t, "status", "--workspace", ws)
	require.NoError(t, err)
	require.Contains(t, out, "_base")
}

func TestStatus_NotInitialized(t *testing.T) {
	t.Setenv("CLAUDE_GIT_HOME", t.TempDir())
	ws := t.TempDir()
	_, err := runCLI(t, "status", "--workspace", ws)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not initialized")
}
