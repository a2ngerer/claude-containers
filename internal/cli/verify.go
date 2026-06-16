package cli

import (
	"fmt"
	"path/filepath"

	"github.com/a2ngerer/agent-containers/internal/compose"
	"github.com/a2ngerer/agent-containers/internal/environment"
	"github.com/a2ngerer/agent-containers/internal/harness"
	"github.com/spf13/cobra"
)

// newVerifyCmd builds `acon verify <persona>`: re-materialize the persona into
// its Claude cache config dir and assert isolation via the drift-verifying
// Claude adapter. Exits non-zero (RunE error) on any mismatch; the error message
// carries the diff produced by the verifier. Verification is a Claude-specific
// guarantee, so verify always targets the reference harness.
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
			h, _ := harness.Get(harness.Default)
			req := harness.Request{
				Manifest:   rm,
				PersonaDir: filepath.Join(environment.RepoDir(e.Hash), "personas", rm.Persona.Name),
				DestDir:    environment.CacheDir(e.Hash, harness.Default, args[0]),
			}
			report, err := h.Materialize(req)
			if err != nil {
				// non-zero exit with the embedded diff
				return err
			}
			printReport(cmd, report)
			fmt.Fprintln(cmd.OutOrStdout(), "\nVerified: materialized environment matches the manifest.")
			return nil
		},
	}
}
