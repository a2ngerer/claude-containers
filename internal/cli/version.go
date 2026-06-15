package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/angerer/claude_git/internal/compose"
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

// newDiffCmd builds the `diff` command (capability diff).
func newDiffCmd(open envOpener) *cobra.Command {
	return &cobra.Command{
		Use:   "diff [a] [b]",
		Short: "Capability diff between two personas",
		Args:  cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := open()
			if err != nil {
				return err
			}
			var refA, refB string
			switch len(args) {
			case 2:
				refA, refB = args[0], args[1]
			case 1:
				active := env.ActivePersona()
				if active == "" {
					return fmt.Errorf("one argument given but no active persona to compare against")
				}
				refA, refB = active, args[0]
			default:
				return fmt.Errorf("diff requires two persona names (or one, compared to the active persona)")
			}
			a, err := resolveManifestRef(env, refA)
			if err != nil {
				return err
			}
			b, err := resolveManifestRef(env, refB)
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), formatCapabilityDiff(compose.Diff(a, b)))
			return nil
		},
	}
}

