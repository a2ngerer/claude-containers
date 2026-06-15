package enforce

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/angerer/claude_git/internal/compose"
	"github.com/angerer/claude_git/internal/domain"
)

// Verify asserts that destDir contains exactly the allowlisted skills/subagents
// from rm and that its settings.json carries every deny rule BuildPermissions
// would produce. It returns a domain.Attestation describing the activation. On
// any drift it returns domain.ErrVerifyMismatch (wrapped) and Clean=false.
func Verify(rm compose.ResolvedManifest, destDir string) (domain.Attestation, error) {
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

	// (1) skills present on disk must equal the allowlist exactly.
	gotSkills, err := listDir(filepath.Join(destDir, "skills"))
	if err != nil {
		return att, fmt.Errorf("read materialized skills: %w", err)
	}
	if diff := setDiff(rm.Skills, gotSkills); diff != "" {
		problems = append(problems, "skills "+diff)
	}

	// (2) subagents present on disk must equal the allowlist exactly.
	gotSubagents, err := listSubagents(filepath.Join(destDir, "agents"))
	if err != nil {
		return att, fmt.Errorf("read materialized agents: %w", err)
	}
	if diff := setDiff(rm.Subagents, gotSubagents); diff != "" {
		problems = append(problems, "subagents "+diff)
	}

	// (3) settings.json must contain every expected deny rule.
	gotDeny, err := readDeny(filepath.Join(destDir, "settings.json"))
	if err != nil {
		return att, fmt.Errorf("read materialized settings: %w", err)
	}
	if missing := missingFrom(ps.Deny, gotDeny); len(missing) > 0 {
		problems = append(problems, "missing deny rules: "+strings.Join(missing, ", "))
	}

	if len(problems) > 0 {
		return att, fmt.Errorf("%w: %s", domain.ErrVerifyMismatch, strings.Join(problems, "; "))
	}

	att.Clean = true
	return att, nil
}

// listDir returns the sorted names of immediate sub-entries of dir. A missing
// dir yields an empty slice (no skills/agents materialized).
func listDir(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names, nil
}

// listSubagents returns sorted subagent basenames (without the .md suffix) found
// in dir.
func listSubagents(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		names = append(names, strings.TrimSuffix(e.Name(), ".md"))
	}
	sort.Strings(names)
	return names, nil
}

// readDeny extracts permissions.deny from a settings.json file.
func readDeny(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sf struct {
		Permissions struct {
			Deny []string `json:"deny"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal(data, &sf); err != nil {
		return nil, fmt.Errorf("parse settings.json: %w", err)
	}
	return sf.Permissions.Deny, nil
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
