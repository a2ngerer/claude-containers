package cli

import "github.com/spf13/cobra"

// Temporary stubs so the root command compiles in Task 1. Tasks 9-11 replace
// these (init.go, list.go, status.go) with the real implementations and shrink
// this file until it is deleted.
func newInitCmd() *cobra.Command {
	return &cobra.Command{Use: "init", Short: "Bind the current workspace", RunE: func(*cobra.Command, []string) error { return nil }}
}

func newListCmd() *cobra.Command {
	return &cobra.Command{Use: "list", Aliases: []string{"ls"}, Short: "List personas", RunE: func(*cobra.Command, []string) error { return nil }}
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{Use: "status", Short: "Show active persona and workspace", RunE: func(*cobra.Command, []string) error { return nil }}
}
