package activate

import (
	"path/filepath"
	"strings"

	"github.com/angerer/claude_git/internal/compose"
	"github.com/angerer/claude_git/internal/enforce"
)

// LaunchSpec is the environment + argv to start (or print) for claude.
type LaunchSpec struct {
	Env  []string // e.g. ["CLAUDE_CONFIG_DIR=..."]
	Argv []string // e.g. ["claude","--setting-sources","user,project", ...]
}

// BuildLaunch assembles the exact, reproducible claude invocation for a
// materialized config dir. MCP flags (--strict-mcp-config, --mcp-config) are
// emitted only when the persona configures an MCP file; otherwise they are
// omitted entirely so no MCP source is consulted.
func BuildLaunch(configDir string, rm compose.ResolvedManifest) LaunchSpec {
	allow := enforce.BuildPermissions(rm.Enforcement).Allow

	argv := []string{
		"claude",
		"--setting-sources", strings.Join(rm.SettingSrc, ","),
	}
	if rm.MCP.Config != "" {
		argv = append(argv,
			"--strict-mcp-config",
			"--mcp-config", filepath.Join(configDir, "mcp.json"),
		)
	}
	argv = append(argv,
		"--allowedTools", strings.Join(allow, ","),
		"--append-system-prompt", "@"+filepath.Join(configDir, "CLAUDE.md"),
	)

	return LaunchSpec{
		Env:  []string{"CLAUDE_CONFIG_DIR=" + configDir},
		Argv: argv,
	}
}
