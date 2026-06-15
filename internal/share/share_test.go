package share

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/angerer/claude_git/internal/domain"
	"github.com/angerer/claude_git/internal/environment"
	"github.com/angerer/claude_git/internal/storage"
	"github.com/stretchr/testify/require"
)

// withToolHome points CLAUDE_GIT_HOME at a fresh temp dir for the duration of one test.
func withToolHome(t *testing.T) {
	t.Helper()
	t.Setenv("CLAUDE_GIT_HOME", t.TempDir())
}

// initBareRemote creates an empty bare git repo at a temp dir and returns the path.
// Uses storage.OpenGit so no exec dependency; the bare repo is compatible with
// the storage engine's Push/Pull refspecs (refs/personas/*, refs/tags/*).
func initBareRemote(t *testing.T) string {
	t.Helper()
	bare := t.TempDir()
	_, err := storage.OpenGit(bare)
	require.NoError(t, err, "init bare remote repo")
	return bare
}

// writePersonaSnapshot writes a persona dir with one file, snapshots it via the
// storage API, and returns the snapshot ID. This drives history on a bare repo
// without requiring a working-tree git commit.
func writePersonaSnapshot(t *testing.T, env *environment.Environment, persona, filename, content string) domain.SnapshotID {
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

func TestPush_AbortsOnSecret(t *testing.T) {
	withToolHome(t)
	ws := t.TempDir()
	env, err := environment.Create(ws)
	require.NoError(t, err)

	// plant a secret file directly in the persona repo
	repo := environment.RepoDir(env.Hash)
	require.NoError(t, os.WriteFile(filepath.Join(repo, "leaked.key"), []byte("x"), 0o600))

	pushErr := Push(env, "origin")
	require.Error(t, pushErr)
	require.True(t, errors.Is(pushErr, ErrSecretsFound), "want ErrSecretsFound, got %v", pushErr)
	require.Contains(t, pushErr.Error(), "leaked.key")
}

func TestPushPull_RoundTrip(t *testing.T) {
	withToolHome(t)
	bare := initBareRemote(t)

	// producer: create env, write a persona snapshot, push
	wsA := t.TempDir()
	envA, err := environment.Create(wsA)
	require.NoError(t, err)
	writePersonaSnapshot(t, envA, "_base", "MARKER.md", "round-trip-token\n")

	require.NoError(t, envA.Store.AddRemote("origin", bare))
	require.NoError(t, Push(envA, "origin"))

	// consumer: clone from the same bare remote into a fresh workspace
	wsB := t.TempDir()
	envB, err := Clone(bare, wsB)
	require.NoError(t, err)

	// the _base persona timeline must be present in the clone
	snaps, err := envB.Store.Timeline("_base")
	require.NoError(t, err)
	require.NotEmpty(t, snaps)

	// the snapshot's tree must contain MARKER.md with the expected content
	snap, err := envB.Store.ReadSnapshot(snaps[0])
	require.NoError(t, err)

	dst := t.TempDir()
	require.NoError(t, envB.Store.CheckoutTree(storage.ObjectID(snap.TreeID), dst))
	data, err := os.ReadFile(filepath.Join(dst, "MARKER.md"))
	require.NoError(t, err)
	require.Equal(t, "round-trip-token\n", string(data))
}

func TestPull_FetchesNewCommits(t *testing.T) {
	withToolHome(t)
	bare := initBareRemote(t)

	wsA := t.TempDir()
	envA, err := environment.Create(wsA)
	require.NoError(t, err)
	require.NoError(t, envA.Store.AddRemote("origin", bare))
	require.NoError(t, Push(envA, "origin"))

	wsB := t.TempDir()
	envB, err := Clone(bare, wsB)
	require.NoError(t, err)

	// A adds a new persona snapshot and pushes it
	writePersonaSnapshot(t, envA, "_base", "SECOND.md", "second\n")
	require.NoError(t, Push(envA, "origin"))

	// Before pull, _base should not exist in envB (empty repo was cloned)
	_, errBefore := envB.Store.Timeline("_base")
	require.Error(t, errBefore, "before pull, _base should not exist in envB")

	// Pull must bring the new snapshot
	require.NoError(t, Pull(envB, "origin"))

	snapsAfter, err := envB.Store.Timeline("_base")
	require.NoError(t, err)
	require.NotEmpty(t, snapsAfter)
}
