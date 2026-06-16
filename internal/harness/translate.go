package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/a2ngerer/agent-containers/internal/safecopy"
)

// ---------------------------------------------------------------------------
// MCP: a neutral model parsed from the persona's Claude-style mcp.json, plus
// per-target re-encoders. Claude's mcp.json is the canonical source shape:
// {"mcpServers": {"<name>": {command,args,env,cwd | type,url,headers}}}.
// ---------------------------------------------------------------------------

type mcpServer struct {
	Command   string
	Args      []string
	Env       map[string]string
	Cwd       string
	Transport string // "stdio" | "http" | "sse"
	URL       string
	Headers   map[string]string
}

func (s mcpServer) remote() bool { return s.Command == "" && s.URL != "" }

type mcpModel struct {
	Names   []string // sorted, for deterministic output
	Servers map[string]mcpServer
}

func (m mcpModel) empty() bool { return len(m.Names) == 0 }

// readMCP loads + parses the persona's mcp.json (Manifest.MCP.Config, relative to
// PersonaDir). A persona with no MCP config, or a missing/empty file, yields an
// empty model rather than an error.
func readMCP(req Request) (mcpModel, error) {
	cfg := req.Manifest.MCP.Config
	if cfg == "" {
		return mcpModel{Servers: map[string]mcpServer{}}, nil
	}
	raw, err := os.ReadFile(filepath.Join(req.PersonaDir, cfg))
	if err != nil {
		if os.IsNotExist(err) {
			return mcpModel{Servers: map[string]mcpServer{}}, nil
		}
		return mcpModel{}, fmt.Errorf("read mcp config %q: %w", cfg, err)
	}
	return parseMCP(raw)
}

