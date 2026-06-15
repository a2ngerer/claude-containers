package compose_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/angerer/claude_git/internal/compose"
	"github.com/angerer/claude_git/internal/domain"
	"github.com/angerer/claude_git/internal/environment"
	"github.com/stretchr/testify/require"
)

// seedEnv creates an isolated tool home + a bound environment with a _base layer
// and writes the given personas into the repo. Returns the open environment.
func seedEnv(t *testing.T, personas ...domain.Persona) *environment.Environment {
	t.Helper()
	t.Setenv("CLAUDE_GIT_HOME", t.TempDir())
	ws := t.TempDir()
	env, err := environment.Create(ws)
	require.NoError(t, err)
	for _, p := range personas {
		require.NoError(t, env.SavePersona(p))
		writeClaudeMD(t, env, p.Name, "MD:"+p.Name)
	}
	return env
}

// writeClaudeMD writes a CLAUDE.md into a persona dir in the repo.
func writeClaudeMD(t *testing.T, env *environment.Environment, persona, body string) {
	t.Helper()
	dir := filepath.Join(environment.RepoDir(env.Hash), "personas", persona)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(body), 0o644))
}

func baseLayer() domain.Persona {
	return domain.Persona{
		Name:    "_base",
		Extends: "",
		Config: domain.Config{
			ClaudeMD:       "CLAUDE.md",
			SettingSources: []string{"user", "project", "local"},
			Skills:         domain.SkillSet{Mode: "allowlist", Include: []string{"base-skill"}},
			Subagents:      domain.SubagentSet{Include: []string{"base-agent"}},
		},
	}
}

func TestCompose_UnionAndScalarOverride(t *testing.T) {
	base := baseLayer()
	leaf := domain.Persona{
		Name:    "coder",
		Extends: "_base",
		Config: domain.Config{
			ClaudeMD:       "CLAUDE.md",
			SettingSources: []string{"user", "project"}, // overrides base
			Skills:         domain.SkillSet{Mode: "allowlist", Include: []string{"build-skill"}},
			Subagents:      domain.SubagentSet{Include: []string{"coder-agent"}},
			MCP:            domain.MCPConfig{Config: "mcp.json", Strict: true},
		},
		Enforcement: domain.Enforcement{PermissionMode: "default", ToolsAllow: []string{"Write"}},
	}
	env := seedEnv(t, base, leaf)

	rm, err := compose.Compose(env, "coder")
	require.NoError(t, err)

	// skills/subagents are the UNION of base + persona
	require.ElementsMatch(t, []string{"base-skill", "build-skill"}, rm.Skills)
	require.ElementsMatch(t, []string{"base-agent", "coder-agent"}, rm.Subagents)
	// scalars come from the persona layer
	require.Equal(t, []string{"user", "project"}, rm.SettingSrc)
	require.Equal(t, "default", rm.Enforcement.PermissionMode)
	require.Equal(t, []string{"Write"}, rm.Enforcement.ToolsAllow)
	require.Equal(t, "mcp.json", rm.MCP.Config)
	require.True(t, rm.MCP.Strict)
	// CLAUDE.md = base body + "\n\n" + persona body
	require.Equal(t, "MD:_base\n\nMD:coder", rm.ClaudeMD)
}

func TestCompose_ReplaceModeDropsBaseSkills(t *testing.T) {
	base := baseLayer()
	leaf := domain.Persona{
		Name:    "reviewer",
		Extends: "_base",
		Config: domain.Config{
			ClaudeMD:       "CLAUDE.md",
			SettingSources: []string{"user", "project"},
			Skills:         domain.SkillSet{Mode: "replace", Include: []string{"security-review"}},
			Subagents:      domain.SubagentSet{Include: []string{"code-reviewer"}},
		},
	}
	env := seedEnv(t, base, leaf)

	rm, err := compose.Compose(env, "reviewer")
	require.NoError(t, err)

	// replace mode: ONLY the persona's skills, base-skill dropped
	require.Equal(t, []string{"security-review"}, rm.Skills)
	// subagents still union
	require.ElementsMatch(t, []string{"base-agent", "code-reviewer"}, rm.Subagents)
}

func TestCompose_NoExtendsHasNoBasePrefix(t *testing.T) {
	standalone := domain.Persona{
		Name:    "solo",
		Extends: "",
		Config: domain.Config{
			ClaudeMD: "CLAUDE.md",
			Skills:   domain.SkillSet{Mode: "allowlist", Include: []string{"only-skill"}},
		},
	}
	env := seedEnv(t, standalone)

	rm, err := compose.Compose(env, "solo")
	require.NoError(t, err)

	require.Equal(t, []string{"only-skill"}, rm.Skills)
	require.Equal(t, "MD:solo", rm.ClaudeMD) // no "\n\n" prefix
}

func TestCompose_ExtendsLayerNotFound(t *testing.T) {
	leaf := domain.Persona{
		Name:    "broken",
		Extends: "_ghost",
		Config:  domain.Config{ClaudeMD: "CLAUDE.md"},
	}
	env := seedEnv(t, leaf)

	_, err := compose.Compose(env, "broken")
	require.ErrorIs(t, err, domain.ErrLayerNotFound)
}
