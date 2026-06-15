package activate

import (
	"path/filepath"
	"strings"

	"github.com/angerer/claude_git/internal/compose"
	"github.com/angerer/claude_git/internal/domain"
	"github.com/angerer/claude_git/internal/enforce"
)

// defaultSettingSources is substituted when a persona leaves SettingSrc empty.
// Emitting "--setting-sources ''" would make Claude Code consult every source,
// defeating isolation; an isolated persona should consult at most the project
// scope, so that is the safe default.
var defaultSettingSources = []string{"project"}

// LaunchSpec is the environment + argv to start (or print) for claude.
type LaunchSpec struct {
	Env  []string // e.g. ["CLAUDE_CONFIG_DIR=..."]
	Argv []string // e.g. ["claude","--setting-sources","user,project", ...]
}

// BuildLaunch assembles the exact, reproducible claude invocation for a
// materialized config dir. MCP flags (--strict-mcp-config, --mcp-config) are
// emitted when the persona configures an MCP file OR when it is MCP-isolated
// (domain.MCPIsolated: read-only or MCP.Strict); in both cases an mcp.json is
// materialized. Otherwise they are omitted entirely so no MCP source is
// consulted. The flags stay consistent with what Materialize writes and Verify
// expects.
func BuildLaunch(configDir string, rm compose.ResolvedManifest) LaunchSpec {
	allow := enforce.BuildPermissions(rm.Enforcement).Allow

	settingSrc := rm.SettingSrc
	if len(settingSrc) == 0 {
		settingSrc = defaultSettingSources
	}

	argv := []string{
		"claude",
		"--setting-sources", strings.Join(settingSrc, ","),
	}
	if rm.MCP.Config != "" || domain.MCPIsolated(rm.Enforcement, rm.MCP) {
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
