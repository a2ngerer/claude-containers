package enforce

import (
	"testing"

	"github.com/a2ngerer/claude-containers/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestBuildPermissions(t *testing.T) {
	tests := []struct {
		name      string
		enf       domain.Enforcement
		wantAllow []string
		wantDeny  []string
		wantMode  string
	}{
		{
			name: "read-only adds standard write denials",
			enf: domain.Enforcement{
				PermissionMode: "read-only",
				ToolsAllow:     []string{"Read", "Grep"},
				ToolsDeny:      []string{"Bash(git push:*)"},
			},
			wantAllow: []string{"Read", "Grep"},
			wantDeny:  []string{"Write", "Edit", "NotebookEdit", "Bash(git push:*)"},
			wantMode:  "read-only",
		},
		{
			name: "default mode keeps only explicit denials",
			enf: domain.Enforcement{
				PermissionMode: "default",
				ToolsAllow:     []string{"Read", "Write", "Edit"},
				ToolsDeny:      []string{"Bash(rm:*)"},
			},
			wantAllow: []string{"Read", "Write", "Edit"},
			wantDeny:  []string{"Bash(rm:*)"},
			wantMode:  "default",
		},
		{
			name: "read-only deduplicates an explicit Write denial",
			enf: domain.Enforcement{
				PermissionMode: "read-only",
				ToolsAllow:     nil,
				ToolsDeny:      []string{"Write", "Bash(git commit:*)"},
			},
			wantAllow: nil,
			wantDeny:  []string{"Write", "Edit", "NotebookEdit", "Bash(git commit:*)"},
			wantMode:  "read-only",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildPermissions(tt.enf)
			require.Equal(t, tt.wantAllow, got.Allow)
			require.Equal(t, tt.wantDeny, got.Deny)
			require.Equal(t, tt.wantMode, got.Mode)
		})
	}
}
