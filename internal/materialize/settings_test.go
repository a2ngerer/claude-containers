package materialize

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/angerer/claude_git/internal/enforce"
	"github.com/stretchr/testify/require"
)

func TestWriteSettings(t *testing.T) {
	dir := t.TempDir()
	ps := enforce.PermissionSet{
		Allow: []string{"Read", "Grep"},
		Deny:  []string{"Write", "Edit", "NotebookEdit"},
		Mode:  "read-only",
	}

	require.NoError(t, writeSettings(dir, ps))

	raw, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	require.NoError(t, err)

	var got settingsFile
	require.NoError(t, json.Unmarshal(raw, &got))
	require.Equal(t, []string{"Read", "Grep"}, got.Permissions.Allow)
	require.Equal(t, []string{"Write", "Edit", "NotebookEdit"}, got.Permissions.Deny)
	require.Equal(t, "read-only", got.PermissionMode)

	// Deterministic, pretty-printed, trailing newline.
	require.Equal(t, byte('\n'), raw[len(raw)-1])
}
