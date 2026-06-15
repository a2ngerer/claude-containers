package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/angerer/claude_git/internal/domain"
	"github.com/angerer/claude_git/internal/environment"
	"github.com/stretchr/testify/require"
)

func TestPersonaTemplate_CoderAndReviewer(t *testing.T) {
	coder, ok := personaTemplate("coder")
	require.True(t, ok)
	require.Contains(t, coder.TOML, `name        = "coder"`)
	require.Contains(t, coder.TOML, `setting_sources = ["user", "project", "local"]`)
	require.Contains(t, coder.TOML, `"Write"`)
	require.NotEmpty(t, coder.ClaudeMD)

	rev, ok := personaTemplate("reviewer")
	require.True(t, ok)
	require.Contains(t, rev.TOML, `name        = "reviewer"`)
	require.Contains(t, rev.TOML, `permission_mode = "read-only"`)
	require.Contains(t, rev.TOML, `setting_sources = ["user", "project"]`)
	// reviewer denies write tools
	deny := rev.TOML[strings.Index(rev.TOML, "deny"):]
	require.Contains(t, deny, `"Write"`)
	require.Contains(t, deny, `"Edit"`)
	require.Contains(t, deny, `"NotebookEdit"`)

	_, ok = personaTemplate("nope")
	require.False(t, ok)
}

func newTestEnv(t *testing.T) *environment.Environment {
	t.Helper()
	t.Setenv("CLAUDE_GIT_HOME", t.TempDir())
	env, err := environment.Create(t.TempDir())
	require.NoError(t, err)
	return env
}

func TestScaffoldPersona_WritesTOMLandMD(t *testing.T) {
	env := newTestEnv(t)
	sc, _ := personaTemplate("reviewer")

	err := scaffoldPersona(env, "rev1", sc)
	require.NoError(t, err)

	dir := filepath.Join(environment.RepoDir(env.Hash), "personas", "rev1")
	md, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	require.NoError(t, err)
	require.Equal(t, sc.ClaudeMD, string(md))

	p, err := env.LoadPersona("rev1")
	require.NoError(t, err)
	require.Equal(t, "rev1", p.Name) // name overridden to the new name
	require.Equal(t, "read-only", p.Enforcement.PermissionMode)
	require.ElementsMatch(t, []string{"Write", "Edit", "NotebookEdit", "Bash(git commit:*)", "Bash(git push:*)"}, p.Enforcement.ToolsDeny)

	// second scaffold with the same name is rejected
	err = scaffoldPersona(env, "rev1", sc)
	require.ErrorIs(t, err, domain.ErrPersonaExists)
}

func runNew(t *testing.T, env *environment.Environment, args ...string) (string, error) {
	t.Helper()
	cmd := newNewCmd(func() (*environment.Environment, error) { return env, nil })
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestNewCmd_DefaultExtendsBase(t *testing.T) {
	env := newTestEnv(t)
	_, err := runNew(t, env, "blank")
	require.NoError(t, err)

	p, err := env.LoadPersona("blank")
	require.NoError(t, err)
	require.Equal(t, "_base", p.Extends)
}

func TestNewCmd_TemplateCoder(t *testing.T) {
	env := newTestEnv(t)
	_, err := runNew(t, env, "c1", "--template", "coder")
	require.NoError(t, err)

	p, err := env.LoadPersona("c1")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"Read", "Grep", "Glob", "Write", "Edit", "Bash"}, p.Enforcement.ToolsAllow)
	require.Equal(t, []string{"user", "project", "local"}, p.Config.SettingSources)
}

func TestNewCmd_ExtendsOverride(t *testing.T) {
	env := newTestEnv(t)
	_, err := runNew(t, env, "x1", "--template", "reviewer", "--extends", "_custom")
	require.NoError(t, err)

	p, err := env.LoadPersona("x1")
	require.NoError(t, err)
	require.Equal(t, "_custom", p.Extends)
}

func TestNewCmd_FromCopiesPersona(t *testing.T) {
	env := newTestEnv(t)
	_, err := runNew(t, env, "src", "--template", "reviewer")
	require.NoError(t, err)

	_, err = runNew(t, env, "dst", "--from", "src")
	require.NoError(t, err)

	p, err := env.LoadPersona("dst")
	require.NoError(t, err)
	require.Equal(t, "read-only", p.Enforcement.PermissionMode) // copied from src
}

func TestNewCmd_TemplateAndFromConflict(t *testing.T) {
	env := newTestEnv(t)
	_, err := runNew(t, env, "bad", "--template", "coder", "--from", "src")
	require.Error(t, err)
}

func runShow(t *testing.T, env *environment.Environment, args ...string) (string, error) {
	t.Helper()
	cmd := newShowCmd(func() (*environment.Environment, error) { return env, nil })
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestShowCmd_RendersCapabilities(t *testing.T) {
	env := newTestEnv(t)
	// seed a _base layer with a build skill that the reviewer will withhold
	base := domain.Persona{
		Name:   "_base",
		Config: domain.Config{ClaudeMD: "CLAUDE.md", Skills: domain.SkillSet{Mode: "allowlist", Include: []string{"superpowers"}}},
	}
	require.NoError(t, env.SavePersona(base))
	_, err := runNew(t, env, "reviewer", "--template", "reviewer")
	require.NoError(t, err)

	out, err := runShow(t, env, "reviewer")
	require.NoError(t, err)

	require.Contains(t, out, "Persona: reviewer")
	require.Contains(t, out, "security-review") // active skill
	require.Contains(t, out, "code-reviewer")   // active subagent
	require.Contains(t, out, "Write")           // denied tool listed
	require.Contains(t, out, "Withheld")        // withheld section present
	require.Contains(t, out, "superpowers")     // base skill withheld by replace mode
	require.Contains(t, out, "user, project")   // setting sources
}
