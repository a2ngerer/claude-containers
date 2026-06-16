package safecopy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func readf(t *testing.T, parts ...string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(parts...))
	require.NoError(t, err)
	return string(b)
}

func TestTreeCopiesAndNormalizes(t *testing.T) {
	src := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(src, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "a.txt"), []byte("A"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(src, "sub", "b.txt"), []byte("B"), 0o755))

	dst := filepath.Join(t.TempDir(), "out")
	require.NoError(t, Tree(src, dst))

	require.Equal(t, "A", readf(t, dst, "a.txt"))
	require.Equal(t, "B", readf(t, dst, "sub", "b.txt"))

	fi, err := os.Stat(filepath.Join(dst, "a.txt"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o644), fi.Mode().Perm(), "file mode must be normalized to 0644")
}

func TestTreeRejectsSymlink(t *testing.T) {
	src := t.TempDir()
	secret := filepath.Join(t.TempDir(), "secret")
	require.NoError(t, os.WriteFile(secret, []byte("TOP SECRET"), 0o600))
	require.NoError(t, os.Symlink(secret, filepath.Join(src, "leak")))

	err := Tree(src, filepath.Join(t.TempDir(), "out"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "symlink not allowed")
}

func TestFileRejectsSymlink(t *testing.T) {
	d := t.TempDir()
	secret := filepath.Join(d, "secret")
	require.NoError(t, os.WriteFile(secret, []byte("TOP SECRET"), 0o600))
	link := filepath.Join(d, "leak")
	require.NoError(t, os.Symlink(secret, link))

	err := File(link, filepath.Join(t.TempDir(), "out"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "symlink not allowed")
}
