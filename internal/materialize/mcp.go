package materialize

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/angerer/claude_git/internal/domain"
)

// mcpFile is the minimal Claude Code mcp.json shape used only for the empty
// placeholder. A real persona-local mcp.json is copied verbatim and never
// re-encoded through this struct.
type mcpFile struct {
	MCPServers map[string]any `json:"mcpServers"`
}

// writeMCP reconciles destDir/mcp.json with the persona's MCP config.
//
//   - Config == "" : guarantee no mcp.json is present (remove a stray file so no
//     project MCP config can leak into the isolated config dir).
//   - Config != "" : ensure the named file exists; if the copy did not provide
//     one, write an empty {"mcpServers":{}} placeholder so the launch flag
//     --mcp-config <dir>/mcp.json never dangles.
func writeMCP(destDir string, mcp domain.MCPConfig) error {
	path := filepath.Join(destDir, "mcp.json")

	if mcp.Config == "" {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove stray mcp.json: %w", err)
		}
		return nil
	}

	if _, err := os.Stat(path); err == nil {
		return nil // persona shipped its own mcp.json; leave it untouched
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("stat mcp.json: %w", err)
	}

	data, err := json.Marshal(mcpFile{MCPServers: map[string]any{}})
	if err != nil {
		return fmt.Errorf("marshal mcp placeholder: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write mcp.json placeholder: %w", err)
	}
	return nil
}
