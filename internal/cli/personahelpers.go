package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	toml "github.com/pelletier/go-toml/v2"

	"github.com/angerer/claude_git/internal/compose"
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

// copyPersonaScaffold builds a personaScaffold from an existing persona's
// saved manifest and its on-disk CLAUDE.md, for `new --from`.
func copyPersonaScaffold(e *environment.Environment, src string) (personaScaffold, error) {
	p, err := e.LoadPersona(src)
	if err != nil {
		return personaScaffold{}, fmt.Errorf("load source persona %q: %w", src, err)
	}
	md := ""
	name := p.Config.ClaudeMD
	if name == "" {
		name = "CLAUDE.md"
	}
	b, err := os.ReadFile(filepath.Join(environment.RepoDir(e.Hash), "personas", src, name))
	if err == nil {
		md = string(b)
	}
	body, err := encodePersonaTOML(p)
	if err != nil {
		return personaScaffold{}, err
	}
	return personaScaffold{TOML: body, ClaudeMD: md}, nil
}

// encodePersonaTOML serializes a domain.Persona to a persona.toml body,
// including the [enforcement.tools] sub-table.
func encodePersonaTOML(p domain.Persona) (string, error) {
	var tp tomlPersona
	tp.Persona = p
	tp.Enforcement.Enforcement = p.Enforcement
	tp.Enforcement.Tools.Allow = p.Enforcement.ToolsAllow
	tp.Enforcement.Tools.Deny = p.Enforcement.ToolsDeny
	b, err := toml.Marshal(tp)
	if err != nil {
		return "", fmt.Errorf("encode persona toml: %w", err)
	}
	return string(b), nil
}

// withheldBaseSkills returns base-layer skills that the composed manifest does
// NOT include — i.e. dropped by Skills.Mode == "replace". Empty otherwise.
func withheldBaseSkills(e *environment.Environment, rm compose.ResolvedManifest) []string {
	if rm.Persona.Extends == "" {
		return nil
	}
	base, err := e.LoadPersona(rm.Persona.Extends)
	if err != nil {
		return nil
	}
	active := make(map[string]struct{}, len(rm.Skills))
	for _, s := range rm.Skills {
		active[s] = struct{}{}
	}
	var withheld []string
	for _, s := range base.Config.Skills.Include {
		if _, ok := active[s]; !ok {
			withheld = append(withheld, s)
		}
	}
	sort.Strings(withheld)
	return withheld
}

// formatShow renders the composed manifest as a human capability preview.
func formatShow(e *environment.Environment, rm compose.ResolvedManifest) string {
	var b strings.Builder
	p := rm.Persona
	ver := p.Metadata.Version
	if ver == "" {
		ver = "0.0.0"
	}
	fmt.Fprintf(&b, "Persona: %s   %s:%s\n", p.Name, p.Name, ver)
	if p.Description != "" {
		fmt.Fprintf(&b, "  %s\n", p.Description)
	}
	fmt.Fprintf(&b, "  Skills:    %s\n", joinOrNone(rm.Skills))
	fmt.Fprintf(&b, "  Subagents: %s\n", joinOrNone(rm.Subagents))
	fmt.Fprintf(&b, "  Allow:     %s\n", joinOrNone(rm.Enforcement.ToolsAllow))
	fmt.Fprintf(&b, "  Denied:    %s\n", joinOrNone(rm.Enforcement.ToolsDeny))
	if wh := withheldBaseSkills(e, rm); len(wh) > 0 {
		fmt.Fprintf(&b, "  Withheld:  %s   (dropped by replace mode)\n", strings.Join(wh, ", "))
	}
	fmt.Fprintf(&b, "  Settings:  %s\n", joinOrNone(rm.SettingSrc))
	mode := rm.Enforcement.PermissionMode
	if mode == "" {
		mode = "default"
	}
	fmt.Fprintf(&b, "  Mode:      %s\n", mode)
	return b.String()
}

// joinOrNone formats a string slice as comma-separated or "(none)".
func joinOrNone(xs []string) string {
	if len(xs) == 0 {
		return "(none)"
	}
	return strings.Join(xs, ", ")
}

// personaTOMLPath returns the on-disk path of a persona's persona.toml.
func personaTOMLPath(e *environment.Environment, name string) string {
	return filepath.Join(environment.RepoDir(e.Hash), "personas", name, "persona.toml")
}

// removePersona deletes a persona directory from the repo. It refuses to remove
// the active persona.
func removePersona(e *environment.Environment, name string) error {
	if _, err := e.LoadPersona(name); err != nil {
		return err
	}
	if e.ActivePersona() == name {
		return fmt.Errorf("cannot remove %q: it is the active persona (run: claude_git deactivate)", name)
	}
	dir := filepath.Join(environment.RepoDir(e.Hash), "personas", name)
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove persona dir: %w", err)
	}
	return nil
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
