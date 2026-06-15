package compose_test

import (
	"testing"

	"github.com/angerer/claude_git/internal/compose"
	"github.com/angerer/claude_git/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestDiff_CapabilityDelta(t *testing.T) {
	a := compose.ResolvedManifest{
		Persona:   domain.Persona{Name: "coder"},
		Skills:    []string{"build-skill", "shared-skill"},
		Subagents: []string{"coder-agent", "shared-agent"},
		Enforcement: domain.Enforcement{
			ToolsAllow: []string{"Write", "Edit", "Read"},
			ToolsDeny:  []string{},
		},
	}
	b := compose.ResolvedManifest{
		Persona:   domain.Persona{Name: "reviewer"},
		Skills:    []string{"security-review", "shared-skill"},
		Subagents: []string{"shared-agent"},
		Enforcement: domain.Enforcement{
			ToolsAllow: []string{"Read"},
			ToolsDeny:  []string{"Write", "Edit"},
		},
	}

	d := compose.Diff(a, b)

	require.Equal(t, "coder", d.NameA)
	require.Equal(t, "reviewer", d.NameB)
	require.Equal(t, []string{"build-skill"}, d.SkillsOnlyA)
	require.Equal(t, []string{"security-review"}, d.SkillsOnlyB)
	require.Equal(t, []string{"coder-agent"}, d.SubagentsOnlyA)
	require.Empty(t, d.SubagentsOnlyB)
	require.Equal(t, []string{"Edit", "Write"}, d.AllowOnlyA) // sorted
	require.Empty(t, d.AllowOnlyB)
	require.Empty(t, d.DenyOnlyA)
	require.Equal(t, []string{"Edit", "Write"}, d.DenyOnlyB)
}
