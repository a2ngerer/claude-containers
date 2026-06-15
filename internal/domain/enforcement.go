// internal/domain/enforcement.go
package domain

type Enforcement struct {
	PermissionMode string   `toml:"permission_mode"` // "read-only" | "default"
	ToolsAllow     []string `toml:"-"`               // loaded from [enforcement.tools] allow
	ToolsDeny      []string `toml:"-"`               // loaded from [enforcement.tools] deny
}

// NOTE: tools.allow/deny live under [enforcement.tools]; loaders map them into these fields.

// MCPIsolated reports whether the persona must be launched with a strict,
// empty MCP surface so no project- or user-level MCP server can leak into the
// isolated config dir. This is the single source of truth shared by
// Materialize (writes the empty mcp.json), BuildLaunch (emits
// --strict-mcp-config --mcp-config) and Verify (expects that mcp.json):
//
//   - a read-only persona is always isolated, and
//   - any persona that explicitly requests MCP.Strict.
//
// A persona that ships its own MCP config (mcp.Config != "") is handled
// separately and always gets the strict flags regardless of this predicate.
func MCPIsolated(enf Enforcement, mcp MCPConfig) bool {
	return enf.PermissionMode == "read-only" || mcp.Strict
}
