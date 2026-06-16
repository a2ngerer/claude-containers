package harness

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/a2ngerer/agent-containers/internal/compose"
	"github.com/a2ngerer/agent-containers/internal/domain"
	toml "github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/require"
)

// seedPersona builds a persona source tree (skills, a withheld skill, a subagent,
// an mcp.json) and the resolved manifest a reviewer persona would compose to.
func seedPersona(t *testing.T) (string, compose.ResolvedManifest) {
	t.Helper()
	dir := t.TempDir()

	mkSkill(t, dir, "security-review", "Audits code for vulnerabilities")
	mkSkill(t, dir, "writing-plans", "Plans multi-step work") // present but withheld

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "agents"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "agents", "code-reviewer.md"),
		[]byte("---\nname: code-reviewer\ndescription: Reviews a diff for bugs\nmodel: sonnet\n---\nYou are an uncontaminated reviewer.\n"), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "mcp.json"),
		[]byte(`{"mcpServers":{"fetch":{"command":"uvx","args":["mcp-server-fetch"]},"remote_api":{"type":"http","url":"https://api.example.com/mcp","headers":{"Authorization":"Bearer x"}}}}`), 0o644))

	rm := compose.ResolvedManifest{
		Persona:    domain.Persona{Name: "reviewer", Metadata: domain.Metadata{Version: "1.2.0"}},
		Skills:     []string{"security-review"},
		Subagents:  []string{"code-reviewer"},
		ClaudeMD:   "# reviewer\nUncontaminated reviewer.\n",
		SettingSrc: []string{"user", "project"},
		Enforcement: domain.Enforcement{
			PermissionMode: "read-only",
			ToolsAllow:     []string{"Read", "Grep", "Bash(git diff:*)"},
			ToolsDeny:      []string{"Write", "Edit", "Bash(git commit:*)"},
		},
		MCP: domain.MCPConfig{Config: "mcp.json", Strict: true},
	}
	return dir, rm
}

func mkSkill(t *testing.T, dir, name, desc string) {
	t.Helper()
	sd := filepath.Join(dir, "skills", name)
	require.NoError(t, os.MkdirAll(sd, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sd, "SKILL.md"),
		[]byte("---\nname: "+name+"\ndescription: "+desc+"\n---\n# "+name+"\n"), 0o644))
}

func reqFor(t *testing.T, h Harness) (Request, string) {
	t.Helper()
	personaDir, rm := seedPersona(t)
	dest := filepath.Join(t.TempDir(), h.ID())
	return Request{Manifest: rm, PersonaDir: personaDir, DestDir: dest}, dest
}

// readFile reads a file under dest, failing the test if absent.
func readFile(t *testing.T, parts ...string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(parts...))
	require.NoError(t, err)
	return string(b)
}

func walkContains(t *testing.T, root, needle string) bool {
	t.Helper()
	found := false
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && strings.Contains(p, needle) {
			found = true
		}
		return nil
	})
	return found
}

func TestRegistry(t *testing.T) {
	ids := IDs()
	require.Len(t, ids, 7, "expected all six harnesses plus the agents target")
	require.Equal(t, "claude", ids[0], "claude is the canonical first entry")
	for _, id := range []string{"claude", "codex", "opencode", "gemini", "kimi", "antigravity", "agents"} {
		_, ok := Get(id)
		require.True(t, ok, "missing adapter %q", id)
	}
	_, ok := Get("nope")
	require.False(t, ok)
}

func TestParseMCP(t *testing.T) {
	m, err := parseMCP([]byte(`{"mcpServers":{"fetch":{"command":"uvx","args":["x"]},"api":{"type":"http","url":"https://h/mcp"}}}`))
	require.NoError(t, err)
	require.Equal(t, []string{"api", "fetch"}, m.Names) // sorted
	require.False(t, m.Servers["fetch"].remote())
	require.True(t, m.Servers["api"].remote())
	require.Equal(t, "https://h/mcp", m.Servers["api"].URL)
}

