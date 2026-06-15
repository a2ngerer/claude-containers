package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/a2ngerer/claude-containers/internal/environment"
	"github.com/a2ngerer/claude-containers/internal/storage"
	"github.com/stretchr/testify/require"
)

// TestPushClone_E2E_RoundTrip exercises the full user path through the CLI:
// create an environment in workspace A, snapshot a persona, `push` to a bare
// remote, `clone` into workspace B, then assert the persona content arrived and
// the secret-safe .gitignore guard is present (acceptance criterion §18.7).
func TestPushClone_E2E_RoundTrip(t *testing.T) {
	t.Setenv("CLAUDE_GIT_HOME", t.TempDir())
	bare := initBareRemoteCLI(t)

	// --- producer: create env in workspace A, snapshot a persona, push ---
	wsA := t.TempDir()
	envA, err := environment.Create(wsA)
	require.NoError(t, err)
	writeCLIPersonaSnapshot(t, envA, "coder", "persona.toml", "name = \"coder\"\ndescription = \"builder\"\n")
	require.NoError(t, envA.Store.AddRemote("origin", bare))

	chdir(t, wsA)
	pushCmd := newPushCmd()
	pushCmd.SetArgs([]string{"origin"})
	require.NoError(t, pushCmd.Execute())

	// --- consumer: clone into workspace B ---
	wsB := t.TempDir()
	chdir(t, wsB)
	cloneCmd := newCloneCmd()
	cloneCmd.SetArgs([]string{bare})
	require.NoError(t, cloneCmd.Execute())

	// the coder persona arrived in B: timeline present, tree content matches
	envB, err := environment.Open(wsB)
	require.NoError(t, err)
	snaps, err := envB.Store.Timeline("coder")
	require.NoError(t, err)
	require.NotEmpty(t, snaps)

	snap, err := envB.Store.ReadSnapshot(snaps[0])
	require.NoError(t, err)
	checkout := t.TempDir()
	require.NoError(t, envB.Store.CheckoutTree(storage.ObjectID(snap.TreeID), checkout))
	data, err := os.ReadFile(filepath.Join(checkout, "persona.toml"))
	require.NoError(t, err)
	require.Contains(t, string(data), "name = \"coder\"")

	// secret-safe: the cloned repo carries the .gitignore guard
	_, statErr := os.Stat(filepath.Join(environment.RepoDir(envB.Hash), ".gitignore"))
	require.NoError(t, statErr)
}
