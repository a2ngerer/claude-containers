package cli

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/angerer/claude_git/internal/compose"
	"github.com/angerer/claude_git/internal/domain"
	"github.com/angerer/claude_git/internal/environment"
	"github.com/angerer/claude_git/internal/storage"
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

// domainObjectID converts a stored tree id string to a storage.ObjectID.
func domainObjectID(treeID string) storage.ObjectID { return storage.ObjectID(treeID) }

// newRollbackCmd builds the `rollback` command.
func newRollbackCmd(open envOpener) *cobra.Command {
	return &cobra.Command{
		Use:   "rollback <persona> <snapshot|version>",
		Short: "Restore a persona to a prior snapshot, recording a new snapshot",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := open()
			if err != nil {
				return err
			}
			persona, ref := args[0], args[1]
			target, err := resolveSnapshotRef(env, persona, ref)
			if err != nil {
				return err
			}
			snap, err := env.Store.ReadSnapshot(target)
			if err != nil {
				return err
			}
			dir := filepath.Join(environment.RepoDir(env.Hash), "personas", persona)
			if err := env.Store.CheckoutTree(domainObjectID(snap.TreeID), dir); err != nil {
				return fmt.Errorf("checkout tree: %w", err)
			}
			id, err := takeSnapshot(env, persona, "rollback to "+ref)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Rolled back %q to %s (new snapshot %s)\n", persona, ref, shortID(string(id)))
			return nil
		},
	}
}

// newTagCmd builds the `tag` command.
func newTagCmd(open envOpener) *cobra.Command {
	return &cobra.Command{
		Use:   "tag <persona> <version>",
		Short: "Tag a persona's newest snapshot with a SemVer version",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := open()
			if err != nil {
				return err
			}
			persona, version := args[0], args[1]
			ids, err := env.Store.Timeline(persona)
			if err != nil && !errors.Is(err, domain.ErrPersonaNotFound) {
				return fmt.Errorf("cannot read timeline for %q: %w", persona, err)
			}
			if len(ids) == 0 {
				return fmt.Errorf("cannot tag %q: no snapshots yet (run: claude_git snapshot %s)", persona, persona)
			}
			if err := env.Store.SetTag(persona, version, ids[0]); err != nil {
				return fmt.Errorf("set tag: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Tagged %q snapshot %s as %s:%s\n", persona, shortID(string(ids[0])), persona, version)
			return nil
		},
	}
}

