package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/a2ngerer/claude-containers/internal/domain"
	"github.com/a2ngerer/claude-containers/internal/environment"
	"github.com/stretchr/testify/require"
)

// chdir switches into dir for the duration of one test, restoring cwd afterward.
func chdir(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(prev) })
}

// execGit runs a git command in dir and returns combined output.
func execGit(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	return cmd.CombinedOutput()
}

// initBareRemoteCLI creates an empty bare git repo with real git and returns its
// path. Real git (not go-git) exercises interop with the remotes users actually
// push to (GitHub, self-hosted).
func initBareRemoteCLI(t *testing.T) string {
	t.Helper()
	bare := t.TempDir()
	out, err := execGit("", "init", "--bare", bare)
	require.NoError(t, err, "git init --bare: %s", out)
	return bare
}

// writeCLIPersonaSnapshot records a persona snapshot via the storage API. The repo
// is bare (no worktree); persona content lives under refs/personas/* as git
// objects, so snapshots are written with WriteTree+WriteSnapshot, not git commit.
func writeCLIPersonaSnapshot(t *testing.T, env *environment.Environment, persona, filename, content string) {
	t.Helper()
	dir := filepath.Join(environment.RepoDir(env.Hash), "personas", persona)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644))
	treeID, err := env.Store.WriteTree(dir)
	require.NoError(t, err)
	_, err = env.Store.WriteSnapshot(domain.Snapshot{
		Persona:   persona,
		TreeID:    string(treeID),
		Message:   "test: " + filename,
		Author:    "tester",
		Timestamp: time.Now(),
	})
	require.NoError(t, err)
}

func TestCloneCmd_SetsUpEnvironmentForCwd(t *testing.T) {
	t.Setenv("CLAUDE_GIT_HOME", t.TempDir())

	src := t.TempDir()
	srcEnv, err := environment.Create(src)
	require.NoError(t, err)
	writeCLIPersonaSnapshot(t, srcEnv, "coder", "persona.toml", "name = \"coder\"\n")

	bare := initBareRemoteCLI(t)
	require.NoError(t, srcEnv.Store.AddRemote("origin", bare))
	require.NoError(t, srcEnv.Store.Push("origin"))

	// run `clone <bare>` with cwd = a fresh workspace
	dst := t.TempDir()
	chdir(t, dst)

	cmd := newCloneCmd()
	cmd.SetArgs([]string{bare})
	require.NoError(t, cmd.Execute())

	// the new environment exists for the resolved cwd: env.toml + timeline + marker
	resolved, err := filepath.EvalSymlinks(dst)
	require.NoError(t, err)
	hash := environment.WorkspaceHash(resolved)

	_, statErr := os.Stat(filepath.Join(environment.EnvDir(hash), "env.toml"))
	require.NoError(t, statErr)

	env, err := environment.Open(dst)
	require.NoError(t, err)
	snaps, err := env.Store.Timeline("coder")
	require.NoError(t, err)
	require.NotEmpty(t, snaps)

	marker, readErr := os.ReadFile(filepath.Join(dst, ".claude_git"))
	require.NoError(t, readErr)
	require.Equal(t, hash+"\n", string(marker))
}
