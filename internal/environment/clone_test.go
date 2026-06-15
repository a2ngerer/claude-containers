package environment_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/a2ngerer/claude-containers/internal/domain"
	"github.com/a2ngerer/claude-containers/internal/environment"
	"github.com/a2ngerer/claude-containers/internal/share"
	"github.com/a2ngerer/claude-containers/internal/storage"
	"github.com/stretchr/testify/require"
)

// writeTestPersonaSnapshot writes a persona dir with one file and records a
// storage snapshot so there is something to push. Returns the snapshot ID.
func writeTestPersonaSnapshot(t *testing.T, env *environment.Environment, persona, filename, content string) domain.SnapshotID {
	t.Helper()
	dir := filepath.Join(environment.RepoDir(env.Hash), "personas", persona)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644))

	treeID, err := env.Store.WriteTree(dir)
	require.NoError(t, err)

	sid, err := env.Store.WriteSnapshot(domain.Snapshot{
		Persona:   persona,
		TreeID:    string(treeID),
		Message:   "test: " + filename,
		Author:    "tester",
		Timestamp: time.Now(),
	})
	require.NoError(t, err)
	return sid
}

func TestCreate_WritesGitignore(t *testing.T) {
	t.Setenv("CLAUDE_GIT_HOME", t.TempDir())
	ws := t.TempDir()

	env, err := environment.Create(ws)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(environment.RepoDir(env.Hash), ".gitignore"))
	require.NoError(t, err)
	require.Equal(t, share.DefaultGitignore(), string(data),
		"repo/.gitignore must equal share.DefaultGitignore()")
}

func TestCloneInto_PopulatesAndOpens(t *testing.T) {
	t.Setenv("CLAUDE_GIT_HOME", t.TempDir())

	// build a source repo, write a persona snapshot, push to a bare remote
	src := t.TempDir()
	srcEnv, err := environment.Create(src)
	require.NoError(t, err)
	writeTestPersonaSnapshot(t, srcEnv, "_base", "hello.md", "hello\n")

	// bare remote: storage.OpenGit on a fresh temp dir creates a bare repo
	bare := t.TempDir()
	_, err = storage.OpenGit(bare)
	require.NoError(t, err)

	require.NoError(t, srcEnv.Store.AddRemote("origin", bare))
	require.NoError(t, share.Push(srcEnv, "origin"))

	// clone into a fresh workspace
	dst := t.TempDir()
	dstEnv, err := environment.CloneInto(dst, bare)
	require.NoError(t, err)

	// env.toml exists
	_, statErr := os.Stat(filepath.Join(environment.EnvDir(dstEnv.Hash), "env.toml"))
	require.NoError(t, statErr)

	// persona refs arrived: _base timeline is accessible
	snaps, err := dstEnv.Store.Timeline("_base")
	require.NoError(t, err)
	require.NotEmpty(t, snaps)

	// workspace marker exists and contains the hash
	marker, readErr := os.ReadFile(filepath.Join(dst, ".claude_git"))
	require.NoError(t, readErr)
	require.Equal(t, dstEnv.Hash+"\n", string(marker))
}

func TestGitignoreInSync(t *testing.T) {
	// The inlined environment.defaultGitignore must stay byte-identical to
	// share.DefaultGitignore(). Compared via Create's output (the only public surface).
	t.Setenv("CLAUDE_GIT_HOME", t.TempDir())
	ws := t.TempDir()
	env, err := environment.Create(ws)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(environment.RepoDir(env.Hash), ".gitignore"))
	require.NoError(t, err)
	require.Equal(t, share.DefaultGitignore(), string(data),
		"environment.defaultGitignore drifted from share.DefaultGitignore() — keep them identical")
}
