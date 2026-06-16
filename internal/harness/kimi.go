package harness

import (
	"fmt"
	"path/filepath"
)

func init() { Register(kimi{}) }

// kimi targets Moonshot's Kimi Code CLI (the KIMI_CODE_HOME-based build, not the
// legacy ~/.kimi one). Instructions are AGENTS.md (injected via ${KIMI_AGENTS_MD}),
// MCP is a Claude-shaped mcp.json (1:1), skills are skills/<n>/SKILL.md. Kimi
// subagents are a YAML spec that does not map from Claude's Markdown, so they are
// folded into the AGENTS.md prose. Permissions are coarse launch flags.
type kimi struct{}

func (kimi) ID() string          { return "kimi" }
func (kimi) DisplayName() string { return "Kimi Code" }

func (kimi) Materialize(req Request) (Report, error) {
	if err := cleanDest(req.DestDir); err != nil {
		return Report{}, err
	}
	rm := req.Manifest
	r := Report{Harness: "kimi", Persona: rm.Persona.Name, Version: rm.Persona.Metadata.Version}

	// Instructions -> AGENTS.md, with subagent roles folded in as prose.
	instructions := composeInstructions(rm.ClaudeMD, subagentsProse(req))
	if err := writeFile(filepath.Join(req.DestDir, "AGENTS.md"), []byte(instructions)); err != nil {
		return Report{}, err
	}
	r.add("instructions", "AGENTS.md", StatusOK)

	mcp, err := readMCP(req)
	if err != nil {
		return Report{}, err
	}
	if !mcp.empty() {
		if err := writeJSON(filepath.Join(req.DestDir, "mcp.json"), mcpClaudeStyle(mcp)); err != nil {
			return Report{}, err
		}
		r.add("mcp", fmt.Sprintf("mcp.json mcpServers (%d)", len(mcp.Names)), StatusTranslated)
	}

	copied, missing, err := copySkillsInto(req, filepath.Join(req.DestDir, "skills"))
	if err != nil {
		return Report{}, err
	}
	if len(copied) > 0 {
		r.add("skills", fmt.Sprintf("%d -> skills/ (SKILL.md)", len(copied)), StatusOK)
	}
	noteMissingSkills(&r, missing)

	if len(rm.Subagents) > 0 {
		r.add("subagents", fmt.Sprintf("%d -> folded into AGENTS.md prose", len(rm.Subagents)), StatusDegraded)
		r.drop("subagents", "Kimi subagents are a YAML spec, not Claude Markdown — declarations dropped, roles kept as prose")
	}

	mode := kimiPermissionFlag(rm.Enforcement.PermissionMode)
	r.add("permissions", fmt.Sprintf("%s -> launch %s", rm.Enforcement.PermissionMode, mode), StatusDegraded)
	r.drop("permissions", "Kimi has only coarse modes (--plan/--auto/--yolo); allow/deny globs dropped")

	r.Withheld = withheldSkills(req)
	return r, nil
}

func (kimi) Launch(req Request) LaunchSpec {
	return LaunchSpec{
		Env: []string{"KIMI_CODE_HOME=" + req.DestDir},
		// -m takes a model ALIAS defined in the config [models] section; the default
		// is the namespaced "kimi-code/kimi-for-coding" (the bare "kimi-for-coding"
		// is the underlying API model name, not the resolvable alias).
		Argv: []string{"kimi", "-m", "kimi-code/kimi-for-coding", kimiPermissionFlag(req.Manifest.Enforcement.PermissionMode)},
	}
}

func (kimi) Detect() Detection {
	return probeHost("kimi", "kimi", "~/.kimi-code", "~/.kimi-code/config.toml")
}

// kimiPermissionFlag maps the persona mode onto Kimi's coarse launch flags:
// read-only -> --plan (read-only planning), everything else -> --auto.
func kimiPermissionFlag(mode string) string {
	if mode == "read-only" {
		return "--plan"
	}
	return "--auto"
}
