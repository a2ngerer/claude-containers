package materialize

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/angerer/claude_git/internal/compose"
	"github.com/angerer/claude_git/internal/enforce"
	"github.com/angerer/claude_git/internal/environment"
)

// Materialize renders rm into destDir (a CLAUDE_CONFIG_DIR outside the
// workspace). It copies only the allowlisted skills/subagents from the persona
// repo, writes the composed CLAUDE.md, the enforcement settings.json, and
// reconciles mcp.json. It is idempotent: running it twice yields a byte-identical
// destDir (clean-then-build).
func Materialize(e *environment.Environment, rm compose.ResolvedManifest, destDir string) error {
	if err := os.RemoveAll(destDir); err != nil {
		return fmt.Errorf("clean dest dir: %w", err)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}

	personaDir := filepath.Join(environment.RepoDir(e.Hash), "personas", rm.Persona.Name)

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

	if err := writeMCP(destDir, rm.MCP); err != nil {
		return err
	}

	return nil
}

// copySkills copies each allowlisted skill directory from <personaDir>/skills/<name>
// to <destDir>/skills/<name>. A missing source skill is an error (the manifest
// promised it).
func copySkills(personaDir, destDir string, skills []string) error {
	for _, name := range skills {
		src := filepath.Join(personaDir, "skills", name)
		dst := filepath.Join(destDir, "skills", name)
		if err := copyTree(src, dst); err != nil {
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
		src := filepath.Join(personaDir, "agents", name+".md")
		dst := filepath.Join(destDir, "agents", name+".md")
		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("copy subagent %q: %w", name, err)
		}
	}
	return nil
}

// copyTree recursively copies the directory at src into dst, preserving the
// relative structure. File mode is normalized (0644 files, 0755 dirs) so two
// materializations are byte/metadata identical.
func copyTree(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(p, target)
	})
}

// copyFile copies a single regular file, creating parent dirs as needed and
// normalizing the mode to 0644.
func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
