package harness

import (
	"fmt"
	"path/filepath"
	"strings"
)

func init() { Register(gemini{}) }

// gemini targets the Google Gemini CLI. GEMINI_CLI_HOME=<dir> makes <dir>/.gemini
// the config root, so everything is materialized under .gemini/. Instructions are
// GEMINI.md, MCP lives inside settings.json (server names must not contain
// underscores), subagents are .gemini/agents/*.md, skills are .gemini/skills/.
type gemini struct{}

func (gemini) ID() string          { return "gemini" }
func (gemini) DisplayName() string { return "Gemini CLI" }

func (gemini) Materialize(req Request) (Report, error) {
	if err := cleanDest(req.DestDir); err != nil {
		return Report{}, err
	}
	rm := req.Manifest
	r := Report{Harness: "gemini", Persona: rm.Persona.Name, Version: rm.Persona.Metadata.Version}
	geminiDir := filepath.Join(req.DestDir, ".gemini")

	if err := writeFile(filepath.Join(geminiDir, "GEMINI.md"), []byte(composeInstructions(rm.ClaudeMD))); err != nil {
		return Report{}, err
	}
	r.add("instructions", "GEMINI.md", StatusOK)

	settings := map[string]any{
		"context": map[string]any{"fileName": []any{"GEMINI.md"}},
	}

	mcp, err := readMCP(req)
	if err != nil {
		return Report{}, err
	}
	if !mcp.empty() {
		servers, renamed := mcpGemini(mcp)
		settings["mcpServers"] = servers
		r.add("mcp", fmt.Sprintf("settings.json mcpServers (%d)", len(mcp.Names)), StatusTranslated)
		for _, ren := range renamed {
			r.drop("mcp", fmt.Sprintf("server %q renamed to %q (Gemini forbids underscores in names)", ren[0], ren[1]))
		}
	}

	readOnly := rm.Enforcement.PermissionMode == "read-only"
	if readOnly {
		settings["tools"] = map[string]any{"exclude": []any{"write_file", "replace"}}
		r.add("permissions", "read-only -> tools.exclude [write_file, replace]", StatusDegraded)
		r.Denied = []string{"write_file", "replace"}
	} else {
		r.add("permissions", "default approval", StatusDegraded)
	}
	r.drop("permissions", "allow/deny globs and bypass mode are not expressible in settings.json (YOLO is CLI-only)")

	if err := writeJSON(filepath.Join(geminiDir, "settings.json"), settings); err != nil {
		return Report{}, err
	}

	copied, missing, err := copySkillsInto(req, filepath.Join(geminiDir, "skills"))
	if err != nil {
		return Report{}, err
	}
	if len(copied) > 0 {
		r.add("skills", fmt.Sprintf("%d -> .gemini/skills/ (SKILL.md)", len(copied)), StatusOK)
	}
	noteMissingSkills(&r, missing)

	if err := geminiSubagents(req, geminiDir, &r); err != nil {
		return Report{}, err
	}

	r.Withheld = withheldSkills(req)
	return r, nil
}

func (gemini) Launch(req Request) LaunchSpec {
	return LaunchSpec{
		Env:  []string{"GEMINI_CLI_HOME=" + req.DestDir, "GEMINI_CLI_TRUST_WORKSPACE=true"},
		Argv: []string{"gemini"},
	}
}

func (gemini) Detect() Detection {
	return probeHost("gemini", "gemini", "~/.gemini", "~/.gemini/settings.json")
}

func geminiSubagents(req Request, geminiDir string, r *Report) error {
	if len(req.Manifest.Subagents) == 0 {
		return nil
	}
	for _, name := range req.Manifest.Subagents {
		sa, err := readSubagent(req, name)
		if err != nil {
			return err
		}
		var b strings.Builder
		b.WriteString("---\n")
		fmt.Fprintf(&b, "name: %s\n", yamlScalar(sa.Name))
		fmt.Fprintf(&b, "description: %s\n", yamlScalar(sa.Description))
		b.WriteString("---\n\n")
		b.WriteString(sa.Body)
		b.WriteString("\n")
		if err := writeFile(filepath.Join(geminiDir, "agents", name+".md"), []byte(b.String())); err != nil {
			return err
		}
	}
	r.add("subagents", fmt.Sprintf("%d -> .gemini/agents/*.md", len(req.Manifest.Subagents)), StatusTranslated)
	return nil
}
