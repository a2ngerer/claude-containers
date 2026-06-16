// internal/cli/init.go
package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/a2ngerer/agent-containers/internal/domain"
	"github.com/a2ngerer/agent-containers/internal/environment"
	"github.com/a2ngerer/agent-containers/internal/harness"
	"github.com/a2ngerer/agent-containers/internal/probe"
	"github.com/a2ngerer/agent-containers/internal/share"
	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	var fromRemote string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Bind the current workspace and seed the _base persona (or clone with --from)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ws, err := resolveWorkspace(cmd)
			if err != nil {
				return err
			}
			// --from onboards a team's existing persona repo. It fully replaces the
			// local seed path: cloning brings the shared _base verbatim, and
			// re-seeding from this machine's .claude/ would pollute that baseline
			// (spec §11: seed _base OR clone, never both).
			if fromRemote != "" {
				env, err := share.Clone(fromRemote, ws)
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(),
					"Onboarded from %q into a new environment for %s.\nRun `acon list` to see available personas.\n",
					fromRemote, env.Workspace)
				return nil
			}
			return runInit(cmd.OutOrStdout(), ws)
		},
	}
	cmd.Flags().StringVar(&fromRemote, "from", "",
		"clone an existing persona repo from this git remote instead of seeding _base")
	return cmd
}

// resolveWorkspace reads the --workspace flag from the persistent root flags,
// defaulting to the process CWD.
func resolveWorkspace(cmd *cobra.Command) (string, error) {
	ws, _ := cmd.Root().PersistentFlags().GetString("workspace")
	if ws == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("determine working directory: %w", err)
		}
		ws = cwd
	}
	abs, err := filepath.Abs(ws)
	if err != nil {
		return "", fmt.Errorf("resolve workspace %q: %w", ws, err)
	}
	// EvalSymlinks resolves macOS /var -> /private/var so the workspace hash
	// matches the one computed by openCWD (which also calls EvalSymlinks).
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolve symlinks for workspace %q: %w", ws, err)
	}
	return filepath.Clean(resolved), nil
}

func runInit(out io.Writer, workspace string) error {
	env, err := environment.Create(workspace)
	if err != nil {
		return fmt.Errorf("create environment: %w", err)
	}

	if err := importBase(env, workspace); err != nil {
		return err
	}

	// marker file: one line = workspace hash
	marker := filepath.Join(workspace, ".acon")
	if err := os.WriteFile(marker, []byte(env.Hash+"\n"), 0o644); err != nil {
		return fmt.Errorf("write marker: %w", err)
	}

	fmt.Fprintf(out, "Initialized acon environment for %s\n", workspace)
	fmt.Fprintf(out, "  hash:   %s\n", env.Hash)
	fmt.Fprintf(out, "  base:   _base persona seeded from existing .claude/ and CLAUDE.md\n")

	// Auto-detect installed harnesses and seed the workspace default so `acon use`
	// targets the right one without a flag. The persona source stays Claude's
	// .claude/ + CLAUDE.md; the default only controls the export target.
	if err := seedDefaultHarness(out, env); err != nil {
		return err
	}

	tracked, err := probe.IsClaudeTracked(workspace)
	if err != nil {
		return fmt.Errorf("probe workspace: %w", err)
	}
	if tracked {
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "WARNING: .claude/ is tracked by this workspace's code repository.")
		fmt.Fprintln(out, "  acon never writes into the workspace, so this is harmless to acon,")
		fmt.Fprintln(out, "  but you may want to untrack it (git rm -r --cached .claude) to avoid committing")
		fmt.Fprintln(out, "  agent config into your code history. acon will not touch your code repo.")
	}
	return nil
}

// seedDefaultHarness detects installed harness binaries, reports them, and sets
// the workspace default. It prefers the reference harness (claude) when present,
// otherwise the first detected; if nothing is detected the implicit claude
// default stands and no value is written.
func seedDefaultHarness(out io.Writer, env *environment.Environment) error {
	var detected []string
	for _, h := range harness.All() {
		if h.Detect().Installed {
			detected = append(detected, h.ID())
		}
	}
	if len(detected) > 0 {
		fmt.Fprintf(out, "  detected: %s\n", strings.Join(detected, ", "))
	}

	def := ""
	for _, id := range detected {
		if id == harness.Default {
			def = harness.Default
			break
		}
	}
	if def == "" && len(detected) > 0 {
		def = detected[0]
	}

	if def != "" && def != harness.Default {
		if err := env.SetDefaultHarness(def); err != nil {
			return err
		}
		fmt.Fprintf(out, "  harness: %s (set as default; change with `acon config harness <id>`)\n", def)
	} else {
		fmt.Fprintf(out, "  harness: %s (default; change with `acon config harness <id>`)\n", harness.Default)
	}
	return nil
}

