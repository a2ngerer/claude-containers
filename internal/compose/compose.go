// Package compose resolves a persona together with its extends-layer into a
// flat ResolvedManifest. It is pure with respect to domain types but reads
// persona content (CLAUDE.md) from the environment's repo.
package compose

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/a2ngerer/claude-containers/internal/domain"
	"github.com/a2ngerer/claude-containers/internal/environment"
)

// ResolvedManifest is the composed, effective configuration of a leaf persona.
type ResolvedManifest struct {
	Persona     domain.Persona // the leaf persona (post-merge effective values)
	Skills      []string       // resolved skill dir names to include
	Subagents   []string       // resolved subagent basenames
	ClaudeMD    string         // composed CLAUDE.md content (base + persona)
	SettingSrc  []string
	Enforcement domain.Enforcement
	MCP         domain.MCPConfig
}

// Compose resolves personaName against its extends layer and returns the
// effective manifest. Resolution order is base -> persona: scalars are taken
// from the persona layer; skills/subagents include-lists are unioned unless
// the persona sets Skills.Mode == "replace".
func Compose(e *environment.Environment, personaName string) (ResolvedManifest, error) {
	leaf, err := e.LoadPersona(personaName)
	if err != nil {
		return ResolvedManifest{}, fmt.Errorf("load persona %q: %w", personaName, err)
	}

	skills := append([]string(nil), leaf.Config.Skills.Include...)
	subagents := append([]string(nil), leaf.Config.Subagents.Include...)
	claudeMD, err := readClaudeMD(e, leaf)
	if err != nil {
		return ResolvedManifest{}, err
	}

	if leaf.Extends != "" {
		base, err := e.LoadPersona(leaf.Extends)
		if err != nil {
			return ResolvedManifest{}, fmt.Errorf("%w: %s", domain.ErrLayerNotFound, leaf.Extends)
		}
		if leaf.Config.Skills.Mode != "replace" {
			skills = union(base.Config.Skills.Include, skills)
		}
		subagents = union(base.Config.Subagents.Include, subagents)
		baseMD, err := readClaudeMD(e, base)
		if err != nil {
			return ResolvedManifest{}, err
		}
		switch {
		case baseMD == "":
			// claudeMD keeps its current value (may also be empty)
		case claudeMD == "":
			claudeMD = baseMD
		default:
			claudeMD = baseMD + "\n\n" + claudeMD
		}
	}

	return ResolvedManifest{
		Persona:     leaf,
		Skills:      skills,
		Subagents:   subagents,
		ClaudeMD:    claudeMD,
		SettingSrc:  leaf.Config.SettingSources,
		Enforcement: leaf.Enforcement,
		MCP:         leaf.Config.MCP,
	}, nil
}

// readClaudeMD reads the persona-local CLAUDE.md (Config.ClaudeMD path,
// default "CLAUDE.md"). A missing file yields empty content, not an error.
func readClaudeMD(e *environment.Environment, p domain.Persona) (string, error) {
	name := p.Config.ClaudeMD
	if name == "" {
		name = "CLAUDE.md"
	}
	path := filepath.Join(environment.RepoDir(e.Hash), "personas", p.Name, name)
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read CLAUDE.md for %q: %w", p.Name, err)
	}
	return string(b), nil
}

// union returns a + (b minus duplicates), preserving a's order then b's.
func union(a, b []string) []string {
	seen := make(map[string]struct{}, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, s := range append(append([]string(nil), a...), b...) {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
