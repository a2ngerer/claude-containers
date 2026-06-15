// internal/domain/enforcement.go
package domain

type Enforcement struct {
	PermissionMode string   `toml:"permission_mode"` // "read-only" | "default"
	ToolsAllow     []string `toml:"-"`               // loaded from [enforcement.tools] allow
	ToolsDeny      []string `toml:"-"`               // loaded from [enforcement.tools] deny
}

// NOTE: tools.allow/deny live under [enforcement.tools]; loaders map them into these fields.
