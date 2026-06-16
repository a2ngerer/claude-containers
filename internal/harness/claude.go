package harness

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/a2ngerer/agent-containers/internal/domain"
	"github.com/a2ngerer/agent-containers/internal/enforce"
	"github.com/a2ngerer/agent-containers/internal/materialize"
)

func init() { Register(claude{}) }

// claude is the reference adapter. It is the canonical SOURCE harness: it reuses
// the existing materialize + drift-verify + launch path verbatim, so its output
// is byte-identical to the pre-multi-harness behaviour and stays the only target
// with an enforced, drift-verified attestation.
type claude struct{}

func (claude) ID() string          { return "claude" }
func (claude) DisplayName() string { return "Claude Code" }

func (claude) Materialize(req Request) (Report, error) {
	if err := materialize.Materialize(req.PersonaDir, req.Manifest, req.DestDir); err != nil {
		return Report{}, err
	}
	// Fail closed: a drift between the manifest and the materialized dir means the
	// environment cannot be trusted, so the error propagates and nothing launches.
	att, err := enforce.Verify(req.Manifest, req.PersonaDir, req.DestDir)
	if err != nil {
		return Report{}, err
	}
	return claudeReport(req, att), nil
}

func (claude) Launch(req Request) LaunchSpec {
	rm := req.Manifest
	allow := enforce.BuildPermissions(rm.Enforcement).Allow

	settingSrc := rm.SettingSrc
	if len(settingSrc) == 0 {
		// An empty --setting-sources would make Claude Code consult every source,
		// defeating isolation; project is the safe default.
		settingSrc = []string{"project"}
	}

	argv := []string{"claude", "--setting-sources", strings.Join(settingSrc, ",")}
	if rm.MCP.Config != "" || domain.MCPIsolated(rm.Enforcement, rm.MCP) {
		argv = append(argv,
			"--strict-mcp-config",
			"--mcp-config", filepath.Join(req.DestDir, "mcp.json"),
		)
	}
	argv = append(argv,
		"--allowedTools", strings.Join(allow, ","),
		"--append-system-prompt", "@"+filepath.Join(req.DestDir, "CLAUDE.md"),
	)
	return LaunchSpec{
		Env:  []string{"CLAUDE_CONFIG_DIR=" + req.DestDir},
		Argv: argv,
	}
}

func (claude) Detect() Detection {
	return probeHost("claude", "claude", "~/.claude", "~/.claude.json")
}

// claudeReport maps the strict attestation into the unified report shape.
func claudeReport(req Request, att domain.Attestation) Report {
	r := Report{
		Harness:  "claude",
		Persona:  att.Persona,
		Version:  att.Version,
		Verified: att.Clean,
		Denied:   att.Denied,
		Settings: att.SettingSrc,
	}
	for _, line := range att.Included {
		r.add(line.Kind, strings.Join(line.Names, ", "), StatusOK)
	}
	r.Withheld = withheldSkills(req)
	return r
}

// withheldSkills lists skills physically present in the persona source tree but
// excluded from the allowlist — the "deliberately removed" security narrative.
func withheldSkills(req Request) []string {
	entries, err := os.ReadDir(filepath.Join(req.PersonaDir, "skills"))
	if err != nil {
		return nil
	}
	allowed := make(map[string]bool, len(req.Manifest.Skills))
	for _, s := range req.Manifest.Skills {
		allowed[s] = true
	}
	var withheld []string
	for _, e := range entries {
		if e.IsDir() && !allowed[e.Name()] {
			withheld = append(withheld, e.Name())
		}
	}
	sort.Strings(withheld)
	return withheld
}
