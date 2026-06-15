package enforce

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/a2ngerer/claude-containers/internal/compose"
	"github.com/a2ngerer/claude-containers/internal/domain"
)

// Verify asserts that destDir contains EXACTLY the artefacts a faithful
// Materialize(rm) would have produced -- nothing more, nothing less. It is an
// allowlist verifier: it builds the complete set of expected relative paths and
// walks destDir; any file or directory not in that set, and any expected path
// missing from disk, is a domain.ErrVerifyMismatch. settings.json is checked for
// the exact allow set, every expected deny rule, and the expected permissionMode.
//
// personaDir is the persona's source tree in the repo
// (<repo>/personas/<name>); the allowlisted skill trees there define which
// files are legitimately part of each skill, so an injected file inside an
// otherwise-allowed skill is still caught. It returns a domain.Attestation; on
// any drift the error wraps domain.ErrVerifyMismatch and Clean=false.
func Verify(rm compose.ResolvedManifest, personaDir, destDir string) (domain.Attestation, error) {
	ps := BuildPermissions(rm.Enforcement)

	att := domain.Attestation{
		Persona:    rm.Persona.Name,
		Version:    rm.Persona.Metadata.Version,
		Denied:     ps.Deny,
		SettingSrc: rm.SettingSrc,
		Clean:      false,
	}
	if len(rm.Skills) > 0 {
		att.Included = append(att.Included, domain.AttestationLine{Kind: "skill", Names: rm.Skills})
	}
	if len(rm.Subagents) > 0 {
		att.Included = append(att.Included, domain.AttestationLine{Kind: "subagent", Names: rm.Subagents})
	}
	if rm.MCP.Config != "" {
		att.Included = append(att.Included, domain.AttestationLine{Kind: "mcp", Names: []string{rm.MCP.Config}})
	}

	var problems []string

	// (1) Build the whitelist of expected relative paths and diff it against the
	// actual destDir tree. This is the core fail-closed gate: anything on disk
	// that Materialize would not have written is rejected.
	expected, err := expectedPaths(rm, personaDir)
	if err != nil {
		return att, err
	}
	if diff := diffTree(expected, destDir); diff != "" {
		problems = append(problems, diff)
	}

	// (2) settings.json must carry the exact allow set, every expected deny rule,
	// and the expected permissionMode.
	if msgs := verifySettings(filepath.Join(destDir, "settings.json"), ps); len(msgs) > 0 {
		problems = append(problems, msgs...)
	}

	// (3) An isolated persona must declare its setting sources; an empty
	// SettingSrc would let every settings source leak in at launch.
	if domain.MCPIsolated(rm.Enforcement, rm.MCP) && len(rm.SettingSrc) == 0 {
		problems = append(problems, "empty setting sources for isolated persona")
	}

	if len(problems) > 0 {
		return att, fmt.Errorf("%w: %s", domain.ErrVerifyMismatch, strings.Join(problems, "; "))
	}

	att.Clean = true
	return att, nil
}

// expectedPaths returns the set of relative paths (files and directories) that a
// faithful Materialize(rm) writes into destDir. Skill file trees are read from
// personaDir so injected files inside an allowed skill are detectable.
func expectedPaths(rm compose.ResolvedManifest, personaDir string) (map[string]bool, error) {
	exp := map[string]bool{
		"CLAUDE.md":     true,
		"settings.json": true,
	}

	// Skills: the allowlisted skill directory plus every file/dir Materialize
	// would have copied from the persona source tree.
	if len(rm.Skills) > 0 {
		exp["skills"] = true
	}
	for _, name := range rm.Skills {
		if !filepath.IsLocal(name) {
			return nil, fmt.Errorf("%w: invalid skill name %q", domain.ErrVerifyMismatch, name)
		}
		rel := filepath.Join("skills", name)
		exp[rel] = true
		srcSkill := filepath.Join(personaDir, "skills", name)
		err := filepath.WalkDir(srcSkill, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			r, err := filepath.Rel(srcSkill, p)
			if err != nil {
				return err
			}
			if r == "." {
				return nil
			}
			exp[filepath.Join(rel, r)] = true
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("read expected skill tree %q: %w", name, err)
		}
	}

	// Subagents: agents/<name>.md, and the agents/ dir itself.
	if len(rm.Subagents) > 0 {
		exp["agents"] = true
	}
	for _, name := range rm.Subagents {
		if !filepath.IsLocal(name) {
			return nil, fmt.Errorf("%w: invalid subagent name %q", domain.ErrVerifyMismatch, name)
		}
		exp[filepath.Join("agents", name+".md")] = true
	}

	// mcp.json is expected iff Materialize would have written one: the persona
	// ships its own config, or it is MCP-isolated (read-only / Strict).
	if rm.MCP.Config != "" || domain.MCPIsolated(rm.Enforcement, rm.MCP) {
		exp["mcp.json"] = true
	}

	return exp, nil
}

