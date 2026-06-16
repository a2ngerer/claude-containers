package activate

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/a2ngerer/agent-containers/internal/domain"
	"github.com/a2ngerer/agent-containers/internal/environment"
	"github.com/a2ngerer/agent-containers/internal/harness"
	"github.com/stretchr/testify/require"
)

// seedReviewerEnv writes a _base layer and a reviewer persona on disk so
// compose.Compose can resolve "reviewer". Returns the opened environment.
func seedReviewerEnv(t *testing.T) *environment.Environment {
	t.Helper()
	home := t.TempDir()
	t.Setenv("ACON_HOME", home)
	ws := t.TempDir()
	e, err := environment.Create(ws)
	require.NoError(t, err)

	repo := environment.RepoDir(e.Hash)

	// _base layer
	base := filepath.Join(repo, "personas", "_base")
	require.NoError(t, os.MkdirAll(base, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(base, "persona.toml"),
		[]byte("name = \"_base\"\n\n[config]\nclaude_md = \"CLAUDE.md\"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(base, "CLAUDE.md"), []byte("# base\n"), 0o644))

	// reviewer persona
	rev := filepath.Join(repo, "personas", "reviewer")
	require.NoError(t, os.MkdirAll(filepath.Join(rev, "skills", "security-review"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(rev, "skills", "security-review", "SKILL.md"), []byte("# sr\n"), 0o644))
	// a withheld build skill physically present but not allowlisted
	require.NoError(t, os.MkdirAll(filepath.Join(rev, "skills", "writing-plans"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(rev, "skills", "writing-plans", "SKILL.md"), []byte("# wp\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(rev, "CLAUDE.md"), []byte("# reviewer\n"), 0o644))

	manifest := `name = "reviewer"
extends = "_base"

[config]
claude_md = "CLAUDE.md"
setting_sources = ["user", "project"]

[config.skills]
mode = "allowlist"
include = ["security-review"]

[config.mcp]
config = ""
strict = true

[enforcement]
permission_mode = "read-only"
tools.allow = ["Read", "Grep"]
tools.deny = ["Bash(git commit:*)"]

[metadata]
version = "1.2.0"
author = "tester"
`
	require.NoError(t, os.WriteFile(filepath.Join(rev, "persona.toml"), []byte(manifest), 0o644))
	return e
}

func TestActivate_HappyPath(t *testing.T) {
	e := seedReviewerEnv(t)

	res, err := Activate(e, "reviewer", "claude")
	require.NoError(t, err)
	require.True(t, res.Report.Verified)
	require.Equal(t, "claude", res.Harness)
	require.Equal(t, environment.CacheDir(e.Hash, "claude", "reviewer"), res.ConfigDir)

	// config dir materialized outside the workspace
	require.NotContains(t, res.ConfigDir, e.Workspace)
	require.FileExists(t, filepath.Join(res.ConfigDir, "settings.json"))
	require.FileExists(t, filepath.Join(res.ConfigDir, "skills", "security-review", "SKILL.md"))

	// withheld narrative includes the non-allowlisted build skill
	require.Contains(t, res.Report.Withheld, "writing-plans")

	// launch is built for the Claude binary
	require.Equal(t, []string{"CLAUDE_CONFIG_DIR=" + res.ConfigDir}, res.Launch.Env)
	require.Equal(t, "claude", res.Launch.Argv[0])

	// active persona recorded
	e2, err := environment.Open(e.Workspace)
	require.NoError(t, err)
	require.Equal(t, "reviewer", e2.ActivePersona())
}

func TestActivate_DefaultsToLatestVersion(t *testing.T) {
	e := seedReviewerEnv(t)
	// "reviewer" with no :version must resolve and activate without error.
	_, err := Activate(e, "reviewer", "claude")
	require.NoError(t, err)
}

func TestActivate_NotFound(t *testing.T) {
	e := seedReviewerEnv(t)
	_, err := Activate(e, "ghost", "claude")
	require.Error(t, err)
}

func TestActivate_UnknownHarness(t *testing.T) {
	e := seedReviewerEnv(t)
	_, err := Activate(e, "reviewer", "does-not-exist-harness")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown harness")
}

// TestActivate_FailClosed_ZeroResultOnError verifies the fail-closed contract:
// any error path returns a zero ActivationResult (no ConfigDir, no launch).
func TestActivate_FailClosed_ZeroResultOnError(t *testing.T) {
	e := seedReviewerEnv(t)

	res, err := Activate(e, "does-not-exist", "claude")
	require.Error(t, err)

	var zero ActivationResult
	require.Equal(t, zero.ConfigDir, res.ConfigDir,
		"ConfigDir must be empty on any error path (fail-closed contract)")
	require.Equal(t, zero.Launch.Argv, res.Launch.Argv,
		"Launch.Argv must be nil on any error path (fail-closed contract)")
	require.Equal(t, zero.Launch.Env, res.Launch.Env,
		"Launch.Env must be nil on any error path (fail-closed contract)")
}

// brokenHarness is a test adapter whose Materialize always fails with a
// verify-mismatch error, standing in for a drift detected by the Claude adapter.
type brokenHarness struct{}

func (brokenHarness) ID() string          { return "broken-test" }
func (brokenHarness) DisplayName() string { return "broken (test)" }
func (brokenHarness) Materialize(harness.Request) (harness.Report, error) {
	return harness.Report{}, fmt.Errorf("%w: injected mismatch for test", domain.ErrVerifyMismatch)
}
func (brokenHarness) Launch(harness.Request) harness.LaunchSpec { return harness.LaunchSpec{} }
func (brokenHarness) Detect() harness.Detection                 { return harness.Detection{ID: "broken-test"} }

// TestActivate_FailClosed_MaterializeError asserts that when the target harness
// Materialize fails (e.g. a drift verify mismatch), Activate fails closed: the
// error propagates and no launch spec or config dir is produced.
func TestActivate_FailClosed_MaterializeError(t *testing.T) {
	harness.Register(brokenHarness{})
	e := seedReviewerEnv(t)

	res, err := Activate(e, "reviewer", "broken-test")
	require.Error(t, err)
	require.ErrorIs(t, err, domain.ErrVerifyMismatch)
	require.Empty(t, res.ConfigDir, "ConfigDir must be empty on fail-closed path")
	require.Empty(t, res.Launch.Argv, "Launch.Argv must be empty on fail-closed path")
}
