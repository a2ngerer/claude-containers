package harness

import (
	"fmt"
	"path/filepath"
)

func init() { Register(antigravity{}) }

// antigravity targets Google Antigravity's CLI (agy). It is the one harness with
// NO config-relocation env var: config is discovered by file placement under
// ~/.gemini and the project root. acon materializes a ~/.gemini-shaped tree plus
// a project AGENTS.md; isolation is best-effort via a HOME override at launch.
// Declarative subagents have no Antigravity file format and are folded to prose.
type antigravity struct{}

func (antigravity) ID() string          { return "antigravity" }
func (antigravity) DisplayName() string { return "Antigravity CLI" }

func (antigravity) Materialize(req Request) (Report, error) {
	if err := cleanDest(req.DestDir); err != nil {
		return Report{}, err
	}
	rm := req.Manifest
	r := Report{Harness: "antigravity", Persona: rm.Persona.Name, Version: rm.Persona.Metadata.Version}
	geminiDir := filepath.Join(req.DestDir, ".gemini")

	// Subagents have no file format; fold them into the instruction prose.
	instructions := composeInstructions(rm.ClaudeMD, subagentsProse(req))
	// Global instructions (loaded via HOME override) and project AGENTS.md.
	if err := writeFile(filepath.Join(geminiDir, "GEMINI.md"), []byte(instructions)); err != nil {
		return Report{}, err
	}
	if err := writeFile(filepath.Join(req.DestDir, "AGENTS.md"), []byte(instructions)); err != nil {
		return Report{}, err
	}
	r.add("instructions", "AGENTS.md + .gemini/GEMINI.md", StatusOK)

	mcp, err := readMCP(req)
	if err != nil {
		return Report{}, err
	}
	if !mcp.empty() {
		if err := writeJSON(filepath.Join(geminiDir, "config", "mcp_config.json"), mcpAntigravity(mcp)); err != nil {
			return Report{}, err
		}
		r.add("mcp", fmt.Sprintf(".gemini/config/mcp_config.json (%d)", len(mcp.Names)), StatusTranslated)
	}

	copied, missing, err := copySkillsInto(req, filepath.Join(geminiDir, "skills"))
	if err != nil {
		return Report{}, err
	}
	if len(copied) > 0 {
		r.add("skills", fmt.Sprintf("%d -> .gemini/skills/ (SKILL.md)", len(copied)), StatusOK)
	}
	noteMissingSkills(&r, missing)

	if len(rm.Subagents) > 0 {
		r.add("subagents", fmt.Sprintf("%d -> folded into instruction prose", len(rm.Subagents)), StatusDegraded)
		r.drop("subagents", "Antigravity subagents are runtime-spawned with no file format — declarations dropped")
	}

	r.add("permissions", "trust/per-tool only", StatusDegraded)
	r.drop("permissions", "Antigravity has no allow/deny grammar — permission_mode and rules dropped")

	r.Withheld = withheldSkills(req)
	return r, nil
}

func (antigravity) Launch(req Request) LaunchSpec {
	return LaunchSpec{
		// Antigravity has no config-dir env var; a HOME override is the only way to
		// retarget ~/.gemini for isolation. The project AGENTS.md still needs to sit
		// at the real project root.
		Env:  []string{"HOME=" + req.DestDir},
		Argv: []string{"agy"},
		Note: "Antigravity has no config-relocation env var. This uses a HOME override for best-effort isolation; alternatively copy AGENTS.md to your project root and the .gemini/ tree into ~/.gemini.",
	}
}

func (antigravity) Detect() Detection {
	return probeHost("antigravity", "agy", "~/.gemini/antigravity-cli")
}