// TestWithholdingHoldsEverywhere is the core cross-harness guarantee: the
// non-allowlisted "writing-plans" skill is physically absent from every
// materialized target, and every report names it as deliberately withheld.
func TestWithholdingHoldsEverywhere(t *testing.T) {
	for _, h := range All() {
		t.Run(h.ID(), func(t *testing.T) {
			req, dest := reqFor(t, h)
			r, err := h.Materialize(req)
			require.NoError(t, err)
			require.False(t, walkContains(t, dest, "writing-plans"),
				"withheld skill leaked into %s materialization", h.ID())
			require.Contains(t, r.Withheld, "writing-plans")
			require.Equal(t, "reviewer", r.Persona)
			require.Equal(t, "1.2.0", r.Version)
		})
	}
}

// TestInstructionsCarryOver asserts each harness writes the persona instructions
// into its native instructions file with the content intact.
func TestInstructionsCarryOver(t *testing.T) {
	cases := map[string]string{
		"claude":      "CLAUDE.md",
		"codex":       "AGENTS.md",
		"opencode":    "AGENTS.md",
		"gemini":      filepath.Join(".gemini", "GEMINI.md"),
		"kimi":        "AGENTS.md",
		"antigravity": "AGENTS.md",
		"agents":      "AGENTS.md",
	}
	for id, rel := range cases {
		t.Run(id, func(t *testing.T) {
			h, _ := Get(id)
			req, dest := reqFor(t, h)
			_, err := h.Materialize(req)
			require.NoError(t, err)
			require.Contains(t, readFile(t, dest, rel), "Uncontaminated reviewer.")
		})
	}
}

func TestClaudeVerifiedOthersReport(t *testing.T) {
	for _, h := range All() {
		t.Run(h.ID(), func(t *testing.T) {
			req, _ := reqFor(t, h)
			r, err := h.Materialize(req)
			require.NoError(t, err)
			if h.ID() == "claude" {
				require.True(t, r.Verified, "claude must be drift-verified")
			} else {
				require.False(t, r.Verified, "only claude is drift-verified")
				require.NotEmpty(t, r.Dropped, "export targets must report some lossiness")
			}
			require.NotEmpty(t, r.Lines)
		})
	}
}

func TestCodexMCPToToml(t *testing.T) {
	h, _ := Get("codex")
	req, dest := reqFor(t, h)
	_, err := h.Materialize(req)
	require.NoError(t, err)

	var cfg map[string]any
	require.NoError(t, toml.Unmarshal([]byte(readFile(t, dest, "config.toml")), &cfg))
	servers, ok := cfg["mcp_servers"].(map[string]any)
	require.True(t, ok, "config.toml must carry [mcp_servers]")
	fetch, ok := servers["fetch"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "uvx", fetch["command"])
	require.Equal(t, "read-only", cfg["sandbox_mode"])
	// subagent rendered as TOML
	require.FileExists(t, filepath.Join(dest, "agents", "code-reviewer.toml"))
}

func TestOpenCodeConfigShape(t *testing.T) {
	h, _ := Get("opencode")
	req, dest := reqFor(t, h)
	_, err := h.Materialize(req)
	require.NoError(t, err)

	var cfg map[string]any
	require.NoError(t, json.Unmarshal([]byte(readFile(t, dest, "opencode.json")), &cfg))
	mcp := cfg["mcp"].(map[string]any)
	fetch := mcp["fetch"].(map[string]any)
	require.Equal(t, "local", fetch["type"])
	perm := cfg["permission"].(map[string]any)
	require.Equal(t, "deny", perm["edit"], "read-only persona must deny edit")
	require.FileExists(t, filepath.Join(dest, "agent", "code-reviewer.md"))
}

func TestGeminiRenamesUnderscoreServers(t *testing.T) {
	h, _ := Get("gemini")
	req, dest := reqFor(t, h)
	_, err := h.Materialize(req)
	require.NoError(t, err)

	var settings map[string]any
	require.NoError(t, json.Unmarshal([]byte(readFile(t, dest, ".gemini", "settings.json")), &settings))
	servers := settings["mcpServers"].(map[string]any)
	require.Contains(t, servers, "remote-api", "underscore server must be renamed")
	require.NotContains(t, servers, "remote_api")
}

