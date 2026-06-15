package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/angerer/claude_git/internal/environment"
	"github.com/stretchr/testify/require"
)

// TestInitFrom_ClonesExistingRepo verifies the team-onboarding path: `init --from
// <remote>` clones an existing persona repo instead of seeding _base from the
// local .claude/. The two paths are mutually exclusive (spec §11).
func TestInitFrom_ClonesExistingRepo(t *testing.T) {
	t.Setenv("CLAUDE_GIT_HOME", t.TempDir())

	// produce a source repo with a reviewer persona, push to a bare remote
	src := t.TempDir()
	srcEnv, err := environment.Create(src)
	require.NoError(t, err)
	writeCLIPersonaSnapshot(t, srcEnv, "reviewer", "persona.toml", "name = \"reviewer\"\n")

	bare := initBareRemoteCLI(t)
	require.NoError(t, srcEnv.Store.AddRemote("origin", bare))
	require.NoError(t, srcEnv.Store.Push("origin"))

	// onboard: `init --from <bare>` in a fresh workspace
	dst := t.TempDir()
	chdir(t, dst)
	cmd := newInitCmd()
	cmd.SetArgs([]string{"--from", bare})
	require.NoError(t, cmd.Execute())

	// the reviewer persona was cloned in, and the marker is set
	env, err := environment.Open(dst)
	require.NoError(t, err)
	snaps, err := env.Store.Timeline("reviewer")
	require.NoError(t, err)
	require.NotEmpty(t, snaps)

	resolved, err := filepath.EvalSymlinks(dst)
	require.NoError(t, err)
	hash := environment.WorkspaceHash(resolved)
	marker, mErr := os.ReadFile(filepath.Join(dst, ".claude_git"))
	require.NoError(t, mErr)
	require.Equal(t, hash+"\n", string(marker))
}
