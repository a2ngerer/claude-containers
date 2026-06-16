package materialize

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/a2ngerer/agent-containers/internal/compose"
	"github.com/a2ngerer/agent-containers/internal/enforce"
	"github.com/a2ngerer/agent-containers/internal/safecopy"
)

// Materialize renders rm into destDir (a CLAUDE_CONFIG_DIR outside the
// workspace). personaDir is the persona's source tree in the repo
// (<repo>/personas/<name>). It copies only the allowlisted skills/subagents from
// that tree, writes the composed CLAUDE.md, the enforcement settings.json, and
// reconciles mcp.json. It is idempotent: running it twice yields a byte-identical
// destDir (clean-then-build).
func Materialize(personaDir string, rm compose.ResolvedManifest, destDir string) error {
	if err := os.RemoveAll(destDir); err != nil {
		return fmt.Errorf("clean dest dir: %w", err)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}

	if err := copySkills(personaDir, destDir, rm.Skills); err != nil {
		return err
	}
	if err := copySubagents(personaDir, destDir, rm.Subagents); err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(destDir, "CLAUDE.md"), []byte(rm.ClaudeMD), 0o644); err != nil {
		return fmt.Errorf("write CLAUDE.md: %w", err)
	}

	ps := enforce.BuildPermissions(rm.Enforcement)
	if err := writeSettings(destDir, ps); err != nil {
		return err
	}

	if err := writeMCP(destDir, rm.Enforcement, rm.MCP); err != nil {
		return err
	}

	return nil
}

// copySkills copies each allowlisted skill directory from <personaDir>/skills/<name>
// to <destDir>/skills/<name>. A missing source skill is an error (the manifest
// promised it).
func copySkills(personaDir, destDir string, skills []string) error {
	for _, name := range skills {
		if !filepath.IsLocal(name) {
			return fmt.Errorf("invalid persona component name: %q", name)
		}
		src := filepath.Join(personaDir, "skills", name)
		dst := filepath.Join(destDir, "skills", name)
		if err := safecopy.Tree(src, dst); err != nil {
			return fmt.Errorf("copy skill %q: %w", name, err)
		}
	}
	return nil
}

// copySubagents copies each allowlisted subagent file <personaDir>/agents/<name>.md
// to <destDir>/agents/<name>.md.
func copySubagents(personaDir, destDir string, subagents []string) error {
	if len(subagents) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Join(destDir, "agents"), 0o755); err != nil {
		return fmt.Errorf("create agents dir: %w", err)
	}
	for _, name := range subagents {
		if !filepath.IsLocal(name) {
			return fmt.Errorf("invalid persona component name: %q", name)
		}
		src := filepath.Join(personaDir, "agents", name+".md")
		dst := filepath.Join(destDir, "agents", name+".md")
		if err := safecopy.File(src, dst); err != nil {
			return fmt.Errorf("copy subagent %q: %w", name, err)
		}
	}
	return nil
}
