package probe

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func gitAvailable(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, string(out))
}

func TestIsClaudeTracked_TrackedRepo(t *testing.T) {
	gitAvailable(t)
	ws := t.TempDir()
	runGit(t, ws, "init")
	runGit(t, ws, "config", "user.email", "t@example.com")
	runGit(t, ws, "config", "user.name", "Tester")
	require.NoError(t, os.MkdirAll(filepath.Join(ws, ".claude"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ws, ".claude", "settings.json"), []byte("{}"), 0o644))
	runGit(t, ws, "add", ".claude")
	runGit(t, ws, "commit", "-m", "track claude")

	tracked, err := IsClaudeTracked(ws)
	require.NoError(t, err)
	require.True(t, tracked)
}

func TestIsClaudeTracked_UntrackedInRepo(t *testing.T) {
	gitAvailable(t)
	ws := t.TempDir()
	runGit(t, ws, "init")
	require.NoError(t, os.MkdirAll(filepath.Join(ws, ".claude"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ws, ".claude", "settings.json"), []byte("{}"), 0o644))

	tracked, err := IsClaudeTracked(ws)
	require.NoError(t, err)
	require.False(t, tracked)
}

func TestIsClaudeTracked_NotAGitRepo(t *testing.T) {
	gitAvailable(t)
	ws := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(ws, ".claude"), 0o755))

	tracked, err := IsClaudeTracked(ws)
	require.NoError(t, err)
	require.False(t, tracked)
}
