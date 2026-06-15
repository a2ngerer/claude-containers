package cli

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/a2ngerer/claude-containers/internal/environment"
)

// openCWD opens the environment bound to the current working directory.
// filepath.EvalSymlinks resolves macOS /var -> /private/var so the workspace
// hash matches what environment.Create recorded.
func openCWD() (*environment.Environment, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	wd, err = filepath.EvalSymlinks(wd)
	if err != nil {
		return nil, err
	}
	return environment.Open(wd)
}

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

	// M1 commands
	root.AddCommand(newInitCmd())
	root.AddCommand(newListCmd())
	root.AddCommand(newStatusCmd())

	// M2 persona commands
	root.AddCommand(newNewCmd(openCWD))
	root.AddCommand(newShowCmd(openCWD))
	root.AddCommand(newEditCmd(openCWD))
	root.AddCommand(newRmCmd(openCWD))

	// M2 versioning commands
	root.AddCommand(newSnapshotCmd(openCWD))
	root.AddCommand(newLogCmd(openCWD))
	root.AddCommand(newDiffCmd(openCWD))
	root.AddCommand(newRollbackCmd(openCWD))
	root.AddCommand(newTagCmd(openCWD))

	// M3 activation commands
	root.AddCommand(newUseCmd())
	root.AddCommand(newDeactivateCmd())
	root.AddCommand(newVerifyCmd())

	// M4 sharing commands
	root.AddCommand(newPushCmd(), newPullCmd(), newCloneCmd())

	return root
}
