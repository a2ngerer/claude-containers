// internal/cli/list.go
package cli

import (
	"fmt"
	"io"

	"github.com/a2ngerer/claude-containers/internal/environment"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List personas in this workspace's environment",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ws, err := resolveWorkspace(cmd)
			if err != nil {
				return err
			}
			return runList(cmd.OutOrStdout(), ws)
		},
	}
}

func runList(out io.Writer, workspace string) error {
	env, err := environment.Open(workspace)
	if err != nil {
		return err
	}
	personas, err := env.ListPersonas()
	if err != nil {
		return fmt.Errorf("list personas: %w", err)
	}
	if len(personas) == 0 {
		fmt.Fprintln(out, "no personas yet")
		return nil
	}
	active := env.ActivePersona()
	for _, p := range personas {
		marker := " "
		if p.Name == active {
			marker = "*"
		}
		version := p.Metadata.Version
		if version == "" {
			version = "-"
		}
		fmt.Fprintf(out, "%s %-20s %s\n", marker, p.Name, version)
	}
	return nil
}
