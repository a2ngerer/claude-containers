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
