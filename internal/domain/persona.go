// internal/domain/persona.go
package domain

import (
	"fmt"
	"os"

	toml "github.com/pelletier/go-toml/v2"
)

type Persona struct {
	Name        string      `toml:"name"`
	Description string      `toml:"description"`
	Extends     string      `toml:"extends"` // layer name, e.g. "_base"; "" = none
	Config      Config      `toml:"config"`
	Enforcement Enforcement `toml:"enforcement"`
	Metadata    Metadata    `toml:"metadata"`
}

type Config struct {
	ClaudeMD       string      `toml:"claude_md"`       // path relative to persona dir
	SettingSources []string    `toml:"setting_sources"` // subset of {"user","project","local"}
	Skills         SkillSet    `toml:"skills"`
	Subagents      SubagentSet `toml:"subagents"`
	MCP            MCPConfig   `toml:"mcp"`
}

type SkillSet struct {
	Mode    string   `toml:"mode"`    // "allowlist" (default) | "replace"
	Include []string `toml:"include"` // skill directory names
}

type SubagentSet struct {
	Include []string `toml:"include"` // subagent file basenames (without .md)
}

type MCPConfig struct {
	Config   string   `toml:"config"`   // path to persona-local mcp.json ("" = none)
	Strict   bool     `toml:"strict"`   // -> --strict-mcp-config
	Requires []string `toml:"requires"` // declared server names (sharing only; never configs)
}

type Metadata struct {
	Version string `toml:"version"` // semver, e.g. "1.2.0"
	Author  string `toml:"author"`
}

// IsLayer reports whether this is a composable layer (name starts with "_").
func (p Persona) IsLayer() bool { return len(p.Name) > 0 && p.Name[0] == '_' }

// personaWire is the on-disk shape. It mirrors Persona but replaces the
// Enforcement block with one that exposes [enforcement.tools] allow/deny, which
// Persona keeps in toml:"-" fields. Used only inside Load/SavePersonaTOML.
type personaWire struct {
	Name        string          `toml:"name"`
	Description string          `toml:"description"`
	Extends     string          `toml:"extends"`
	Config      Config          `toml:"config"`
	Enforcement enforcementWire `toml:"enforcement"`
	Metadata    Metadata        `toml:"metadata"`
}

type enforcementWire struct {
	PermissionMode string    `toml:"permission_mode"`
	Tools          toolsWire `toml:"tools"`
}

type toolsWire struct {
	Allow []string `toml:"allow"`
	Deny  []string `toml:"deny"`
}

// LoadPersonaTOML reads a persona.toml and maps [enforcement.tools] allow/deny
// into Enforcement.ToolsAllow/ToolsDeny.
func LoadPersonaTOML(path string) (Persona, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Persona{}, fmt.Errorf("read persona toml %q: %w", path, err)
	}
	var w personaWire
	if err := toml.Unmarshal(raw, &w); err != nil {
		return Persona{}, fmt.Errorf("unmarshal persona toml %q: %w", path, err)
	}
	return Persona{
		Name:        w.Name,
		Description: w.Description,
		Extends:     w.Extends,
		Config:      w.Config,
		Enforcement: Enforcement{
			PermissionMode: w.Enforcement.PermissionMode,
			ToolsAllow:     w.Enforcement.Tools.Allow,
			ToolsDeny:      w.Enforcement.Tools.Deny,
		},
		Metadata: w.Metadata,
	}, nil
}

// SavePersonaTOML writes a persona.toml, projecting Enforcement.ToolsAllow/ToolsDeny
// back under [enforcement.tools].
func SavePersonaTOML(p Persona, path string) error {
	w := personaWire{
		Name:        p.Name,
		Description: p.Description,
		Extends:     p.Extends,
		Config:      p.Config,
		Enforcement: enforcementWire{
			PermissionMode: p.Enforcement.PermissionMode,
			Tools: toolsWire{
				Allow: p.Enforcement.ToolsAllow,
				Deny:  p.Enforcement.ToolsDeny,
			},
		},
		Metadata: p.Metadata,
	}
	out, err := toml.Marshal(w)
	if err != nil {
		return fmt.Errorf("marshal persona toml: %w", err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return fmt.Errorf("write persona toml %q: %w", path, err)
	}
	return nil
}
