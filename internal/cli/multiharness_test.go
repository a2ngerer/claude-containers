package cli

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfigHarness_SetAndGet(t *testing.T) {
	seedReviewer(t)

	set := newConfigHarnessCmd()
	var out bytes.Buffer
	set.SetOut(&out)
	set.SetArgs([]string{"opencode"})
	require.NoError(t, set.Execute())
	require.Contains(t, out.String(), "opencode")

	// a fresh command must read the persisted default from env.toml
	get := newConfigHarnessCmd()
	out.Reset()
	get.SetOut(&out)
	get.SetArgs([]string{})
	require.NoError(t, get.Execute())
	require.Contains(t, out.String(), "opencode")
}

func TestConfigHarness_Unknown(t *testing.T) {
	seedReviewer(t)
	cmd := newConfigHarnessCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"bogus-harness"})
	require.Error(t, cmd.Execute())
}

func TestExportCmd_OpenCode(t *testing.T) {
	seedReviewer(t)
	outDir := t.TempDir()

	cmd := newExportCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"reviewer", "--harness", "opencode", "--out", outDir})
	require.NoError(t, cmd.Execute())

	require.FileExists(t, filepath.Join(outDir, "AGENTS.md"))
	require.FileExists(t, filepath.Join(outDir, "opencode.json"))
	require.Contains(t, out.String(), "Exported to")
	require.Contains(t, out.String(), "→ opencode")
}

func TestHarnessesCmd(t *testing.T) {
	cmd := newHarnessesCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	require.NoError(t, cmd.Execute())
	s := out.String()
	require.Contains(t, s, "claude")
	require.Contains(t, s, "opencode")
	require.Contains(t, s, "Antigravity")
}

func TestUseCmd_HarnessOverride(t *testing.T) {
	seedReviewer(t)
	cmd := newUseCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"reviewer", "--harness", "opencode"})
	require.NoError(t, cmd.Execute())

	s := out.String()
	require.Contains(t, s, "→ opencode")
	require.Contains(t, s, "OPENCODE_CONFIG_DIR")
}
