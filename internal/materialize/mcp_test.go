package materialize

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/a2ngerer/agent-containers/internal/domain"
	"github.com/stretchr/testify/require"
)

// notIsolated is a default-mode enforcement so writeMCP's Config-only branches
// are exercised without the isolation override forcing an empty placeholder.
var notIsolated = domain.Enforcement{PermissionMode: "default"}

func TestWriteMCP_NoConfigRemovesStrayFile(t *testing.T) {
	dir := t.TempDir()
	stray := filepath.Join(dir, "mcp.json")
	require.NoError(t, os.WriteFile(stray, []byte("{}"), 0o644))

	require.NoError(t, writeMCP(dir, notIsolated, domain.MCPConfig{Config: ""}))

	_, err := os.Stat(stray)
	require.True(t, os.IsNotExist(err), "mcp.json must not exist when Config is empty and not isolated")
}

// I3 — when the persona is MCP-isolated (read-only or Strict) but ships no MCP
// config, writeMCP must materialize an empty placeholder instead of removing it.
func TestWriteMCP_IsolatedWritesEmptyPlaceholder(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, writeMCP(dir, domain.Enforcement{PermissionMode: "read-only"}, domain.MCPConfig{Config: ""}))

	got, err := os.ReadFile(filepath.Join(dir, "mcp.json"))
	require.NoError(t, err)
	require.JSONEq(t, `{"mcpServers":{}}`, string(got))
}

func TestWriteMCP_StrictWritesEmptyPlaceholder(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, writeMCP(dir, notIsolated, domain.MCPConfig{Config: "", Strict: true}))

	got, err := os.ReadFile(filepath.Join(dir, "mcp.json"))
	require.NoError(t, err)
	require.JSONEq(t, `{"mcpServers":{}}`, string(got))
}

func TestWriteMCP_ConfigKeepsExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	original := []byte(`{"mcpServers":{"github":{}}}`)
	require.NoError(t, os.WriteFile(path, original, 0o644))

	require.NoError(t, writeMCP(dir, notIsolated, domain.MCPConfig{Config: "mcp.json"}))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, original, got, "existing persona mcp.json must be left untouched")
}

func TestWriteMCP_ConfigWritesPlaceholderWhenMissing(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, writeMCP(dir, notIsolated, domain.MCPConfig{Config: "mcp.json"}))

	got, err := os.ReadFile(filepath.Join(dir, "mcp.json"))
	require.NoError(t, err)
	require.JSONEq(t, `{"mcpServers":{}}`, string(got))
}
