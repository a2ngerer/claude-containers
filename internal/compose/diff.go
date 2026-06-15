package compose

import "sort"

// CapabilityDiff is the structural delta between two ResolvedManifests.
// It answers "what can A do that B cannot, and vice versa" — the auditable
// isolation guarantee the product sells. It is NOT a textual file diff.
type CapabilityDiff struct {
	NameA, NameB   string
	SkillsOnlyA    []string
	SkillsOnlyB    []string
	SubagentsOnlyA []string
	SubagentsOnlyB []string
	AllowOnlyA     []string
	AllowOnlyB     []string
	DenyOnlyA      []string
	DenyOnlyB      []string
}

// Diff computes the capability delta between manifests a and b.
func Diff(a, b ResolvedManifest) CapabilityDiff {
	return CapabilityDiff{
		NameA:          a.Persona.Name,
		NameB:          b.Persona.Name,
		SkillsOnlyA:    onlyIn(a.Skills, b.Skills),
		SkillsOnlyB:    onlyIn(b.Skills, a.Skills),
		SubagentsOnlyA: onlyIn(a.Subagents, b.Subagents),
		SubagentsOnlyB: onlyIn(b.Subagents, a.Subagents),
		AllowOnlyA:     onlyIn(a.Enforcement.ToolsAllow, b.Enforcement.ToolsAllow),
		AllowOnlyB:     onlyIn(b.Enforcement.ToolsAllow, a.Enforcement.ToolsAllow),
		DenyOnlyA:      onlyIn(a.Enforcement.ToolsDeny, b.Enforcement.ToolsDeny),
		DenyOnlyB:      onlyIn(b.Enforcement.ToolsDeny, a.Enforcement.ToolsDeny),
	}
}

// onlyIn returns the sorted set of elements present in a but not in b.
func onlyIn(a, b []string) []string {
	inB := make(map[string]struct{}, len(b))
	for _, s := range b {
		inB[s] = struct{}{}
	}
	var out []string
	for _, s := range a {
		if _, ok := inB[s]; !ok {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
