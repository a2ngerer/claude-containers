package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/angerer/claude_git/internal/environment"
)

// newSnapshotCmd builds the `snapshot` command (alias `commit`).
func newSnapshotCmd(open envOpener) *cobra.Command {
	var msg string
	cmd := &cobra.Command{
		Use:     "snapshot [persona]",
		Aliases: []string{"commit"},
		Short:   "Record an immutable snapshot of a persona",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := open()
			if err != nil {
				return err
			}
			persona, err := resolveActivePersona(env, args)
			if err != nil {
				return err
			}
			id, err := takeSnapshot(env, persona, msg)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Snapshot %s recorded for %q\n", shortID(string(id)), persona)
			return nil
		},
	}
	cmd.Flags().StringVarP(&msg, "message", "m", "", "snapshot message")
	return cmd
}

// newLogCmd builds the `log` command.
func newLogCmd(open envOpener) *cobra.Command {
	return &cobra.Command{
		Use:   "log [persona]",
		Short: "Show a persona's snapshot history",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := open()
			if err != nil {
				return err
			}
			persona, err := resolveActivePersona(env, args)
			if err != nil {
				return err
			}
			out, err := formatTimeline(env, persona)
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), out)
			return nil
		},
	}
}

// Keep environment import used by later commands in this file.
var _ = (*environment.Environment)(nil)