func parseMCP(raw []byte) (mcpModel, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return mcpModel{Servers: map[string]mcpServer{}}, nil
	}
	var wire struct {
		MCPServers map[string]struct {
			Type      string            `json:"type"`
			Command   string            `json:"command"`
			Args      []string          `json:"args"`
			Env       map[string]string `json:"env"`
			Cwd       string            `json:"cwd"`
			URL       string            `json:"url"`
			HTTPURL   string            `json:"httpUrl"`
			ServerURL string            `json:"serverUrl"`
			Headers   map[string]string `json:"headers"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return mcpModel{}, fmt.Errorf("parse mcp json: %w", err)
	}
	m := mcpModel{Servers: make(map[string]mcpServer, len(wire.MCPServers))}
	for name, s := range wire.MCPServers {
		url := firstNonEmpty(s.URL, s.HTTPURL, s.ServerURL)
		transport := s.Type
		if transport == "" {
			if url != "" {
				transport = "http"
			} else {
				transport = "stdio"
			}
		}
		m.Servers[name] = mcpServer{
			Command: s.Command, Args: s.Args, Env: s.Env, Cwd: s.Cwd,
			Transport: transport, URL: url, Headers: s.Headers,
		}
		m.Names = append(m.Names, name)
	}
	sort.Strings(m.Names)
	return m, nil
}

// mcpClaudeStyle re-emits the servers in Claude/Kimi shape: mcpServers map with
// command/args/env (stdio) or type/url/headers (remote).
func mcpClaudeStyle(m mcpModel) map[string]any {
	servers := map[string]any{}
	for _, name := range m.Names {
		s := m.Servers[name]
		if s.remote() {
			servers[name] = compact(map[string]any{"type": s.Transport, "url": s.URL, "headers": mapAny(s.Headers)})
		} else {
			servers[name] = compact(map[string]any{"command": s.Command, "args": strAny(s.Args), "env": mapAny(s.Env)})
		}
	}
	return map[string]any{"mcpServers": servers}
}

// mcpGemini re-emits servers for the mcpServers block inside Gemini settings.json.
// Gemini's policy parser splits the tool FQN on underscores, so server names must
// not contain underscores; renamed names are returned for the report.
func mcpGemini(m mcpModel) (servers map[string]any, renamed [][2]string) {
	servers = map[string]any{}
	for _, name := range m.Names {
		s := m.Servers[name]
		clean := strings.ReplaceAll(name, "_", "-")
		if clean != name {
			renamed = append(renamed, [2]string{name, clean})
		}
		switch {
		case s.remote() && s.Transport == "sse":
			// Gemini selects the transport by key: url -> SSE, httpUrl -> streamable
			// HTTP. An SSE endpoint under httpUrl silently loads zero tools.
			servers[clean] = compact(map[string]any{"url": s.URL, "headers": mapAny(s.Headers)})
		case s.remote():
			servers[clean] = compact(map[string]any{"httpUrl": s.URL, "headers": mapAny(s.Headers)})
		default:
			servers[clean] = compact(map[string]any{"command": s.Command, "args": strAny(s.Args), "env": mapAny(s.Env)})
		}
	}
	return servers, renamed
}

// mcpOpenCode re-emits servers for the opencode.json "mcp" object: local servers
// fold command+args into a single command[] array under "environment".
func mcpOpenCode(m mcpModel) map[string]any {
	servers := map[string]any{}
	for _, name := range m.Names {
		s := m.Servers[name]
		if s.remote() {
			servers[name] = compact(map[string]any{"type": "remote", "url": s.URL, "headers": mapAny(s.Headers), "enabled": true})
		} else {
			cmd := append([]string{s.Command}, s.Args...)
			servers[name] = compact(map[string]any{"type": "local", "command": strAny(cmd), "environment": mapAny(s.Env), "enabled": true})
		}
	}
	return servers
}

// mcpAntigravity re-emits servers for ~/.gemini/config/mcp_config.json: remote
// servers use serverUrl (Antigravity's canonical key) rather than url.
func mcpAntigravity(m mcpModel) map[string]any {
	servers := map[string]any{}
	for _, name := range m.Names {
		s := m.Servers[name]
		if s.remote() {
			servers[name] = compact(map[string]any{"serverUrl": s.URL, "headers": mapAny(s.Headers)})
		} else {
			servers[name] = compact(map[string]any{"command": s.Command, "args": strAny(s.Args), "env": mapAny(s.Env)})
		}
	}
	return map[string]any{"mcpServers": servers}
}

// ---------------------------------------------------------------------------
// Subagents: parse a Claude agents/<name>.md (YAML-ish frontmatter + body) into
// a neutral struct. Frontmatter is simple key: value, so a line parser avoids a
// YAML dependency; an absent frontmatter block leaves the whole file as the body.
// ---------------------------------------------------------------------------

type subagent struct {
	Name        string
	Description string
	Model       string
	Tools       string
	Body        string
}

func readSubagent(req Request, name string) (subagent, error) {
	raw, err := os.ReadFile(filepath.Join(req.PersonaDir, "agents", name+".md"))
	if err != nil {
		return subagent{}, fmt.Errorf("read subagent %q: %w", name, err)
	}
	return parseSubagent(name, raw), nil
}

func parseSubagent(fallbackName string, raw []byte) subagent {
	front, body := splitFrontmatter(string(raw))
	return subagent{
		Name:        firstNonEmpty(front["name"], fallbackName),
		Description: front["description"],
		Model:       front["model"],
		Tools:       front["tools"],
		Body:        strings.TrimSpace(body),
	}
}

// splitFrontmatter separates a leading --- ... --- YAML block from the body. It
// returns a flat key->value map (only top-level scalar keys) and the remainder.
func splitFrontmatter(s string) (map[string]string, string) {
	front := map[string]string{}
	lines := strings.Split(s, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return front, s
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return front, s
	}
	for _, ln := range lines[1:end] {
		if i := strings.IndexByte(ln, ':'); i > 0 {
			k := strings.TrimSpace(ln[:i])
			v := strings.TrimSpace(ln[i+1:])
			v = strings.Trim(v, `"'`)
			if k != "" {
				front[k] = v
			}
		}
	}
	return front, strings.Join(lines[end+1:], "\n")
}

// ---------------------------------------------------------------------------
// Skills: copy allowlisted skill dirs, and fold skills/subagents into prose for
// targets that cannot represent them natively.
// ---------------------------------------------------------------------------

