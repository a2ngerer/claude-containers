package cli

import (
	"fmt"
	"path/filepath"

	"github.com/a2ngerer/agent-containers/internal/activate"
	"github.com/spf13/cobra"
)

// newExportCmd builds `acon export <persona> --harness <id> [--out <dir>]`:
// materialize a persona's config for another harness into an explicit directory.
// This is the headline flow — take a setup authored for Claude Code and render it
// for OpenCode, Codex, Gemini, Kimi, Antigravity, or a plain AGENTS.md — and it
// prints an honest translation report of what crossed the boundary intact and
// what was lost.
func newExportCmd() *cobra.Command {
	var harnessFlag, outDir string
	cmd := &cobra.Command{
		Use:   "export <persona>",
		Short: "Materialize a persona's config for another harness (containerize and take it elsewhere)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := openCWD()
			if err != nil {
				return err
			}
			harnessID, err := resolveHarness(e, harnessFlag)
			if err != nil {
				return err
			}
			dest := outDir
			if dest == "" {
				dest = filepath.Join("acon-export", harnessID, args[0])
			}
			absDest, err := filepath.Abs(dest)
			if err != nil {
				return fmt.Errorf("resolve output dir %q: %w", dest, err)
			}

			report, launch, err := activate.Export(e, args[0], harnessID, absDest)
			if err != nil {
				return err
			}
			printReport(cmd, report)
			fmt.Fprintf(cmd.OutOrStdout(), "\nExported to %s\n", absDest)
			printLaunchHint(cmd, launch)
			return nil
		},
	}
	cmd.Flags().StringVar(&harnessFlag, "harness", "", "target harness (default: workspace default, else claude)")
	cmd.Flags().StringVar(&outDir, "out", "", "output directory (default: ./acon-export/<harness>/<persona>)")
	return cmd
}
