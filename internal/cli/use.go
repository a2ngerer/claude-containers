package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/a2ngerer/claude-containers/internal/activate"
	"github.com/a2ngerer/claude-containers/internal/domain"
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

// newUseCmd builds `claude_git use <persona>`: compose, materialize, attest, and
// either print the launch command (default) or exec claude directly (--exec).
func newUseCmd() *cobra.Command {
	var execDirect bool
	cmd := &cobra.Command{
		Use:   "use <persona>",
		Short: "Activate a persona for this workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := openCWD()
			if err != nil {
				return err
			}
			res, err := activate.Activate(e, args[0])
			if err != nil {
				return err
			}
			printAttestation(cmd, res.Attestation)
			if execDirect {
				return execLaunch(res.Launch)
			}
			printLaunchHint(cmd, res.Launch)
			return nil
		},
	}
	cmd.Flags().BoolVar(&execDirect, "exec", false, "exec claude directly instead of printing the command")
	return cmd
}

// newDeactivateCmd builds `claude_git deactivate`: clear the active persona.
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

// printAttestation renders the human-readable cleanliness certificate.
func printAttestation(cmd *cobra.Command, att domain.Attestation) {
	out := cmd.OutOrStdout()
	clean := "uncontaminated"
	if !att.Clean {
		clean = "UNVERIFIED"
	}
	fmt.Fprintf(out, "Persona: %s  (%s)   %s:%s\n", att.Persona, clean, att.Persona, att.Version)
	for _, line := range att.Included {
		fmt.Fprintf(out, "  %-9s %s\n", line.Kind+":", strings.Join(line.Names, ", "))
	}
	for _, line := range att.Withheld {
		fmt.Fprintf(out, "  Withheld  %s [%s]  (deliberately removed)\n", line.Kind, strings.Join(line.Names, ", "))
	}
	if len(att.Denied) > 0 {
		fmt.Fprintf(out, "  Denied:   %s\n", strings.Join(att.Denied, ", "))
	}
	if len(att.SettingSrc) > 0 {
		fmt.Fprintf(out, "  Settings: %s\n", strings.Join(att.SettingSrc, "+"))
	}
}

// printLaunchHint prints the env + command the user runs to start claude.
func printLaunchHint(cmd *cobra.Command, spec activate.LaunchSpec) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "\nTo use this environment, start (or restart) Claude Code with:")
	fmt.Fprintf(out, "  %s %s\n", strings.Join(spec.Env, " "), strings.Join(spec.Argv, " "))
	fmt.Fprintln(out, "-> Start (or restart) Claude Code in this directory to use this environment.")
}

// execLaunch replaces the current process with claude (--exec path).
func execLaunch(spec activate.LaunchSpec) error {
	path, err := exec.LookPath("claude")
	if err != nil {
		return fmt.Errorf("claude not found on PATH: %w", err)
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
