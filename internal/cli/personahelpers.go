package cli

import (
	"fmt"
	"os"
	"path/filepath"

	toml "github.com/pelletier/go-toml/v2"

	"github.com/angerer/claude_git/internal/domain"
	"github.com/angerer/claude_git/internal/environment"
)

// tomlPersona mirrors the [enforcement.tools] sub-table that domain.Enforcement
// keeps out of struct tags (ToolsAllow/ToolsDeny are toml:"-"). Used only for
// loading and encoding persona.toml against the domain type.
type tomlPersona struct {
	domain.Persona
	Enforcement struct {
		domain.Enforcement
		Tools struct {
			Allow []string `toml:"allow"`
			Deny  []string `toml:"deny"`
		} `toml:"tools"`
	} `toml:"enforcement"`
}

// parsePersonaTOML decodes a persona.toml body into a domain.Persona, mapping
// the [enforcement.tools] allow/deny lists into ToolsAllow/ToolsDeny.
func parsePersonaTOML(body []byte) (domain.Persona, error) {
	var tp tomlPersona
	if err := toml.Unmarshal(body, &tp); err != nil {
		return domain.Persona{}, fmt.Errorf("parse persona toml: %w", err)
	}
	p := tp.Persona
	p.Enforcement = tp.Enforcement.Enforcement
	p.Enforcement.ToolsAllow = tp.Enforcement.Tools.Allow
	p.Enforcement.ToolsDeny = tp.Enforcement.Tools.Deny
	return p, nil
}

// scaffoldPersona materializes a persona template into personas/<name>/ in the
// repo. The persona's name is forced to `name`. Refuses to overwrite.
func scaffoldPersona(e *environment.Environment, name string, sc personaScaffold) error {
	if _, err := e.LoadPersona(name); err == nil {
		return fmt.Errorf("%q: %w", name, domain.ErrPersonaExists)
	}
	p, err := parsePersonaTOML([]byte(sc.TOML))
	if err != nil {
		return err
	}
	p.Name = name
	if err := e.SavePersona(p); err != nil {
		return fmt.Errorf("save persona %q: %w", name, err)
	}
	dir := filepath.Join(environment.RepoDir(e.Hash), "personas", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir persona dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(sc.ClaudeMD), 0o644); err != nil {
		return fmt.Errorf("write CLAUDE.md: %w", err)
	}
	return nil
}
