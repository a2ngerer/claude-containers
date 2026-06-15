package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/a2ngerer/claude-containers/internal/compose"
	"github.com/a2ngerer/claude-containers/internal/domain"
	"github.com/a2ngerer/claude-containers/internal/environment"
)

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
	raw, err := domain.MarshalPersonaTOML(p)
	if err != nil {
		return personaScaffold{}, err
	}
	return personaScaffold{TOML: string(raw), ClaudeMD: md}, nil
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

// resolveActivePersona returns the persona name from args[0] if present,
// otherwise the environment's active persona.
func resolveActivePersona(e *environment.Environment, args []string) (string, error) {
	if len(args) > 0 && args[0] != "" {
		return args[0], nil
	}
	if a := e.ActivePersona(); a != "" {
		return a, nil
	}
	return "", fmt.Errorf("no persona given and no active persona set")
}

// takeSnapshot writes the persona dir as a tree and records a snapshot.
// It returns the new snapshot id.
func takeSnapshot(e *environment.Environment, persona, msg string) (domain.SnapshotID, error) {
	if _, err := e.LoadPersona(persona); err != nil {
		return "", err
	}
	if msg == "" {
		msg = "snapshot " + persona
	}
	dir := filepath.Join(environment.RepoDir(e.Hash), "personas", persona)
	tree, err := e.Store.WriteTree(dir)
	if err != nil {
		return "", fmt.Errorf("write tree: %w", err)
	}
	prev, _ := e.Store.Timeline(persona) // newest first; may be empty
	var parents []domain.SnapshotID
	if len(prev) > 0 {
		parents = []domain.SnapshotID{prev[0]}
	}
	snap := domain.Snapshot{
		Persona:   persona,
		Parents:   parents,
		Message:   msg,
		Author:    e.Author(),
		Timestamp: time.Now().UTC(),
		TreeID:    string(tree),
	}
	id, err := e.Store.WriteSnapshot(snap)
	if err != nil {
		return "", fmt.Errorf("write snapshot: %w", err)
	}
	return id, nil
}

// shortID truncates an id for display.
func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// formatTimeline renders a persona's snapshots newest-first, one line each.
func formatTimeline(e *environment.Environment, persona string) (string, error) {
	ids, err := e.Store.Timeline(persona)
	if err != nil {
		return "", fmt.Errorf("timeline for %q: %w", persona, err)
	}
	if len(ids) == 0 {
		return fmt.Sprintf("No snapshots for %q yet.\n", persona), nil
	}
	var b strings.Builder
	for _, id := range ids {
		s, err := e.Store.ReadSnapshot(id)
		if err != nil {
			return "", fmt.Errorf("read snapshot %s: %w", id, err)
		}
		fmt.Fprintf(&b, "%s  %s  %s\n", shortID(string(id)), s.Timestamp.UTC().Format(time.RFC3339), s.Message)
	}
	return b.String(), nil
}

// resolveManifestRef composes a persona ref into a ResolvedManifest. For M2 the
// ref is a bare persona name; ":version" / snapshot refs are resolved by the
// versioning commands directly and are not handled here.
func resolveManifestRef(e *environment.Environment, ref string) (compose.ResolvedManifest, error) {
	return compose.Compose(e, ref)
}

// formatCapabilityDiff renders a CapabilityDiff for the terminal.
func formatCapabilityDiff(d compose.CapabilityDiff) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Capability diff: %s  vs  %s\n", d.NameA, d.NameB)
	section := func(label string, onlyA, onlyB []string) {
		fmt.Fprintf(&b, "  %s\n", label)
		fmt.Fprintf(&b, "    only in %s: %s\n", d.NameA, joinOrNone(onlyA))
		fmt.Fprintf(&b, "    only in %s: %s\n", d.NameB, joinOrNone(onlyB))
	}
	section("Skills", d.SkillsOnlyA, d.SkillsOnlyB)
	section("Subagents", d.SubagentsOnlyA, d.SubagentsOnlyB)
	section("Tools allowed", d.AllowOnlyA, d.AllowOnlyB)
	section("Tools denied", d.DenyOnlyA, d.DenyOnlyB)
	return b.String()
}

// resolveSnapshotRef resolves ref (a short/full snapshot id or a version tag)
// to a concrete SnapshotID within a persona's history.
func resolveSnapshotRef(e *environment.Environment, persona, ref string) (domain.SnapshotID, error) {
	// try version tag first
	if id, err := e.Store.ResolveTag(persona, ref); err == nil {
		return id, nil
	}
	// fall back to (short) id match against the timeline
	ids, err := e.Store.Timeline(persona)
	if err != nil {
		return "", fmt.Errorf("timeline for %q: %w", persona, err)
	}
	for _, id := range ids {
		s := string(id)
		if s == ref || strings.HasPrefix(s, ref) {
			return id, nil
		}
	}
	return "", fmt.Errorf("snapshot or version %q not found for persona %q", ref, persona)
}

// scaffoldPersona materializes a persona template into personas/<name>/ in the
// repo. The persona's name is forced to `name`. Refuses to overwrite.
func scaffoldPersona(e *environment.Environment, name string, sc personaScaffold) error {
	if _, err := e.LoadPersona(name); err == nil {
		return fmt.Errorf("%q: %w", name, domain.ErrPersonaExists)
	}
	p, err := domain.ParsePersonaTOML([]byte(sc.TOML))
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
