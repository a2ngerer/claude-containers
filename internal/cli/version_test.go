package cli

import (
	"bytes"
	"testing"

	"github.com/angerer/claude_git/internal/domain"
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

func runLog(t *testing.T, env *environment.Environment, args ...string) (string, error) {
	t.Helper()
	cmd := newLogCmd(func() (*environment.Environment, error) { return env, nil })
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestLogCmd_Roundtrip(t *testing.T) {
	env := newVerTestEnv(t)
	_, err := runNew(t, env, "coder", "--template", "coder")
	require.NoError(t, err)
	_, err = runSnapshot(t, env, "coder", "-m", "one")
	require.NoError(t, err)
	_, err = runSnapshot(t, env, "coder", "-m", "two")
	require.NoError(t, err)

	out, err := runLog(t, env, "coder")
	require.NoError(t, err)

	// newest first: "two" appears before "one"
	idxTwo := indexOf(out, "two")
	idxOne := indexOf(out, "one")
	require.NotEqual(t, -1, idxTwo)
	require.NotEqual(t, -1, idxOne)
	require.Less(t, idxTwo, idxOne)
}

// indexOf is a tiny local substring finder to avoid importing strings here.
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func runDiff(t *testing.T, env *environment.Environment, args ...string) (string, error) {
	t.Helper()
	cmd := newDiffCmd(func() (*environment.Environment, error) { return env, nil })
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestDiffCmd_CapabilityDelta(t *testing.T) {
	env := newVerTestEnv(t)
	base := domain.Persona{
		Name:   "_base",
		Config: domain.Config{ClaudeMD: "CLAUDE.md", Skills: domain.SkillSet{Mode: "allowlist", Include: []string{"shared"}}},
	}
	require.NoError(t, env.SavePersona(base))
	_, err := runNew(t, env, "coder", "--template", "coder")
	require.NoError(t, err)
	_, err = runNew(t, env, "reviewer", "--template", "reviewer")
	require.NoError(t, err)

	out, err := runDiff(t, env, "coder", "reviewer")
	require.NoError(t, err)

	require.Contains(t, out, "coder")
	require.Contains(t, out, "reviewer")
	require.Contains(t, out, "security-review") // only in reviewer
	require.Contains(t, out, "Write")           // allow-only-in-coder OR deny-only-in-reviewer
}
