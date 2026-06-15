package environment

import (
	"crypto/sha1"
	"encoding/hex"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWorkspaceHash_StableAndCleaned(t *testing.T) {
	base := "/Users/x/proj"
	h1 := WorkspaceHash(base)
	h2 := WorkspaceHash("/Users/x/proj/") // trailing slash -> cleaned to same
	h3 := WorkspaceHash("/Users/x/proj/../proj")
	require.Equal(t, h1, h2)
	require.Equal(t, h1, h3)

	sum := sha1.Sum([]byte(filepath.Clean(base)))
	require.Equal(t, hex.EncodeToString(sum[:]), h1)
	require.Len(t, h1, 40)
}

func TestWorkspaceHash_DifferentPathsDiffer(t *testing.T) {
	require.NotEqual(t, WorkspaceHash("/a/b"), WorkspaceHash("/a/c"))
}

func TestToolHome_RespectsEnv(t *testing.T) {
	t.Setenv("CLAUDE_GIT_HOME", "/tmp/cg-home")
	require.Equal(t, "/tmp/cg-home", ToolHome())
}

func TestToolHome_DefaultUnderHome(t *testing.T) {
	t.Setenv("CLAUDE_GIT_HOME", "")
	t.Setenv("HOME", "/tmp/fake-home")
	require.Equal(t, filepath.Join("/tmp/fake-home", ".claude_git"), ToolHome())
}

func TestDerivedPaths(t *testing.T) {
	t.Setenv("CLAUDE_GIT_HOME", "/tmp/cg")
	hash := "abc123"
	require.Equal(t, filepath.Join("/tmp/cg", "environments", hash), EnvDir(hash))
	require.Equal(t, filepath.Join("/tmp/cg", "environments", hash, "repo"), RepoDir(hash))
	require.Equal(t, filepath.Join("/tmp/cg", "cache", hash, "reviewer"), CacheDir(hash, "reviewer"))
}
