package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestList_ShowsBaseAfterInit(t *testing.T) {
	t.Setenv("CLAUDE_GIT_HOME", t.TempDir())
	ws := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(ws, "CLAUDE.md"), []byte("# rules\n"), 0o644))

	_, err := runCLI(t, "init", "--workspace", ws)
	require.NoError(t, err)

	out, err := runCLI(t, "list", "--workspace", ws)
	require.NoError(t, err)
	require.Contains(t, out, "_base")
	require.Contains(t, out, "0.1.0")
}

func TestList_NotInitialized(t *testing.T) {
	t.Setenv("CLAUDE_GIT_HOME", t.TempDir())
	ws := t.TempDir()
	_, err := runCLI(t, "list", "--workspace", ws)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not initialized")
}
