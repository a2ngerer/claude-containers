# Multi-Harness Support — Design

Status: design locked, 2026-06-16. Implements the "make a persona portable across
agent harnesses" follow-up. Source-of-truth research (config layouts + activation
mechanisms) was gathered per harness and adversarially verified; the per-harness
facts below are the corrected, load-bearing ones.

## Goal

Today `acon` materializes a persona into Claude Code's config layout (`.claude/` +
`CLAUDE.md`) and activates it by pointing `CLAUDE_CONFIG_DIR` at it. Generalize this
so a persona can be materialized into **any** of six harnesses, and — the headline
feature — so a setup authored for Claude Code can be **exported and run in another
harness** (e.g. OpenCode).

**Claude Code is the canonical source.** A persona is authored in Claude's
vocabulary (instructions, skills, subagents, MCP, permissions). Other harnesses are
**export targets**: each adapter renders the neutral manifest into that harness's
layout and reports, honestly, what translated cleanly and what was lost. Import
*from* other harnesses is out of scope for this iteration.

## Architecture

The existing domain model (`ResolvedManifest`: instructions + skills + subagents +
MCP + enforcement) is already nearly harness-neutral. It becomes the **neutral
core**. A new `internal/harness` package adds an adapter per harness behind one
interface:

```go
type Harness interface {
    ID() string            // "claude" | "codex" | "opencode" | "gemini" | "kimi" | "antigravity"
    DisplayName() string
    // Materialize renders rm into destDir in this harness's layout and returns a
    // translation report (included artifacts + lossiness). Idempotent.
    Materialize(rm compose.ResolvedManifest, destDir string) (Report, error)
    // Launch returns the env + argv to start this harness against destDir.
    Launch(rm compose.ResolvedManifest, destDir string) LaunchSpec
    // Detect reports whether this harness looks installed/configured on the host.
    Detect() Detection
}
```

- **Claude adapter** wraps the existing `materialize` / `enforce.Verify` /
  `activate.BuildLaunch` code so the Claude path stays byte-identical and all
  existing tests keep passing. Its `Report` is the existing attestation.
- The other five adapters implement their own `Materialize`/`Launch`.
- A registry resolves an `ID` to a `Harness`; adapters self-register via `init()`
  so no central file needs editing per adapter.

`acon use` / `acon export` resolve the target harness, call `Materialize`, print the
report, then `Launch` (use) or stop (export).

### The translation report — honest lossiness

The report is the new trust artifact, in the same spirit as the "enforced, not
promised" attestation: it states exactly what crossed the boundary intact and what
degraded or dropped.

```
Exported  reviewer  ->  opencode      reviewer:0.1.0
  instructions   AGENTS.md                              ok
  skills         3  (SKILL.md, 1:1)                     ok
  subagents      2  -> agent/*.md (frontmatter remapped) translated
  mcp            mcp.json -> opencode "mcp" (strict)     translated
  permissions    read-only -> permission.edit=deny      degraded
  dropped        permission_mode sandbox nuance; hooks
```

`Report{ Harness, Persona, Version, Lines []ReportLine, Dropped []LossyNote }`
where each line carries a `Status` of `ok | translated | degraded`.

## Harness selection UX (hybrid)

Resolution order for any command needing a target: `--harness <id>` flag >
workspace default (`default_harness` in `env.toml`) > `claude` (reference default).

- `acon config harness <id>` sets the workspace default; `acon config harness`
  prints it; `acon config` prints all settings.
- `--harness <id>` overrides per command on `use` / `export`.
- `acon init` auto-detects installed harnesses (binary on `PATH` + known config
  dirs), prints them, and seeds the default (prefers `claude`, else the first
  detected). The persona's *source* is still Claude's `.claude/` + `CLAUDE.md`.

## Per-harness facts (verified)

Two activation families:

1. **Config-dir via env var** (5/6) — materialize a dir, set one env var, run the
   binary. Mirrors today's Claude path; only the var name, internal layout, and
   isolation flags differ.
2. **Convention / file-placement** (Antigravity) — no relocation env var; files are
   discovered by location. Best-effort `HOME` override for isolation, flagged.

