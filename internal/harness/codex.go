package harness

import (
	"fmt"
	"path/filepath"

	toml "github.com/pelletier/go-toml/v2"
)

func init() { Register(codex{}) }

// codex targets the OpenAI Codex CLI. The whole config dir is relocated via the
// CODEX_HOME env var. Instructions live in AGENTS.md, settings + MCP in a single
// config.toml ([mcp_servers.<id>] inline tables), subagents are TOML files with a
// developer_instructions key, and skills use the cross-tool .agents/skills path —
// which lives OUTSIDE CODEX_HOME, so they are materialized with a report note.
type codex struct{}

func (codex) ID() string          { return "codex" }
func (codex) DisplayName() string { return "Codex CLI" }

func (codex) Materialize(req Request) (Report, error) {
	if err := cleanDest(req.DestDir); err != nil {
		return Report{}, err
	}
	rm := req.Manifest
	r := Report{Harness: "codex", Persona: rm.Persona.Name, Version: rm.Persona.Metadata.Version}

	// Instructions -> AGENTS.md
	if err := writeFile(filepath.Join(req.DestDir, "AGENTS.md"), []byte(composeInstructions(rm.ClaudeMD))); err != nil {
		return Report{}, err
	}
	r.add("instructions", "AGENTS.md", StatusOK)

	// MCP -> [mcp_servers.<id>] tables, approval/sandbox -> config.toml.
	mcp, err := readMCP(req)
	if err != nil {
		return Report{}, err
	}
	approval, sandbox := codexPermissions(rm.Enforcement.PermissionMode)
	cfg := map[string]any{"approval_policy": approval, "sandbox_mode": sandbox}
	if !mcp.empty() {
		cfg["mcp_servers"] = mcpCodex(mcp)
		r.add("mcp", fmt.Sprintf("config.toml [mcp_servers] (%d)", len(mcp.Names)), StatusTranslated)
	}
	data, err := toml.Marshal(cfg)
	if err != nil {
		return Report{}, fmt.Errorf("marshal config.toml: %w", err)
	}
	if err := writeFile(filepath.Join(req.DestDir, "config.toml"), data); err != nil {
		return Report{}, err
	}
	r.add("permissions", fmt.Sprintf("approval=%s sandbox=%s", approval, sandbox), StatusDegraded)
	r.drop("permissions", "allow/deny command globs have no Codex equivalent (use execpolicy for fine rules)")

	// Skills -> skills/ (note: Codex discovers skills under .agents/skills, outside CODEX_HOME).
	copied, missing, err := copySkillsInto(req, filepath.Join(req.DestDir, "skills"))
	if err != nil {
		return Report{}, err
	}
	if len(copied) > 0 {
		r.add("skills", fmt.Sprintf("%d -> skills/ (copy to .agents/skills to enable)", len(copied)), StatusTranslated)
	}
	noteMissingSkills(&r, missing)

	// Subagents -> agents/<name>.toml (MD body -> developer_instructions).
	if err := codexSubagents(req, &r); err != nil {
		return Report{}, err
	}

	r.Withheld = withheldSkills(req)
	return r, nil
}

func (codex) Launch(req Request) LaunchSpec {
	approval, sandbox := codexPermissions(req.Manifest.Enforcement.PermissionMode)
	return LaunchSpec{
		Env:  []string{"CODEX_HOME=" + req.DestDir},
		Argv: []string{"codex", "-a", approval, "-s", sandbox},
	}
}

func (codex) Detect() Detection {
	return probeHost("codex", "codex", "~/.codex", "~/.codex/config.toml")
}

// codexPermissions maps the persona permission mode onto Codex's orthogonal
// (approval_policy, sandbox_mode) pair. read-only is the strongest containment
// Codex offers without an execpolicy file.
func codexPermissions(mode string) (approval, sandbox string) {
	if mode == "read-only" {
		return "on-request", "read-only"
	}
	return "on-request", "workspace-write"
}

func mcpCodex(m mcpModel) map[string]any {
	servers := map[string]any{}
	for _, name := range m.Names {
		s := m.Servers[name]
		entry := map[string]any{}
		if s.remote() {
			entry["url"] = s.URL
		} else {
			entry["command"] = s.Command
			if len(s.Args) > 0 {
				entry["args"] = s.Args
			}
			if len(s.Env) > 0 {
				entry["env"] = s.Env
			}
		}
		servers[name] = entry
	}
	return servers
}

func codexSubagents(req Request, r *Report) error {
	if len(req.Manifest.Subagents) == 0 {
		return nil
	}
	for _, name := range req.Manifest.Subagents {
		sa, err := readSubagent(req, name)
		if err != nil {
			return err
		}
		spec := map[string]any{
			"name":                   sa.Name,
			"description":            sa.Description,
			"developer_instructions": sa.Body,
		}
		data, err := toml.Marshal(spec)
		if err != nil {
			return fmt.Errorf("marshal subagent %q: %w", name, err)
		}
		if err := writeFile(filepath.Join(req.DestDir, "agents", name+".toml"), data); err != nil {
			return err
		}
	}
	r.add("subagents", fmt.Sprintf("%d -> agents/*.toml (developer_instructions)", len(req.Manifest.Subagents)), StatusTranslated)
	return nil
}

// noteMissingSkills records skills the manifest promised but that are absent from
// the persona source tree (e.g. host-global skills referenced by the base layer).
func noteMissingSkills(r *Report, missing []string) {
	for _, name := range missing {
		r.drop("skills", "skill "+name+" not present in the persona tree (host-global?) — not exported")
	}
}
