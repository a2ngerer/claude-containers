package cli

import (
	"github.com/spf13/cobra"
)

// NewRootCmd builds the cobra root command with all global flags and the
// M1 subcommands wired in. cmd/claude_git/main.go Execute()s the result.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "claude_git",
		Short:         "Version control and isolated, swappable environments for the Claude Code config layer",
		Long:          "claude_git treats CLAUDE.md plus the .claude/ directory as a versioned, swappable, shareable persona — \"Docker for Claude agent environments.\"",
		SilenceUsage:  true,
		SilenceErrors: false,
	}

	// Global flags shared by all subcommands. The workspace defaults to the
	// process working directory; tests and power users override it.
	root.PersistentFlags().String("workspace", "", "workspace path (default: current directory)")

	root.AddCommand(newInitCmd())
	root.AddCommand(newListCmd())
	root.AddCommand(newStatusCmd())

	return root
}