func TestKimiMCPOneToOne(t *testing.T) {
	h, _ := Get("kimi")
	req, dest := reqFor(t, h)
	_, err := h.Materialize(req)
	require.NoError(t, err)

	var cfg map[string]any
	require.NoError(t, json.Unmarshal([]byte(readFile(t, dest, "mcp.json")), &cfg))
	servers := cfg["mcpServers"].(map[string]any)
	require.Contains(t, servers, "fetch")
	require.Contains(t, servers, "remote_api") // Kimi keeps Claude shape verbatim
}

func TestAntigravityRemoteUsesServerURL(t *testing.T) {
	h, _ := Get("antigravity")
	req, dest := reqFor(t, h)
	_, err := h.Materialize(req)
	require.NoError(t, err)

	var cfg map[string]any
	require.NoError(t, json.Unmarshal([]byte(readFile(t, dest, ".gemini", "config", "mcp_config.json")), &cfg))
	servers := cfg["mcpServers"].(map[string]any)
	remote := servers["remote_api"].(map[string]any)
	require.Equal(t, "https://api.example.com/mcp", remote["serverUrl"])
	require.NotContains(t, remote, "url")
}

func TestAgentsTargetFlattensToProse(t *testing.T) {
	h, _ := Get("agents")
	req, dest := reqFor(t, h)
	r, err := h.Materialize(req)
	require.NoError(t, err)

	md := readFile(t, dest, "AGENTS.md")
	require.Contains(t, md, "Available skills")
	require.Contains(t, md, "security-review")
	require.Contains(t, md, "Subagent roles")
	// MCP cannot be represented in AGENTS.md
	require.False(t, walkContains(t, dest, "mcp"))
	require.NotEmpty(t, r.Dropped)
}

// TestMaterializeIdempotent verifies the clean-then-build contract holds for an
// export target as well as the reference harness.
func TestMaterializeIdempotent(t *testing.T) {
	for _, id := range []string{"claude", "opencode", "gemini"} {
		t.Run(id, func(t *testing.T) {
			h, _ := Get(id)
			req, dest := reqFor(t, h)
			_, err := h.Materialize(req)
			require.NoError(t, err)
			first := snapshotTree(t, dest)
			_, err = h.Materialize(req)
			require.NoError(t, err)
			require.Equal(t, first, snapshotTree(t, dest), "second materialize must be byte-identical")
		})
	}
}

// TestOpenCodeLaunchPointsAtConfigFile guards the fix that OPENCODE_CONFIG_DIR
// alone does not load opencode.json — the explicit config-file env var is needed.
func TestOpenCodeLaunchPointsAtConfigFile(t *testing.T) {
	h, _ := Get("opencode")
	req, dest := reqFor(t, h)
	_, err := h.Materialize(req)
	require.NoError(t, err)
	spec := h.Launch(req)
	require.Contains(t, spec.Env, "OPENCODE_CONFIG="+filepath.Join(dest, "opencode.json"))
}

// TestGeminiSSEUsesURLKey guards the fix that Gemini selects MCP transport by
// key: an SSE server must use "url" (SSE) and a streamable server "httpUrl".
func TestGeminiSSEUsesURLKey(t *testing.T) {
	m, err := parseMCP([]byte(`{"mcpServers":{"legacy":{"type":"sse","url":"https://h/sse"},"stream":{"type":"http","url":"https://h/mcp"}}}`))
	require.NoError(t, err)
	servers, _ := mcpGemini(m)
	legacy := servers["legacy"].(map[string]any)
	require.Equal(t, "https://h/sse", legacy["url"])
	require.NotContains(t, legacy, "httpUrl")
	stream := servers["stream"].(map[string]any)
	require.Equal(t, "https://h/mcp", stream["httpUrl"])
}

func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	require.NoError(t, filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out[rel] = string(b)
		return nil
	}))
	return out
}
