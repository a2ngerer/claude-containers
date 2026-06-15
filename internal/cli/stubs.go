package cli

import "github.com/spf13/cobra"

// Temporary stubs so the root command compiles. Tasks 10-11 replace
// these (list.go, status.go) with the real implementations.
func newListCmd() *cobra.Command {
	return &cobra.Command{Use: "list", Aliases: []string{"ls"}, Short: "List personas", RunE: func(*cobra.Command, []string) error { return nil }}
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{Use: "status", Short: "Show active persona and workspace", RunE: func(*cobra.Command, []string) error { return nil }}
}
