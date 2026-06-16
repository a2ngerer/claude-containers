package harness

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/a2ngerer/agent-containers/internal/domain"
)

func init() { Register(opencode{}) }

// opencode targets SST OpenCode. OPENCODE_CONFIG_DIR relocates the global config
// dir; combined with the OPENCODE_DISABLE_* flags it isolates the persona from
// ambient project and ~/.claude config. Instructions are AGENTS.md, config is
// opencode.json (mcp + permission), subagents are agent/<name>.md.
type opencode struct{}

func (opencode) ID() string          { return "opencode" }
func (opencode) DisplayName() string { return "OpenCode" }

func (opencode) Materialize(req Request) (Report, error) {
	if err := cleanDest(req.DestDir); err != nil {
		return Report{}, err
	}
	rm := req.Manifest
	r := Report{Harness: "opencode", Persona: rm.Persona.Name, Version: rm.Persona.Metadata.Version}

	if err := writeFile(filepath.Join(req.DestDir, "AGENTS.md"), []byte(composeInstructions(rm.ClaudeMD))); err != nil {
		return Report{}, err
	}
	r.add("instructions", "AGENTS.md", StatusOK)

	cfg := map[string]any{
		"$schema":      "https://opencode.ai/config.json",
		"instructions": []any{"AGENTS.md"},
	}

	mcp, err := readMCP(req)
	if err != nil {
		return Report{}, err
	}
	if !mcp.empty() {
		cfg["mcp"] = mcpOpenCode(mcp)
		r.add("mcp", fmt.Sprintf(`"mcp" object (%d)`, len(mcp.Names)), StatusTranslated)
	}

	perm, denied := opencodePermission(rm.Enforcement)
	cfg["permission"] = perm
	r.Denied = denied
	if rm.Enforcement.PermissionMode == "read-only" {
		r.add("permissions", "read-only -> permission.edit=deny, bash denied", StatusTranslated)
	} else {
		r.add("permissions", "allow/deny -> permission map", StatusTranslated)
	}
	r.drop("permissions", "Claude permission_mode/sandbox nuance has no OpenCode analog")

	if err := writeJSON(filepath.Join(req.DestDir, "opencode.json"), cfg); err != nil {
		return Report{}, err
	}

	copied, missing, err := copySkillsInto(req, filepath.Join(req.DestDir, "skills"))
	if err != nil {
		return Report{}, err
	}
	if len(copied) > 0 {
		r.add("skills", fmt.Sprintf("%d -> skills/ (SKILL.md)", len(copied)), StatusOK)
	}
	noteMissingSkills(&r, missing)

	if err := opencodeSubagents(req, &r); err != nil {
		return Report{}, err
	}

	r.Withheld = withheldSkills(req)
	return r, nil
}

func (opencode) Launch(req Request) LaunchSpec {
	return LaunchSpec{
		Env: []string{
			// OPENCODE_CONFIG points at the explicit config FILE: OPENCODE_CONFIG_DIR
			// only registers a dir as a source for agent/command/plugin discovery and
			// does NOT load an opencode.json placed in it, so the config file must be
			// named here for mcp/permission/instructions to take effect.
			"OPENCODE_CONFIG=" + filepath.Join(req.DestDir, "opencode.json"),
			"OPENCODE_CONFIG_DIR=" + req.DestDir,
			"OPENCODE_DISABLE_PROJECT_CONFIG=1",
			"OPENCODE_DISABLE_CLAUDE_CODE_PROMPT=1",
			"OPENCODE_DISABLE_CLAUDE_CODE_SKILLS=1",
		},
		Argv: []string{"opencode"},
	}
}

func (opencode) Detect() Detection {
	return probeHost("opencode", "opencode", "~/.config/opencode", "~/.opencode")
}

// opencodePermission maps the persona enforcement onto OpenCode's permission
// object (per-tool allow/ask/deny + bash pattern map). Returns the denied tool
// list for the report.
func opencodePermission(enf domain.Enforcement) (map[string]any, []string) {
	readOnly := enf.PermissionMode == "read-only" || containsAny(enf.ToolsDeny, "Write", "Edit", "NotebookEdit")
	perm := map[string]any{}
	var denied []string
	if readOnly {
		perm["edit"] = "deny"
		denied = append(denied, "edit")
	} else {
		perm["edit"] = "allow"
	}

	bash := map[string]any{}
	for _, a := range enf.ToolsAllow {
		if p, ok := bashPattern(a); ok {
			bash[p] = "allow"
		}
	}
	for _, d := range enf.ToolsDeny {
		if p, ok := bashPattern(d); ok {
			bash[p] = "deny"
		}
	}
	switch {
	case readOnly:
		bash["*"] = "deny"
		perm["bash"] = bash
	case len(bash) > 0:
		perm["bash"] = bash
	default:
		perm["bash"] = "allow"
	}
	return perm, denied
}

func opencodeSubagents(req Request, r *Report) error {
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
		fmt.Fprintf(&b, "description: %s\n", yamlScalar(sa.Description))
		b.WriteString("mode: subagent\n")
		b.WriteString("---\n\n")
		b.WriteString(sa.Body)
		b.WriteString("\n")
		if err := writeFile(filepath.Join(req.DestDir, "agent", name+".md"), []byte(b.String())); err != nil {
			return err
		}
	}
	r.add("subagents", fmt.Sprintf("%d -> agent/*.md (mode: subagent)", len(req.Manifest.Subagents)), StatusTranslated)
	r.drop("subagents", "Claude model/tool-scoping frontmatter not translated (set per OpenCode provider/model)")
	return nil
}

// bashPattern converts a Claude "Bash(git diff:*)" rule into an OpenCode bash
// glob ("git diff *"). Returns false for non-Bash rules.
func bashPattern(rule string) (string, bool) {
	if !strings.HasPrefix(rule, "Bash(") || !strings.HasSuffix(rule, ")") {
		return "", false
	}
	inner := rule[len("Bash(") : len(rule)-1]
	inner = strings.ReplaceAll(inner, ":*", " *")
	inner = strings.ReplaceAll(inner, ":", " ")
	return strings.TrimSpace(inner), true
}

func containsAny(haystack []string, needles ...string) bool {
	set := make(map[string]bool, len(haystack))
	for _, h := range haystack {
		set[h] = true
	}
	for _, n := range needles {
		if set[n] {
			return true
		}
	}
	return false
}

// yamlScalar quotes a frontmatter value when it contains characters that would
// otherwise break a bare YAML scalar.
func yamlScalar(s string) string {
	if s == "" {
		return `""`
	}
	if strings.ContainsAny(s, ":#\"'\n") {
		return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
	}
	return s
}
