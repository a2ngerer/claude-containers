package cli

import (
	"fmt"
	"os"

	"github.com/angerer/claude_git/internal/share"
	"github.com/spf13/cobra"
)

// newPushCmd: `claude_git push [remote]` — secret-scan, then push (default "origin").
func newPushCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "push [remote]",
		Short: "Push the persona repo to a remote (aborts if secrets are detected)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			remote := remoteArg(args)
			e, err := openCWD()
			if err != nil {
				return err
			}
			if err := share.Push(e, remote); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Pushed persona repo to %q.\n", remote)
			return nil
		},
	}
}

// newPullCmd: `claude_git pull [remote]` — fetch + integrate remote changes.
func newPullCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pull [remote]",
		Short: "Pull persona repo changes from a remote",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			remote := remoteArg(args)
			e, err := openCWD()
			if err != nil {
				return err
			}
			if err := share.Pull(e, remote); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Pulled persona repo from %q.\n", remote)
			return nil
		},
	}
}

// newCloneCmd: `claude_git clone <remote>` — clone an existing persona repo into a
// new environment bound to the current workspace (team onboarding into an empty dir).
func newCloneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clone <remote>",
		Short: "Clone an existing persona repo into a new environment for this workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			wd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}
			env, err := share.Clone(args[0], wd)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(),
				"Cloned %q into a new environment for %s.\nRun `claude_git list` to see available personas.\n",
				args[0], env.Workspace)
			return nil
		},
	}
}

// remoteArg returns the remote name from optional args, defaulting to "origin".
func remoteArg(args []string) string {
	if len(args) == 1 {
		return args[0]
	}
	return "origin"
}
