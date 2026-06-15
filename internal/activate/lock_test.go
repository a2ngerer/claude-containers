package activate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/angerer/claude_git/internal/domain"
	"github.com/angerer/claude_git/internal/environment"
	"github.com/stretchr/testify/require"
)

func lockEnv(t *testing.T) *environment.Environment {
	t.Helper()
	home := t.TempDir()
	t.Setenv("CLAUDE_GIT_HOME", home)
	ws := t.TempDir()
	e, err := environment.Create(ws)
	require.NoError(t, err)
	return e
}

func lockPath(e *environment.Environment) string {
	return filepath.Join(environment.EnvDir(e.Hash), "lock")
}

func TestLock_AcquireOnFreeDir(t *testing.T) {
	e := lockEnv(t)

	lk, err := Acquire(e, "reviewer")
	require.NoError(t, err)
	require.NotNil(t, lk)

	data, err := os.ReadFile(lockPath(e))
	require.NoError(t, err)
	var st lockState
	require.NoError(t, json.Unmarshal(data, &st))
	require.Equal(t, "reviewer", st.Persona)
	require.Equal(t, os.Getpid(), st.PID)
}

func TestLock_ReLockBySamePID(t *testing.T) {
	e := lockEnv(t)
	_, err := Acquire(e, "coder")
	require.NoError(t, err)

	// Same process re-acquiring (e.g. switching persona) must succeed.
	lk, err := Acquire(e, "reviewer")
	require.NoError(t, err)
	require.NotNil(t, lk)
}

func TestLock_ForeignLivePIDIsLocked(t *testing.T) {
	e := lockEnv(t)
	// PID 1 is always alive on POSIX and is not us.
	writeForeignLock(t, e, "coder", 1)

	_, err := Acquire(e, "reviewer")
	require.ErrorIs(t, err, domain.ErrLocked)
}

func TestLock_StalePIDReLock(t *testing.T) {
	e := lockEnv(t)
	// A PID essentially guaranteed not to exist -> stale lock.
	writeForeignLock(t, e, "coder", 2147483640)

	lk, err := Acquire(e, "reviewer")
	require.NoError(t, err)
	require.NotNil(t, lk)
}

func TestLock_Release(t *testing.T) {
	e := lockEnv(t)
	lk, err := Acquire(e, "reviewer")
	require.NoError(t, err)

	require.NoError(t, lk.Release())
	_, err = os.Stat(lockPath(e))
	require.True(t, os.IsNotExist(err))
}

func writeForeignLock(t *testing.T, e *environment.Environment, persona string, pid int) {
	t.Helper()
	data, err := json.Marshal(lockState{Persona: persona, PID: pid})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(lockPath(e), data, 0o644))
}
