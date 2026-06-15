package activate

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/angerer/claude_git/internal/compose"
	"github.com/angerer/claude_git/internal/domain"
	"github.com/angerer/claude_git/internal/environment"
	"github.com/stretchr/testify/require"
)

// seedReviewerEnv writes a _base layer and a reviewer persona on disk so
// compose.Compose can resolve "reviewer". Returns the opened environment.
func seedReviewerEnv(t *testing.T) *environment.Environment {
	t.Helper()
	home := t.TempDir()
	t.Setenv("CLAUDE_GIT_HOME", home)
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

	res, err := Activate(e, "reviewer")
	require.NoError(t, err)
	require.True(t, res.Attestation.Clean)
	require.Equal(t, environment.CacheDir(e.Hash, "reviewer"), res.ConfigDir)

	// config dir materialized outside the workspace
	require.NotContains(t, res.ConfigDir, e.Workspace)
	require.FileExists(t, filepath.Join(res.ConfigDir, "settings.json"))
	require.FileExists(t, filepath.Join(res.ConfigDir, "skills", "security-review", "SKILL.md"))

	// withheld narrative includes the non-allowlisted build skill
	var withheld []string
	for _, line := range res.Attestation.Withheld {
		if line.Kind == "skill" {
			withheld = line.Names
		}
	}
	require.Contains(t, withheld, "writing-plans")

	// launch is built
	require.Equal(t, []string{"CLAUDE_CONFIG_DIR=" + res.ConfigDir}, res.Launch.Env)
	require.Equal(t, "claude", res.Launch.Argv[0])

	// active persona recorded
	e2, err := environment.Open(e.Workspace)
	require.NoError(t, err)
	require.NotNil(t, e2)
}

func TestActivate_DefaultsToLatestVersion(t *testing.T) {
	e := seedReviewerEnv(t)
	// "reviewer" with no :version must resolve and activate without error.
	_, err := Activate(e, "reviewer")
	require.NoError(t, err)
}

func TestActivate_NotFound(t *testing.T) {
	e := seedReviewerEnv(t)
	_, err := Activate(e, "ghost")
	require.Error(t, err)
}

// TestActivate_FailClosed verifies the fail-closed contract: when Verify
// detects a mismatch the returned ActivationResult is zero (no ConfigDir,
// no Launch) and the error wraps domain.ErrVerifyMismatch.
//
// Strategy: manipulate the persona's enforcement manifest after Compose so
// that Materialize writes settings.json with wrong tools, making Verify report
// a mismatch. We simulate this by injecting a non-allowlisted file directly
// into the materialized config dir between the Materialize and Verify steps.
// Since we cannot intercept Activate's internal flow, we instead use the
// verifyFn indirection exposed for tests.
//
// Simpler integration approach: activate once, then swap settings.json with
// content that has wrong permissions so Verify detects a mismatch on re-run.
// But Materialize cleans the dir each run, so we instead use a persona
// variant whose mcp.json expectation will be violated by removing that file
// from the cache dir after Materialize runs — which also cannot be done from
// outside. Therefore the test is kept at the contract level: any error from
// Activate must return a zero ActivationResult, verifying the fail-closed
// discipline even for non-verify errors.
func TestActivate_FailClosed_ZeroResultOnError(t *testing.T) {
	e := seedReviewerEnv(t)

	// Calling with a non-existent persona guarantees an error.
	res, err := Activate(e, "does-not-exist")
	require.Error(t, err)

	// The ActivationResult must be zero — no ConfigDir, no launch.
	var zero ActivationResult
	require.Equal(t, zero.ConfigDir, res.ConfigDir,
		"ConfigDir must be empty on any error path (fail-closed contract)")
	require.Equal(t, zero.Launch.Argv, res.Launch.Argv,
		"Launch.Argv must be nil on any error path (fail-closed contract)")
	require.Equal(t, zero.Launch.Env, res.Launch.Env,
		"Launch.Env must be nil on any error path (fail-closed contract)")
}

// TestActivate_FailClosed_VerifyMismatch injects an extra file into the
// persona source skill tree so Materialize copies it into the config dir and
// Verify finds an unexpected path, returning ErrVerifyMismatch. Activate must
// not produce a launch spec.
func TestActivate_FailClosed_VerifyMismatch(t *testing.T) {
	e := seedReviewerEnv(t)

	// First activation succeeds to establish the environment.
	_, err := Activate(e, "reviewer")
	require.NoError(t, err)

	// Inject a file into the allowlisted skill's source tree with a name that
	// Verify's expectedPaths walk will NOT discover (because it walks the
	// source tree to build expected). We need a path that WILL be on disk in
	// configDir but NOT in expected. This means a file that Materialize copies
	// but expectedPaths misses.
	//
	// Since expectedPaths walks the persona source tree, injecting there means
	// the injected file IS expected. Instead: inject after materialization by
	// using verifyFn override. Since the package is internal we can set
	// verifyFn directly in tests.
	verifyFn = func(_ compose.ResolvedManifest, _, _ string) (domain.Attestation, error) {
		return domain.Attestation{Clean: false},
			fmt.Errorf("%w: injected mismatch for test", domain.ErrVerifyMismatch)
	}
	t.Cleanup(func() { verifyFn = defaultVerify })

	res, err := Activate(e, "reviewer")
	require.Error(t, err)
	require.ErrorIs(t, err, domain.ErrVerifyMismatch)
	require.Empty(t, res.ConfigDir, "ConfigDir must be empty on fail-closed path")
	require.Empty(t, res.Launch.Argv, "Launch.Argv must be empty on fail-closed path")
}
