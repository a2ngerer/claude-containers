package materialize

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/a2ngerer/agent-containers/internal/compose"
	"github.com/a2ngerer/agent-containers/internal/domain"
	"github.com/a2ngerer/agent-containers/internal/enforce"
	"github.com/a2ngerer/agent-containers/internal/environment"
	"github.com/stretchr/testify/require"
)

// seedRepo creates a minimal persona repo tree the way init/snapshot would, so
// Materialize has skills/agents to copy. Returns the opened environment.
func seedRepo(t *testing.T, persona string) *environment.Environment {
	t.Helper()
	home := t.TempDir()
	t.Setenv("ACON_HOME", home)
	ws := t.TempDir()

	e, err := environment.Create(ws)
	require.NoError(t, err)

	pdir := filepath.Join(environment.RepoDir(e.Hash), "personas", persona)
	// allowlisted skill
	require.NoError(t, os.MkdirAll(filepath.Join(pdir, "skills", "security-review"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(pdir, "skills", "security-review", "SKILL.md"),
		[]byte("# security-review\n"), 0o644))
	// a skill that is NOT in the allowlist -> must never be materialized
	require.NoError(t, os.MkdirAll(filepath.Join(pdir, "skills", "writing-plans"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(pdir, "skills", "writing-plans", "SKILL.md"),
		[]byte("# writing-plans\n"), 0o644))
	// allowlisted subagent
	require.NoError(t, os.MkdirAll(filepath.Join(pdir, "agents"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(pdir, "agents", "code-reviewer.md"),
		[]byte("# code-reviewer\n"), 0o644))

	return e
}

func reviewerManifest() compose.ResolvedManifest {
	return compose.ResolvedManifest{
		Persona: domain.Persona{
			Name:     "reviewer",
			Metadata: domain.Metadata{Version: "1.2.0"},
		},
		Skills:     []string{"security-review"},
		Subagents:  []string{"code-reviewer"},
		ClaudeMD:   "# reviewer\nUncontaminated.\n",
		SettingSrc: []string{"user", "project"},
		Enforcement: domain.Enforcement{
			PermissionMode: "read-only",
			ToolsAllow:     []string{"Read", "Grep"},
			ToolsDeny:      []string{"Bash(git commit:*)"},
		},
		MCP: domain.MCPConfig{Config: "", Strict: true},
	}
}

func TestMaterialize_AllowlistOnly(t *testing.T) {
	e := seedRepo(t, "reviewer")
	rm := reviewerManifest()
	dest := filepath.Join(t.TempDir(), "cfg")

	require.NoError(t, Materialize(e, rm, dest))

	// allowlisted skill present
	require.FileExists(t, filepath.Join(dest, "skills", "security-review", "SKILL.md"))
	// withheld skill absent
	_, err := os.Stat(filepath.Join(dest, "skills", "writing-plans"))
	require.True(t, os.IsNotExist(err), "non-allowlisted skill must not be materialized")
	// subagent present
	require.FileExists(t, filepath.Join(dest, "agents", "code-reviewer.md"))
	// generated files
	require.FileExists(t, filepath.Join(dest, "CLAUDE.md"))
	require.FileExists(t, filepath.Join(dest, "settings.json"))
	// I3: read-only persona is MCP-isolated -> an empty mcp.json is always
	// materialized so the launch can pass --strict-mcp-config and no project
	// or user MCP server leaks in.
	require.FileExists(t, filepath.Join(dest, "mcp.json"))

	md, err := os.ReadFile(filepath.Join(dest, "CLAUDE.md"))
	require.NoError(t, err)
	require.Equal(t, "# reviewer\nUncontaminated.\n", string(md))
}

// C1 — symlink leak: a file symlink inside an allowlisted skill must make
// Materialize fail closed instead of following it and copying host content
// (or smuggling an executable skill) into the isolated config dir.
func TestMaterialize_SkillSymlinkRejected(t *testing.T) {
	e := seedRepo(t, "reviewer")
	rm := reviewerManifest()

	// Plant a symlink inside the allowlisted skill pointing at a host secret.
	pdir := filepath.Join(environment.RepoDir(e.Hash), "personas", "reviewer")
	secret := filepath.Join(t.TempDir(), "host-secret")
	require.NoError(t, os.WriteFile(secret, []byte("TOP SECRET"), 0o600))
	link := filepath.Join(pdir, "skills", "security-review", "leak.txt")
	require.NoError(t, os.Symlink(secret, link))

	dest := filepath.Join(t.TempDir(), "cfg")
	err := Materialize(e, rm, dest)
	require.Error(t, err, "symlink in persona skill must be rejected fail-closed")
	require.Contains(t, err.Error(), "symlink not allowed")
}

// C2 — path traversal: a skill/subagent name from the untrusted manifest that
// escapes its target dir (e.g. "../evil") must be rejected before any
// filepath.Join, so nothing is written outside destDir.
func TestMaterialize_TraversalSkillNameRejected(t *testing.T) {
	e := seedRepo(t, "reviewer")
	rm := reviewerManifest()
	rm.Skills = []string{"../evil"}

	dest := filepath.Join(t.TempDir(), "cfg")
	err := Materialize(e, rm, dest)
	require.Error(t, err, "skill name escaping destDir must be rejected")
	require.Contains(t, err.Error(), "invalid persona component name")
}

func TestMaterialize_TraversalSubagentNameRejected(t *testing.T) {
	e := seedRepo(t, "reviewer")
	rm := reviewerManifest()
	rm.Subagents = []string{"../../evil"}

	dest := filepath.Join(t.TempDir(), "cfg")
	err := Materialize(e, rm, dest)
	require.Error(t, err, "subagent name escaping destDir must be rejected")
	require.Contains(t, err.Error(), "invalid persona component name")
}

// I3 — a read-only persona with no MCP config and Strict=false is still
// MCP-isolated: an empty mcp.json must be materialized so launch can enforce
// --strict-mcp-config.
func TestMaterialize_ReadOnlyWritesEmptyMCP(t *testing.T) {
	e := seedRepo(t, "reviewer")
	rm := reviewerManifest()
	rm.MCP = domain.MCPConfig{Config: "", Strict: false} // read-only still isolates

	dest := filepath.Join(t.TempDir(), "cfg")
	require.NoError(t, Materialize(e, rm, dest))

	got, err := os.ReadFile(filepath.Join(dest, "mcp.json"))
	require.NoError(t, err)
	require.JSONEq(t, `{"mcpServers":{}}`, string(got))
}

// I3 — a non-isolated persona (default mode, no MCP, not strict) gets no
// mcp.json, so the launch omits MCP flags entirely.
func TestMaterialize_NonIsolatedHasNoMCP(t *testing.T) {
	e := seedRepo(t, "reviewer")
	rm := reviewerManifest()
	rm.Enforcement.PermissionMode = "default"
	rm.MCP = domain.MCPConfig{Config: "", Strict: false}

	dest := filepath.Join(t.TempDir(), "cfg")
	require.NoError(t, Materialize(e, rm, dest))

	_, err := os.Stat(filepath.Join(dest, "mcp.json"))
	require.True(t, os.IsNotExist(err), "non-isolated persona must not get an mcp.json")
}

// Materialize <-> Verify consistency: a freshly materialized dir must pass
// Verify with Clean=true, against the real production code on both sides. This
// is the contract the whole isolation core rests on.
func TestMaterialize_VerifyRoundTrip(t *testing.T) {
	e := seedRepo(t, "reviewer")
	rm := reviewerManifest()
	dest := filepath.Join(t.TempDir(), "cfg")
	require.NoError(t, Materialize(e, rm, dest))

	personaDir := filepath.Join(environment.RepoDir(e.Hash), "personas", rm.Persona.Name)
	att, err := enforce.Verify(rm, personaDir, dest)
	require.NoError(t, err, "Materialize output must satisfy Verify")
	require.True(t, att.Clean)
}

func TestMaterialize_Idempotent(t *testing.T) {
	e := seedRepo(t, "reviewer")
	rm := reviewerManifest()
	dest := filepath.Join(t.TempDir(), "cfg")

	require.NoError(t, Materialize(e, rm, dest))
	first := snapshotDir(t, dest)

	require.NoError(t, Materialize(e, rm, dest))
	second := snapshotDir(t, dest)

	require.Equal(t, first, second, "second materialize must be byte-identical")
}

// snapshotDir returns a deterministic map of relative path -> file content for
// every regular file under root.
func snapshotDir(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out[rel] = string(data)
		return nil
	})
	require.NoError(t, err)
	return out
}
