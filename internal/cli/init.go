// internal/cli/init.go
package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/angerer/claude_git/internal/domain"
	"github.com/angerer/claude_git/internal/environment"
	"github.com/angerer/claude_git/internal/probe"
	"github.com/angerer/claude_git/internal/share"
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
					"Onboarded from %q into a new environment for %s.\nRun `claude_git list` to see available personas.\n",
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
	marker := filepath.Join(workspace, ".claude_git")
	if err := os.WriteFile(marker, []byte(env.Hash+"\n"), 0o644); err != nil {
		return fmt.Errorf("write marker: %w", err)
	}

	fmt.Fprintf(out, "Initialized claude_git environment for %s\n", workspace)
	fmt.Fprintf(out, "  hash:   %s\n", env.Hash)
	fmt.Fprintf(out, "  base:   _base persona seeded from existing .claude/ and CLAUDE.md\n")

	tracked, err := probe.IsClaudeTracked(workspace)
	if err != nil {
		return fmt.Errorf("probe workspace: %w", err)
	}
	if tracked {
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "WARNING: .claude/ is tracked by this workspace's code repository.")
		fmt.Fprintln(out, "  claude_git never writes into the workspace, so this is harmless to claude_git,")
		fmt.Fprintln(out, "  but you may want to untrack it (git rm -r --cached .claude) to avoid committing")
		fmt.Fprintln(out, "  agent config into your code history. claude_git will not touch your code repo.")
	}
	return nil
}

// importBase seeds the _base persona: copies the workspace .claude/ tree and
// CLAUDE.md into the persona dir and writes a minimal _base persona.toml.
// The author is taken from env.Author() which was derived at Create time.
func importBase(env *environment.Environment, workspace string) error {
	baseDir := filepath.Join(environment.RepoDir(env.Hash), "personas", "_base")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return fmt.Errorf("create _base dir: %w", err)
	}

	srcClaude := filepath.Join(workspace, ".claude")
	if info, err := os.Stat(srcClaude); err == nil && info.IsDir() {
		if err := copyTree(srcClaude, filepath.Join(baseDir, ".claude")); err != nil {
			return fmt.Errorf("import .claude: %w", err)
		}
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
		Extends:     "",
		Config: domain.Config{
			ClaudeMD:       "CLAUDE.md",
			SettingSources: []string{"user", "project"},
			Skills:         domain.SkillSet{Mode: "allowlist"},
		},
		Enforcement: domain.Enforcement{PermissionMode: "default"},
		Metadata:    domain.Metadata{Version: "0.1.0", Author: env.Author()},
	}
	return domain.SavePersonaTOML(base, filepath.Join(baseDir, "persona.toml"))
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
