package materialize

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/angerer/claude_git/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestWriteMCP_NoConfigRemovesStrayFile(t *testing.T) {
	dir := t.TempDir()
	stray := filepath.Join(dir, "mcp.json")
	require.NoError(t, os.WriteFile(stray, []byte("{}"), 0o644))

	require.NoError(t, writeMCP(dir, domain.MCPConfig{Config: ""}))

	_, err := os.Stat(stray)
	require.True(t, os.IsNotExist(err), "mcp.json must not exist when Config is empty")
}

func TestWriteMCP_ConfigKeepsExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	original := []byte(`{"mcpServers":{"github":{}}}`)
	require.NoError(t, os.WriteFile(path, original, 0o644))

	require.NoError(t, writeMCP(dir, domain.MCPConfig{Config: "mcp.json"}))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, original, got, "existing persona mcp.json must be left untouched")
}

func TestWriteMCP_ConfigWritesPlaceholderWhenMissing(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, writeMCP(dir, domain.MCPConfig{Config: "mcp.json"}))

	got, err := os.ReadFile(filepath.Join(dir, "mcp.json"))
	require.NoError(t, err)
	require.JSONEq(t, `{"mcpServers":{}}`, string(got))
}
