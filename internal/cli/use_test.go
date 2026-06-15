package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDispatchArgs(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "reserved subcommand is left untouched",
			in:   []string{"snapshot", "reviewer", "-m", "msg"},
			want: []string{"snapshot", "reviewer", "-m", "msg"},
		},
		{
			name: "bare persona name is rewritten to use",
			in:   []string{"reviewer"},
			want: []string{"use", "reviewer"},
		},
		{
			name: "persona with version is rewritten to use",
			in:   []string{"reviewer:1.2.0"},
			want: []string{"use", "reviewer:1.2.0"},
		},
		{
			name: "no args is left untouched",
			in:   []string{},
			want: []string{},
		},
		{
			name: "leading flag is left untouched",
			in:   []string{"--help"},
			want: []string{"--help"},
		},
		{
			name: "init stays init",
			in:   []string{"init"},
			want: []string{"init"},
		},
		{
			name: "verify stays verify",
			in:   []string{"verify", "reviewer"},
			want: []string{"verify", "reviewer"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, DispatchArgs(tt.in))
		})
	}
}

func TestIsReserved(t *testing.T) {
	require.True(t, isReserved("snapshot"))
	require.True(t, isReserved("use"))
	require.True(t, isReserved("deactivate"))
	require.True(t, isReserved("verify"))
	require.False(t, isReserved("reviewer"))
	require.False(t, isReserved("coder"))
}

func TestMergeEnv(t *testing.T) {
	t.Run("override wins over base for same key", func(t *testing.T) {
		base := []string{"CLAUDE_CONFIG_DIR=/old", "HOME=/home/user"}
		overrides := []string{"CLAUDE_CONFIG_DIR=/new"}
		got := mergeEnv(base, overrides)
		// exactly one CLAUDE_CONFIG_DIR entry, value must be /new
		var matches []string
		for _, kv := range got {
			if len(kv) >= len("CLAUDE_CONFIG_DIR=") && kv[:len("CLAUDE_CONFIG_DIR=")] == "CLAUDE_CONFIG_DIR=" {
				matches = append(matches, kv)
			}
		}
		require.Len(t, matches, 1, "expected exactly one CLAUDE_CONFIG_DIR entry")
		require.Equal(t, "CLAUDE_CONFIG_DIR=/new", matches[0])
	})

	t.Run("keys absent from overrides are kept from base", func(t *testing.T) {
		base := []string{"HOME=/home/user", "PATH=/usr/bin"}
		overrides := []string{"CLAUDE_CONFIG_DIR=/new"}
		got := mergeEnv(base, overrides)
		require.Contains(t, got, "HOME=/home/user")
		require.Contains(t, got, "PATH=/usr/bin")
		require.Contains(t, got, "CLAUDE_CONFIG_DIR=/new")
	})

	t.Run("empty overrides returns base unchanged", func(t *testing.T) {
		base := []string{"A=1", "B=2"}
		got := mergeEnv(base, nil)
		require.Equal(t, base, got)
	})
}

func TestDispatchArgsHelpFlag(t *testing.T) {
	// "-h" is a flag, so it must NOT be rewritten to "use".
	got := DispatchArgs([]string{"-h"})
	require.Equal(t, []string{"-h"}, got)
}
