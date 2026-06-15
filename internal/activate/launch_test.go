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

func TestBuildLaunch_NoMCP(t *testing.T) {
	rm := baseRM()
	rm.MCP = domain.MCPConfig{Config: ""}

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
