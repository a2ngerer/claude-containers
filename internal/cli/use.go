package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/a2ngerer/agent-containers/internal/activate"
	"github.com/a2ngerer/agent-containers/internal/environment"
	"github.com/a2ngerer/agent-containers/internal/harness"
	"github.com/spf13/cobra"
)

// reservedSubcommands is the authoritative set of command names that must NOT
// be interpreted as a persona name by the dispatch shim. Keep in sync with
// all registered cobra commands.
var reservedSubcommands = map[string]bool{
	"init": true, "new": true, "use": true, "list": true, "ls": true,
	"status": true, "snapshot": true, "commit": true, "log": true,
	"diff": true, "rollback": true, "tag": true, "show": true,
	"verify": true, "edit": true, "rm": true, "deactivate": true,
	"push": true, "pull": true, "clone": true, "pull-persona": true,
	"config": true, "export": true, "harnesses": true,
	"help": true, "completion": true,
}

// isReserved reports whether name is a reserved subcommand.
func isReserved(name string) bool { return reservedSubcommands[name] }

// DispatchArgs implements the CLI dispatch rule: if the first argument is not a
// reserved subcommand and not a flag, it is treated as a persona name and the
// args are rewritten to "use <name>". All other inputs pass through unchanged.
func DispatchArgs(args []string) []string {
	if len(args) == 0 {
		return args
	}
	first := args[0]
	if strings.HasPrefix(first, "-") || isReserved(first) {
		return args
	}
	return append([]string{"use"}, args...)
}

// resolveHarness picks the target harness: an explicit flag wins, else the
// workspace default, else the reference harness. It validates the id against the
// registry so an unknown harness fails fast with the list of known ids.
func resolveHarness(e *environment.Environment, flagVal string) (string, error) {
	id := flagVal
	if id == "" {
		id = e.DefaultHarness()
	}
	if id == "" {
		id = harness.Default
	}
	if _, ok := harness.Get(id); !ok {
		return "", fmt.Errorf("unknown harness %q (known: %s)", id, strings.Join(harness.IDs(), ", "))
	}
	return id, nil
}

// newUseCmd builds `acon use <persona> [--harness id]`: compose, materialize for
// the target harness, report, and either print the launch command (default) or
// exec the harness directly (--exec).
func newUseCmd() *cobra.Command {
	var execDirect bool
	var harnessFlag string
	cmd := &cobra.Command{
		Use:   "use <persona>",
		Short: "Activate a persona for this workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := openCWD()
			if err != nil {
				return err
			}
			harnessID, err := resolveHarness(e, harnessFlag)
			if err != nil {
				return err
			}
			res, err := activate.Activate(e, args[0], harnessID)
			if err != nil {
				return err
			}
			printReport(cmd, res.Report)
			if execDirect {
				return execLaunch(res.Launch)
			}
			printLaunchHint(cmd, res.Launch)
			return nil
		},
	}
	cmd.Flags().BoolVar(&execDirect, "exec", false, "exec the harness directly instead of printing the command")
	cmd.Flags().StringVar(&harnessFlag, "harness", "", "target harness (default: workspace default, else claude)")
	return cmd
}

// newDeactivateCmd builds `acon deactivate`: clear the active persona.
func newDeactivateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "deactivate",
		Short: "Clear the active persona for this workspace",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := openCWD()
			if err != nil {
				return err
			}
			if err := e.SetActive(""); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Active persona cleared.")
			return nil
		},
	}
}

// printReport renders the unified translation/attestation certificate. For the
// Claude target it reads as the drift-verified "uncontaminated" attestation; for
// export targets it reads as an honest translation record with degraded/dropped
// artifacts called out explicitly.
func printReport(cmd *cobra.Command, r harness.Report) {
	out := cmd.OutOrStdout()
	state := "translated"
	if r.Verified {
		state = "uncontaminated"
	}
	fmt.Fprintf(out, "Persona: %s  (%s → %s)   %s:%s\n", r.Persona, state, r.Harness, r.Persona, r.Version)
	for _, line := range r.Lines {
		marker := ""
		switch line.Status {
		case harness.StatusTranslated:
			marker = "  ~translated"
		case harness.StatusDegraded:
			marker = "  !degraded"
		}
		fmt.Fprintf(out, "  %-13s %s%s\n", line.Kind+":", line.Detail, marker)
	}
	if len(r.Withheld) > 0 {
		fmt.Fprintf(out, "  Withheld  skills [%s]  (deliberately removed)\n", strings.Join(r.Withheld, ", "))
	}
	if len(r.Denied) > 0 {
		fmt.Fprintf(out, "  Denied:   %s\n", strings.Join(r.Denied, ", "))
	}
	if len(r.Settings) > 0 {
		fmt.Fprintf(out, "  Settings: %s\n", strings.Join(r.Settings, "+"))
	}
	for _, d := range r.Dropped {
		fmt.Fprintf(out, "  dropped   %s: %s\n", d.Kind, d.Reason)
	}
}

// printLaunchHint prints the env + command the user runs to start the harness,
// or, for a convention-only target with no binary, its placement note.
func printLaunchHint(cmd *cobra.Command, spec harness.LaunchSpec) {
	out := cmd.OutOrStdout()
	if len(spec.Argv) == 0 {
		if spec.Note != "" {
			fmt.Fprintf(out, "\n%s\n", spec.Note)
		}
		return
	}
	fmt.Fprintln(out, "\nTo use this environment, start (or restart) the harness with:")
	fmt.Fprintf(out, "  %s %s\n", strings.Join(spec.Env, " "), strings.Join(spec.Argv, " "))
	if spec.Note != "" {
		fmt.Fprintf(out, "Note: %s\n", spec.Note)
	}
}

// execLaunch replaces the current process with the harness binary (--exec path).
func execLaunch(spec harness.LaunchSpec) error {
	if len(spec.Argv) == 0 {
		return fmt.Errorf("this target is not directly launchable: %s", spec.Note)
	}
	bin := spec.Argv[0]
	path, err := exec.LookPath(bin)
	if err != nil {
		return fmt.Errorf("%s not found on PATH: %w", bin, err)
	}
	env := mergeEnv(os.Environ(), spec.Env)
	return syscall.Exec(path, spec.Argv, env)
}

// mergeEnv combines base and overrides so that keys present in overrides
// shadow any matching keys from base. Order within each slice is preserved.
func mergeEnv(base, overrides []string) []string {
	overrideKeys := map[string]bool{}
	for _, kv := range overrides {
		if i := strings.IndexByte(kv, '='); i >= 0 {
			overrideKeys[kv[:i]] = true
		}
	}
	out := make([]string, 0, len(base)+len(overrides))
	for _, kv := range base {
		if i := strings.IndexByte(kv, '='); i >= 0 && overrideKeys[kv[:i]] {
			continue
		}
		out = append(out, kv)
	}
	return append(out, overrides...)
}
