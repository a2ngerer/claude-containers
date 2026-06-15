package activate

import (
	"testing"

	"github.com/angerer/claude_git/internal/compose"
	"github.com/angerer/claude_git/internal/domain"
	"github.com/stretchr/testify/require"
)

func baseRM() compose.ResolvedManifest {
	return compose.ResolvedManifest{
		Persona:    domain.Persona{Name: "reviewer"},
		SettingSrc: []string{"user", "project"},
		Enforcement: domain.Enforcement{
			PermissionMode: "read-only",
			ToolsAllow:     []string{"Read", "Grep", "Glob"},
		},
	}
}

// I3 — a read-only persona is MCP-isolated even with no MCP config of its own:
// the launch must pass --strict-mcp-config and point --mcp-config at the empty
// mcp.json so no project/user MCP server leaks in.
func TestBuildLaunch_ReadOnlyForcesStrictMCP(t *testing.T) {
	rm := baseRM() // read-only
	rm.MCP = domain.MCPConfig{Config: "", Strict: false}

	spec := BuildLaunch("/tmp/cfg", rm)

	require.Equal(t, []string{"CLAUDE_CONFIG_DIR=/tmp/cfg"}, spec.Env)
	require.Equal(t, []string{
		"claude",
		"--setting-sources", "user,project",
		"--strict-mcp-config",
		"--mcp-config", "/tmp/cfg/mcp.json",
		"--allowedTools", "Read,Grep,Glob",
		"--append-system-prompt", "@/tmp/cfg/CLAUDE.md",
	}, spec.Argv)
}

// I3 — a non-isolated persona (default mode, no MCP, not strict) omits MCP
// flags entirely.
func TestBuildLaunch_NoMCP(t *testing.T) {
	rm := baseRM()
	rm.Enforcement.PermissionMode = "default"
	rm.MCP = domain.MCPConfig{Config: "", Strict: false}

	spec := BuildLaunch("/tmp/cfg", rm)

	require.Equal(t, []string{"CLAUDE_CONFIG_DIR=/tmp/cfg"}, spec.Env)
	require.Equal(t, []string{
		"claude",
		"--setting-sources", "user,project",
		"--allowedTools", "Read,Grep,Glob",
		"--append-system-prompt", "@/tmp/cfg/CLAUDE.md",
	}, spec.Argv)
	require.NotContains(t, spec.Argv, "--mcp-config")
	require.NotContains(t, spec.Argv, "--strict-mcp-config")
}

// I4 — an empty SettingSrc must not be emitted as `--setting-sources ""`,
// which Claude Code would read as "consult every source" and defeat isolation.
// A sensible default is substituted instead.
func TestBuildLaunch_EmptySettingSrcDefaults(t *testing.T) {
	rm := baseRM()
	rm.SettingSrc = nil
	rm.MCP = domain.MCPConfig{Config: ""}

	spec := BuildLaunch("/tmp/cfg", rm)

	// --setting-sources is present and its value is non-empty.
	idx := -1
	for i, a := range spec.Argv {
		if a == "--setting-sources" {
			idx = i
			break
		}
	}
	require.GreaterOrEqual(t, idx, 0, "--setting-sources flag must be present")
	require.Less(t, idx+1, len(spec.Argv), "--setting-sources must have a value")
	require.NotEmpty(t, spec.Argv[idx+1], "setting-sources value must not be empty")
}

func TestBuildLaunch_WithMCP(t *testing.T) {
	rm := baseRM()
	rm.MCP = domain.MCPConfig{Config: "mcp.json", Strict: true}

	spec := BuildLaunch("/tmp/cfg", rm)

	require.Equal(t, []string{
		"claude",
		"--setting-sources", "user,project",
		"--strict-mcp-config",
		"--mcp-config", "/tmp/cfg/mcp.json",
		"--allowedTools", "Read,Grep,Glob",
		"--append-system-prompt", "@/tmp/cfg/CLAUDE.md",
	}, spec.Argv)
}
