package materialize

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/a2ngerer/agent-containers/internal/enforce"
)

// settingsPermissions mirrors the "permissions" object Claude Code reads from
// settings.json. allow/deny use omitempty so an empty persona produces a clean
// object rather than null arrays.
type settingsPermissions struct {
	Allow []string `json:"allow,omitempty"`
	Deny  []string `json:"deny,omitempty"`
}

// settingsFile is the on-disk shape of the generated settings.json. permissionMode
// carries the persona's mode ("read-only" | "default") for visibility and verify.
type settingsFile struct {
	Permissions    settingsPermissions `json:"permissions"`
	PermissionMode string              `json:"permissionMode,omitempty"`
}

// writeSettings serializes the permission set into destDir/settings.json with a
// deterministic two-space indent and a trailing newline (so two runs are byte
// identical).
func writeSettings(destDir string, ps enforce.PermissionSet) error {
	sf := settingsFile{
		Permissions: settingsPermissions{
			Allow: ps.Allow,
			Deny:  ps.Deny,
		},
		PermissionMode: ps.Mode,
	}
	data, err := json.MarshalIndent(sf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	data = append(data, '\n')
	path := filepath.Join(destDir, "settings.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write settings.json: %w", err)
	}
	return nil
}
