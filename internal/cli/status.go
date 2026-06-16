// internal/cli/status.go
package cli

import (
	"fmt"
	"io"

	"github.com/a2ngerer/agent-containers/internal/environment"
	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the active persona and bound workspace",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ws, err := resolveWorkspace(cmd)
			if err != nil {
				return err
			}
			return runStatus(cmd.OutOrStdout(), ws)
		},
	}
}

func runStatus(out io.Writer, workspace string) error {
	env, err := environment.Open(workspace)
	if err != nil {
		return err
	}
	active := env.ActivePersona()
	if active == "" {
		active = "none"
	}
	fmt.Fprintf(out, "workspace: %s\n", env.Workspace)
	fmt.Fprintf(out, "hash:      %s\n", env.Hash)
	fmt.Fprintf(out, "repo:      %s\n", environment.RepoDir(env.Hash))
	fmt.Fprintf(out, "active:    %s\n", active)
	return nil
}
