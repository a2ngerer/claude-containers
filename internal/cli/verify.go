package cli

import (
	"fmt"
	"path/filepath"

	"github.com/a2ngerer/agent-containers/internal/compose"
	"github.com/a2ngerer/agent-containers/internal/enforce"
	"github.com/a2ngerer/agent-containers/internal/environment"
	"github.com/a2ngerer/agent-containers/internal/materialize"
	"github.com/spf13/cobra"
)

// newVerifyCmd builds `acon verify <persona>`: re-materialize the persona
// into its cache config dir and assert isolation. Exits non-zero (RunE error) on
// any mismatch; the error message carries the diff produced by enforce.Verify.
func newVerifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "verify <persona>",
		Short: "Re-check that a persona's materialized environment matches its manifest",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := openCWD()
			if err != nil {
				return err
			}
			rm, err := compose.Compose(e, args[0])
			if err != nil {
				return err
			}
			configDir := environment.CacheDir(e.Hash, args[0])
			if err := materialize.Materialize(e, rm, configDir); err != nil {
				return fmt.Errorf("materialize %q: %w", args[0], err)
			}
			personaDir := filepath.Join(environment.RepoDir(e.Hash), "personas", rm.Persona.Name)
			att, err := enforce.Verify(rm, personaDir, configDir)
			if err != nil {
				// non-zero exit with the embedded diff
				return err
			}
			printAttestation(cmd, att)
			fmt.Fprintln(cmd.OutOrStdout(), "\nVerified: materialized environment matches the manifest.")
			return nil
		},
	}
}