// diffTree walks destDir and compares it against the expected path set. It
// returns "" when they match exactly, otherwise a description listing
// unexpected (smuggled) and missing entries.
func diffTree(expected map[string]bool, destDir string) string {
	seen := make(map[string]bool, len(expected))
	var unexpected []string

	err := filepath.WalkDir(destDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(destDir, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		// A symlink in the materialized dir is never expected; treat it as
		// unexpected and do not descend into it.
		if d.Type()&fs.ModeSymlink != 0 {
			unexpected = append(unexpected, rel+" (symlink)")
			return nil
		}
		if expected[rel] {
			seen[rel] = true
			return nil
		}
		unexpected = append(unexpected, rel)
		return nil
	})
	if err != nil {
		return "walk dest dir: " + err.Error()
	}

	var missing []string
	for e := range expected {
		if !seen[e] {
			missing = append(missing, e)
		}
	}

	if len(unexpected) == 0 && len(missing) == 0 {
		return ""
	}
	sort.Strings(unexpected)
	sort.Strings(missing)
	parts := []string{}
	if len(unexpected) > 0 {
		parts = append(parts, "unexpected paths: "+strings.Join(unexpected, ", "))
	}
	if len(missing) > 0 {
		parts = append(parts, "missing paths: "+strings.Join(missing, ", "))
	}
	return strings.Join(parts, "; ")
}

// settingsShape is the subset of settings.json that Verify enforces.
type settingsShape struct {
	Permissions struct {
		Allow []string `json:"allow"`
		Deny  []string `json:"deny"`
	} `json:"permissions"`
	PermissionMode string `json:"permissionMode"`
}

// verifySettings checks settings.json against the expected permission set:
// allow must match exactly (set equality), every expected deny rule must be
// present, and permissionMode must equal the expected mode.
func verifySettings(path string, ps PermissionSet) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return []string{"read settings.json: " + err.Error()}
	}
	var sf settingsShape
	if err := json.Unmarshal(data, &sf); err != nil {
		return []string{"parse settings.json: " + err.Error()}
	}

	var msgs []string
	if d := setDiff(ps.Allow, sf.Permissions.Allow); d != "" {
		msgs = append(msgs, "allow "+d)
	}
	if missing := missingFrom(ps.Deny, sf.Permissions.Deny); len(missing) > 0 {
		msgs = append(msgs, "missing deny rules: "+strings.Join(missing, ", "))
	}
	if sf.PermissionMode != ps.Mode {
		msgs = append(msgs, fmt.Sprintf("permissionMode %q != expected %q", sf.PermissionMode, ps.Mode))
	}
	return msgs
}

// setDiff returns "" if want and got contain the same elements (order-independent),
// otherwise a human-readable description of the discrepancy.
func setDiff(want, got []string) string {
	wantSet := toSet(want)
	gotSet := toSet(got)
	var extra, missing []string
	for g := range gotSet {
		if !wantSet[g] {
			extra = append(extra, g)
		}
	}
	for w := range wantSet {
		if !gotSet[w] {
			missing = append(missing, w)
		}
	}
	if len(extra) == 0 && len(missing) == 0 {
		return ""
	}
	sort.Strings(extra)
	sort.Strings(missing)
	parts := []string{}
	if len(extra) > 0 {
		parts = append(parts, "unexpected: "+strings.Join(extra, ", "))
	}
	if len(missing) > 0 {
		parts = append(parts, "missing: "+strings.Join(missing, ", "))
	}
	return strings.Join(parts, "; ")
}

// missingFrom returns the elements of want that are absent from got.
func missingFrom(want, got []string) []string {
	gotSet := toSet(got)
	var missing []string
	for _, w := range want {
		if !gotSet[w] {
			missing = append(missing, w)
		}
	}
	return missing
}

func toSet(s []string) map[string]bool {
	m := make(map[string]bool, len(s))
	for _, v := range s {
		m[v] = true
	}
	return m
}
