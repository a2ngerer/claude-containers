package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewRootCmd_Use(t *testing.T) {
	cmd := NewRootCmd()
	require.Equal(t, "acon", cmd.Use)
}

func TestNewRootCmd_HasSubcommands(t *testing.T) {
	cmd := NewRootCmd()
	names := map[string]bool{}
	for _, c := range cmd.Commands() {
		names[c.Name()] = true
	}
	require.True(t, names["init"], "init subcommand must be registered")
	require.True(t, names["list"], "list subcommand must be registered")
	require.True(t, names["status"], "status subcommand must be registered")
}

func TestRootCmd_HelpDoesNotError(t *testing.T) {
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--help"})
	require.NoError(t, cmd.Execute())
}

func TestRoot_RegistersM2Commands(t *testing.T) {
	root := NewRootCmd()
	have := map[string]bool{}
	for _, c := range root.Commands() {
		have[c.Name()] = true
	}
	for _, name := range []string{"new", "show", "edit", "rm", "snapshot", "log", "diff", "rollback", "tag"} {
		require.True(t, have[name], "root must register %q", name)
	}
}