// copySkillsInto copies each allowlisted skill dir from <PersonaDir>/skills/<n>
// into <destSkillsRoot>/<n>. Skills the manifest lists but that are absent on
// disk are returned as missing (the caller notes them) rather than failing, so a
// cross-harness export is forgiving of a base layer that references host skills.
func copySkillsInto(req Request, destSkillsRoot string) (copied []string, missing []string, err error) {
	for _, name := range req.Manifest.Skills {
		if !filepath.IsLocal(name) {
			return copied, missing, fmt.Errorf("invalid skill name %q", name)
		}
		src := filepath.Join(req.PersonaDir, "skills", name)
		if fi, statErr := os.Stat(src); statErr != nil || !fi.IsDir() {
			missing = append(missing, name)
			continue
		}
		if err := safecopy.Tree(src, filepath.Join(destSkillsRoot, name)); err != nil {
			return copied, missing, fmt.Errorf("copy skill %q: %w", name, err)
		}
		copied = append(copied, name)
	}
	return copied, missing, nil
}

// skillDescription reads the description from a skill's SKILL.md frontmatter.
func skillDescription(req Request, name string) string {
	raw, err := os.ReadFile(filepath.Join(req.PersonaDir, "skills", name, "SKILL.md"))
	if err != nil {
		return ""
	}
	front, _ := splitFrontmatter(string(raw))
	return front["description"]
}

// skillsProse renders the allowlisted skills as a Markdown section, for targets
// (Kimi/Antigravity/AGENTS.md) that have no skill-dir analog.
func skillsProse(req Request) string {
	if len(req.Manifest.Skills) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Available skills\n\n")
	for _, name := range req.Manifest.Skills {
		if d := skillDescription(req, name); d != "" {
			fmt.Fprintf(&b, "- **%s** — %s\n", name, d)
		} else {
			fmt.Fprintf(&b, "- **%s**\n", name)
		}
	}
	return b.String()
}

// subagentsProse renders the subagents as a Markdown section, for targets that
// cannot predeclare subagents as files.
func subagentsProse(req Request) string {
	if len(req.Manifest.Subagents) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Subagent roles\n\n")
	for _, name := range req.Manifest.Subagents {
		sa, err := readSubagent(req, name)
		if err != nil || sa.Description == "" {
			fmt.Fprintf(&b, "- **%s**\n", name)
			continue
		}
		fmt.Fprintf(&b, "- **%s** — %s\n", name, sa.Description)
	}
	return b.String()
}

// composeInstructions joins the persona instructions (the composed CLAUDE.md
// body) with any extra prose sections, skipping empties.
func composeInstructions(base string, sections ...string) string {
	parts := []string{}
	if s := strings.TrimSpace(base); s != "" {
		parts = append(parts, s)
	}
	for _, sec := range sections {
		if s := strings.TrimSpace(sec); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "\n\n") + "\n"
}

// ---------------------------------------------------------------------------
// Output helpers: deterministic so two materializations are byte-identical.
// ---------------------------------------------------------------------------

// writeJSON marshals v with sorted keys (Go's default for maps), two-space
// indent, and a trailing newline, then writes it 0644.
func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}
	data = append(data, '\n')
	return writeFile(path, data)
}

// writeFile creates parent dirs and writes data 0644.
func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create dir for %q: %w", path, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %q: %w", path, err)
	}
	return nil
}

// cleanDest removes and recreates destDir so Materialize is a clean rebuild.
func cleanDest(destDir string) error {
	if err := os.RemoveAll(destDir); err != nil {
		return fmt.Errorf("clean dest dir: %w", err)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}
	return nil
}

// --- small generic helpers ---

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// compact drops nil/empty slice/map/"" values so emitted JSON stays minimal.
func compact(m map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range m {
		switch t := v.(type) {
		case nil:
			continue
		case string:
			if t == "" {
				continue
			}
		case []any:
			if len(t) == 0 {
				continue
			}
		case map[string]any:
			if len(t) == 0 {
				continue
			}
		}
		out[k] = v
	}
	return out
}

func strAny(ss []string) []any {
	if len(ss) == 0 {
		return nil
	}
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

func mapAny(m map[string]string) map[string]any {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