| Harness | Bin | Activation env | Instructions | Settings | MCP | Subagents | Skills |
|---|---|---|---|---|---|---|---|
| Claude | `claude` | `CLAUDE_CONFIG_DIR=<dir>` | `CLAUDE.md` | `settings.json` (JSON) | `mcp.json` + `--strict-mcp-config` | `agents/*.md` | `skills/<n>/SKILL.md` |
| Codex | `codex` | `CODEX_HOME=<dir>` | `AGENTS.md` | `config.toml` (TOML) | `[mcp_servers.<id>]` in config.toml (no strict flag) | `agents/<n>.toml` (name/description/developer_instructions) | `.agents/skills` — **outside** CODEX_HOME |
| OpenCode | `opencode` | `OPENCODE_CONFIG_DIR=<dir>` (+ `OPENCODE_DISABLE_PROJECT_CONFIG=1`, `OPENCODE_DISABLE_CLAUDE_CODE_PROMPT=1`, `OPENCODE_DISABLE_CLAUDE_CODE_SKILLS=1`) | `AGENTS.md` | `opencode.json` | `"mcp"` key: local `{type:'local',command:[...],environment}` / remote `{type:'remote',url,headers}` | `agent/<n>.md` (frontmatter `permission`,`tools`,`mode:subagent`) | config `skills.paths` |
| Gemini | `gemini` | `GEMINI_CLI_HOME=<dir>` (config root = `<dir>/.gemini`) + `GEMINI_CLI_TRUST_WORKSPACE=true` | `GEMINI.md` (AGENTS.md only via `context.fileName`) | `.gemini/settings.json` | `mcpServers` block **inside** settings.json (no underscores in names) | `.gemini/agents/<n>.md` | `.gemini/skills/<n>/SKILL.md` |
| Kimi (Kimi Code) | `kimi` | `KIMI_CODE_HOME=<dir>` | `AGENTS.md` (via `${KIMI_AGENTS_MD}`) | `config.toml` (TOML) | `mcp.json` (`mcpServers`, 1:1) | YAML spec — **lossy**, folded to prose | `skills/<n>/SKILL.md` (`type: prompt`) |
| Antigravity | `agy` | none — file placement; `HOME=<dir>` best-effort | `AGENTS.md` + `GEMINI.md` | `~/.gemini/antigravity-cli/settings.json` | `~/.gemini/config/mcp_config.json` (`mcpServers`; remote `serverUrl`/`url`) | runtime-only — **dropped**, folded to prose | `skills/<n>/SKILL.md` |

Generic **`agents` target** (AGENTS.md standard, repo `agentsmd/agents.md`): writes a
single `AGENTS.md` (instructions + skills/subagents flattened to prose). MCP and
permissions are not representable and are dropped (must go to a real harness). Not a
runnable harness; an export-only convenience.

### Translation rules (Claude source -> target)

- **instructions** — `CLAUDE.md` body copied verbatim into the target's instructions
  file. Cleanest mapping everywhere.
- **skills** — `SKILL.md` dirs are the Agent Skills open standard; copied 1:1 into
  Codex/OpenCode/Gemini/Kimi/Antigravity. For Codex they land in a `skills/` subdir
  with a report note (Codex discovers skills only under `.agents/skills`, outside
  `CODEX_HOME`). For the `agents` target they flatten to an "Available workflows"
  prose section.
- **subagents** — copied as Markdown for OpenCode/Gemini (frontmatter remapped:
  permission/tools/model form). Reformatted to TOML for Codex
  (`developer_instructions`). Dropped → prose for Kimi/Antigravity/`agents`.
- **mcp** — re-emitted into each target's MCP format: JSON `mcpServers` (Claude,
  Kimi, Antigravity), inside `settings.json` (Gemini, underscores stripped), TOML
  tables (Codex), `"mcp"` object (OpenCode). Strict/empty isolation preserved where
  the target supports it.
- **permissions** — clean for Claude/OpenCode (allow/ask/deny). Lossy elsewhere:
  Codex `approval_policy`+`sandbox_mode`; Gemini approval mode + `tools.allowed`/
  `excluded`; Kimi coarse `--auto`/`--plan`/`--yolo`; Antigravity dropped. The
  read-only guarantee is mapped to the strongest available primitive per target and
  the residue is listed under `dropped`.

The enforced uncontaminated-reviewer guarantee (withheld skills physically absent,
write tools denied, drift-verified) remains **first-class for Claude**. For other
targets the same withholding still happens at materialization (withheld skills are
never written), and the report states which permission residue could not be
enforced natively — never over-claiming enforcement a harness cannot provide.

## Out of scope (this iteration)

- Import *from* a non-Claude harness into a persona.
- Claude hooks / statusline / output-styles translation (acon does not model hooks).
- `acon verify` for non-Claude targets (verify stays Claude-strict; other targets
  rely on the materialization report).