// importBase seeds the _base persona from the workspace .claude/ + CLAUDE.md. It
// materializes the skills, subagents, and MCP config into the persona store AND
// enumerates them in the manifest, so the baseline is a full, materializable, and
// exportable persona — the foundation of "containerize my whole Claude setup."
// The author is taken from env.Author() which was derived at Create time.
func importBase(env *environment.Environment, workspace string) error {
	baseDir := filepath.Join(environment.RepoDir(env.Hash), "personas", "_base")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return fmt.Errorf("create _base dir: %w", err)
	}
	srcClaude := filepath.Join(workspace, ".claude")

	cfg := domain.Config{
		ClaudeMD:       "CLAUDE.md",
		SettingSources: []string{"user", "project"},
		Skills:         domain.SkillSet{Mode: "allowlist"},
	}

	skills, err := importSkills(srcClaude, baseDir)
	if err != nil {
		return err
	}
	cfg.Skills.Include = skills

	subs, err := importSubagents(srcClaude, baseDir)
	if err != nil {
		return err
	}
	cfg.Subagents.Include = subs

	hasMCP, err := importMCP(workspace, srcClaude, baseDir)
	if err != nil {
		return err
	}
	if hasMCP {
		cfg.MCP.Config = "mcp.json"
	}

	srcMD := filepath.Join(workspace, "CLAUDE.md")
	if _, err := os.Stat(srcMD); err == nil {
		if err := copyFile(srcMD, filepath.Join(baseDir, "CLAUDE.md")); err != nil {
			return fmt.Errorf("import CLAUDE.md: %w", err)
		}
	}

	base := domain.Persona{
		Name:        "_base",
		Description: "Shared base layer imported from the workspace .claude/ and CLAUDE.md.",
		Config:      cfg,
		Enforcement: domain.Enforcement{PermissionMode: "default"},
		Metadata:    domain.Metadata{Version: "0.1.0", Author: env.Author()},
	}
	return domain.SavePersonaTOML(base, filepath.Join(baseDir, "persona.toml"))
}

// importSkills copies each .claude/skills/<name>/ dir into the persona store and
// returns the sorted skill names for the manifest allowlist.
func importSkills(srcClaude, baseDir string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(srcClaude, "skills"))
	if err != nil {
		return nil, nil // no skills/ -> none
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if err := copyTree(filepath.Join(srcClaude, "skills", e.Name()), filepath.Join(baseDir, "skills", e.Name())); err != nil {
			return nil, fmt.Errorf("import skill %q: %w", e.Name(), err)
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names, nil
}

// importSubagents copies each .claude/agents/<name>.md into the persona store and
// returns the sorted subagent basenames for the manifest.
func importSubagents(srcClaude, baseDir string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(srcClaude, "agents"))
	if err != nil {
		return nil, nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		if err := copyFile(filepath.Join(srcClaude, "agents", e.Name()), filepath.Join(baseDir, "agents", e.Name())); err != nil {
			return nil, fmt.Errorf("import subagent %q: %w", e.Name(), err)
		}
		names = append(names, strings.TrimSuffix(e.Name(), ".md"))
	}
	sort.Strings(names)
	return names, nil
}

// importMCP copies the first MCP config it finds (workspace .mcp.json, then the
// .claude/ variants) into the persona store as mcp.json. Returns whether one was
// imported.
func importMCP(workspace, srcClaude, baseDir string) (bool, error) {
	for _, cand := range []string{
		filepath.Join(workspace, ".mcp.json"),
		filepath.Join(srcClaude, ".mcp.json"),
		filepath.Join(srcClaude, "mcp.json"),
	} {
		if _, err := os.Stat(cand); err == nil {
			if err := copyFile(cand, filepath.Join(baseDir, "mcp.json")); err != nil {
				return false, fmt.Errorf("import mcp config: %w", err)
			}
			return true, nil
		}
	}
	return false, nil
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) (err error) {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := out.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}
