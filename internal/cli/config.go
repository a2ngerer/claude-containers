package cli

import (
	"fmt"
	"strings"

	"github.com/a2ngerer/agent-containers/internal/harness"
	"github.com/spf13/cobra"
)

// newConfigCmd builds `acon config`: show workspace settings, with a `harness`
// subcommand to get or set the default target harness.
func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show or change workspace settings (e.g. the default harness)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := openCWD()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			dh := e.DefaultHarness()
			if dh == "" {
				dh = harness.Default + "  (default)"
			}
			ap := e.ActivePersona()
			if ap == "" {
				ap = "(none)"
			}
			fmt.Fprintf(out, "workspace:        %s\n", e.Workspace)
			fmt.Fprintf(out, "default harness:  %s\n", dh)
			fmt.Fprintf(out, "active persona:   %s\n", ap)
			return nil
		},
	}
	cmd.AddCommand(newConfigHarnessCmd())
	return cmd
}

// newConfigHarnessCmd builds `acon config harness [id]`: print the workspace
// default harness, or set it.
func newConfigHarnessCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "harness [id]",
		Short: "Get or set the default target harness for this workspace",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := openCWD()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(args) == 0 {
				cur := e.DefaultHarness()
				if cur == "" {
					cur = harness.Default + "  (default)"
				}
				fmt.Fprintf(out, "default harness: %s\n", cur)
				fmt.Fprintf(out, "available:       %s\n", strings.Join(harness.IDs(), ", "))
				return nil
			}
			id := args[0]
			if _, ok := harness.Get(id); !ok {
				return fmt.Errorf("unknown harness %q (known: %s)", id, strings.Join(harness.IDs(), ", "))
			}
			if err := e.SetDefaultHarness(id); err != nil {
				return err
			}
			fmt.Fprintf(out, "default harness set to %q for this workspace.\n", id)
			return nil
		},
	}
}

// newHarnessesCmd builds `acon harnesses`: list every supported harness and
// whether its binary is installed on this host. It needs no initialized
// workspace, so it works anywhere.
func newHarnessesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "harnesses",
		Short: "List supported harnesses and which are installed on this machine",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			for _, h := range harness.All() {
				d := h.Detect()
				status := "not found"
				switch {
				case d.Installed:
					status = "installed"
				case d.Configured:
					status = "configured"
				}
				marker := " "
				if h.ID() == harness.Default {
					marker = "*"
				}
				fmt.Fprintf(out, "%s %-12s %-18s %s\n", marker, h.ID(), h.DisplayName(), status)
			}
			fmt.Fprintln(out, "\n* = reference source harness")
			return nil
		},
	}
}
