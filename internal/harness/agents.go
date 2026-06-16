package harness

import "path/filepath"

func init() { Register(agentsmd{}) }

// agentsmd targets the cross-tool AGENTS.md standard (agentsmd/agents.md). It is
// not a runnable harness but an export convenience: it flattens the persona into
// a single AGENTS.md (instructions + skills/subagents as prose). MCP and
// permissions have no representation in the standard and are dropped — they must
// be materialized for a concrete harness instead.
type agentsmd struct{}

func (agentsmd) ID() string          { return "agents" }
func (agentsmd) DisplayName() string { return "AGENTS.md" }

func (agentsmd) Materialize(req Request) (Report, error) {
	if err := cleanDest(req.DestDir); err != nil {
		return Report{}, err
	}
	rm := req.Manifest
	r := Report{Harness: "agents", Persona: rm.Persona.Name, Version: rm.Persona.Metadata.Version}

	content := composeInstructions(rm.ClaudeMD, skillsProse(req), subagentsProse(req))
	if err := writeFile(filepath.Join(req.DestDir, "AGENTS.md"), []byte(content)); err != nil {
		return Report{}, err
	}
	r.add("instructions", "AGENTS.md", StatusOK)

	if len(rm.Skills) > 0 {
		r.add("skills", "folded into AGENTS.md prose", StatusDegraded)
		r.drop("skills", "AGENTS.md has no skill-dir concept — auto-invocation semantics lost")
	}
	if len(rm.Subagents) > 0 {
		r.add("subagents", "folded into AGENTS.md prose", StatusDegraded)
		r.drop("subagents", "AGENTS.md has no subagent concept — separate-context semantics lost")
	}
	if rm.MCP.Config != "" {
		r.drop("mcp", "AGENTS.md cannot carry MCP servers — materialize a concrete harness for MCP")
	}
	r.drop("permissions", "AGENTS.md is advisory prose, not enforceable config — permissions dropped")

	r.Withheld = withheldSkills(req)
	return r, nil
}

func (agentsmd) Launch(req Request) LaunchSpec {
	return LaunchSpec{
		Note: "AGENTS.md is a convention, not a runnable harness. Place the generated AGENTS.md at your project root; any AGENTS.md-aware tool launched there will read it.",
	}
}

func (agentsmd) Detect() Detection { return Detection{ID: "agents"} }
