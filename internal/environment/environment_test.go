package environment

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/a2ngerer/claude-containers/internal/domain"
	"github.com/stretchr/testify/require"
)

func setupHome(t *testing.T) {
	t.Helper()
	t.Setenv("CLAUDE_GIT_HOME", t.TempDir())
}

func TestCreate_MakesDirsAndConfig(t *testing.T) {
	setupHome(t)
	ws := t.TempDir()
	// EvalSymlinks is now applied inside Create/Open; resolve here too so the
	// expected values match the canonical path (macOS /var -> /private/var).
	wsReal, err := filepath.EvalSymlinks(ws)
	require.NoError(t, err)

	env, err := Create(ws)
	require.NoError(t, err)
	require.Equal(t, WorkspaceHash(filepath.Clean(wsReal)), env.Hash)
	require.Equal(t, filepath.Clean(wsReal), env.Workspace)
	require.NotNil(t, env.Store)

	// re-open must succeed and report the same workspace
	reopened, err := Open(ws)
	require.NoError(t, err)
	require.Equal(t, env.Hash, reopened.Hash)
	require.Equal(t, filepath.Clean(wsReal), reopened.Workspace)
}

func TestOpen_NotInitialized(t *testing.T) {
	setupHome(t)
	ws := t.TempDir()
	_, err := Open(ws)
	require.True(t, errors.Is(err, domain.ErrNotInitialized))
}

func TestPersonaCRUD(t *testing.T) {
	setupHome(t)
	ws := t.TempDir()
	env, err := Create(ws)
	require.NoError(t, err)

	p := domain.Persona{
		Name:        "_base",
		Description: "shared base",
		Enforcement: domain.Enforcement{
			PermissionMode: "default",
			ToolsAllow:     []string{"Read"},
			ToolsDeny:      []string{"Write"},
		},
		Metadata: domain.Metadata{Version: "0.1.0", Author: "alexander.angerer"},
	}
	require.NoError(t, env.SavePersona(p))

	got, err := env.LoadPersona("_base")
	require.NoError(t, err)
	require.Equal(t, "_base", got.Name)
	require.Equal(t, []string{"Read"}, got.Enforcement.ToolsAllow)
	require.Equal(t, []string{"Write"}, got.Enforcement.ToolsDeny)

	list, err := env.ListPersonas()
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "_base", list[0].Name)
}

func TestLoadPersona_NotFound(t *testing.T) {
	setupHome(t)
	ws := t.TempDir()
	env, err := Create(ws)
	require.NoError(t, err)
	_, err = env.LoadPersona("ghost")
	require.True(t, errors.Is(err, domain.ErrPersonaNotFound))
}

func TestSetActive_Persists(t *testing.T) {
	setupHome(t)
	ws := t.TempDir()
	env, err := Create(ws)
	require.NoError(t, err)
	require.NoError(t, env.SavePersona(domain.Persona{Name: "coder"}))
	require.NoError(t, env.SetActive("coder"))

	reopened, err := Open(ws)
	require.NoError(t, err)
	require.Equal(t, "coder", reopened.ActivePersona())
}

func TestCreate_SetsAuthor(t *testing.T) {
	setupHome(t)
	ws := t.TempDir()
	env, err := Create(ws)
	require.NoError(t, err)
	// Author must be non-empty (derived from git config or OS user).
	require.NotEmpty(t, env.Author())

	// Must persist: reopening the env returns the same author.
	reopened, err := Open(ws)
	require.NoError(t, err)
	require.Equal(t, env.Author(), reopened.Author())
}
