package cli

import (
	"bytes"
	"testing"

	"github.com/angerer/claude_git/internal/environment"
	"github.com/stretchr/testify/require"
)

func newVerTestEnv(t *testing.T) *environment.Environment {
	t.Helper()
	t.Setenv("CLAUDE_GIT_HOME", t.TempDir())
	env, err := environment.Create(t.TempDir())
	require.NoError(t, err)
	return env
}

func runSnapshot(t *testing.T, env *environment.Environment, args ...string) (string, error) {
	t.Helper()
	cmd := newSnapshotCmd(func() (*environment.Environment, error) { return env, nil })
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestSnapshotCmd_WritesToTimeline(t *testing.T) {
	env := newVerTestEnv(t)
	_, err := runNew(t, env, "coder", "--template", "coder")
	require.NoError(t, err)

	out, err := runSnapshot(t, env, "coder", "-m", "first")
	require.NoError(t, err)
	require.Contains(t, out, "Snapshot")

	ids, err := env.Store.Timeline("coder")
	require.NoError(t, err)
	require.Len(t, ids, 1)

	snap, err := env.Store.ReadSnapshot(ids[0])
	require.NoError(t, err)
	require.Equal(t, "coder", snap.Persona)
	require.Equal(t, "first", snap.Message)
	require.NotEmpty(t, snap.TreeID)
	require.False(t, snap.Timestamp.IsZero())
}

func TestSnapshotCmd_DefaultsToActivePersona(t *testing.T) {
	env := newVerTestEnv(t)
	_, err := runNew(t, env, "coder", "--template", "coder")
	require.NoError(t, err)
	require.NoError(t, env.SetActive("coder"))

	_, err = runSnapshot(t, env) // no persona arg
	require.NoError(t, err)

	ids, err := env.Store.Timeline("coder")
	require.NoError(t, err)
	require.Len(t, ids, 1)
}
