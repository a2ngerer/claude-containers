package cli

import "github.com/spf13/cobra"

// Temporary stub so the root command compiles. Task 11 replaces this (status.go).
func newStatusCmd() *cobra.Command {
	return &cobra.Command{Use: "status", Short: "Show active persona and workspace", RunE: func(*cobra.Command, []string) error { return nil }}
}
