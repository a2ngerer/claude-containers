package cli

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/a2ngerer/agent-containers/internal/environment"
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
// M1 subcommands wired in. cmd/acon/main.go Execute()s the result.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "acon",
		Short:         "Version, swap, share, and port the agent config layer across harnesses",
		Long:          "acon treats CLAUDE.md plus the .claude/ directory as a versioned, swappable, shareable persona — \"Docker for your agents\" — and exports it to other harnesses (OpenCode, Codex, Gemini, Kimi, Antigravity).",
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

	// M5 multi-harness commands
	root.AddCommand(newConfigCmd(), newExportCmd(), newHarnessesCmd())

	return root
}
