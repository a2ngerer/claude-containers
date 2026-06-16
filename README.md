<div align="center">

# Agent Containers

### Docker for your coding agents.

**Version, swap, and share the config layer that shapes how your agent thinks —
`CLAUDE.md` plus `.claude/` — as isolated, named personas. Then take any persona
to another harness with one command.**

[![Website](https://img.shields.io/badge/website-live-d97757.svg)](https://agent-containers.alexanderangerer.com)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](go.mod)
[![Status](https://img.shields.io/badge/status-alpha-orange.svg)]()
[![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Linux-lightgrey.svg)]()

</div>

---

## Why

A coding agent's behaviour is not just the model — it is everything in its config
layer: the skills it can call, the subagents it can spawn, the MCP servers it can
reach, the permissions it holds. Today that lives as **one global blob** per
machine, and it is **locked to one harness**. You cannot cleanly swap *"a coder
loaded with build skills"* for *"a reviewer that deliberately lacks them"*, you
cannot version those setups, you cannot hand them to a teammate — and if you spend
weeks tuning Claude Code and then want to try OpenCode, you start over.

**Agent Containers** treats that config layer as a versioned, swappable, shareable
**persona** — a container. One workspace, many interchangeable agents, each tuned
for one job. Switch with a single command. Snapshot it like code. Push it to your
team. **And export it to whatever harness you want to run today.**

```text
                       your workspace  (code repo — .claude/ is never touched)
                                   ▲
                  acon use   │   swaps the active container
                                   │
   ┌──────────────┬────────────────┴───────────────┬──────────────────┐
   │   _base      │   coder                         │   reviewer       │
   │  shared      │   + build / codegen skills      │   NO build skills│
   │  baseline    │   + refactor subagents          │   write: denied  │
   │              │   write: allowed                │   read-only       │
   └──────────────┴─────────────────────────────────┴──────────────────┘
            versioned  ·  layered  ·  shareable  ·  portable across harnesses
```

## The killer feature, part 1: an uncontaminated reviewer

If the same agent that wrote the code also reviews it, its judgment is shaped by
the very skills and habits that produced the bug. Agent Containers lets you build
a reviewer that is **provably stripped** of the coder's powers — no build skills,
no write access — and then *attests* it:

```console
$ acon new reviewer --template reviewer
$ acon verify reviewer

Persona: reviewer  (uncontaminated → claude)   reviewer:0.1.0
  skill:     code-review, security-audit
  Withheld   skills [build, codegen, refactor]  (deliberately removed)
  Denied:    Write, Edit, NotebookEdit
  Settings:  user+project
```

The withholding is enforced, not advisory: write tools are denied through
`permissions.deny`, and only allow-listed skills are materialized. `verify` fails
loudly if the materialized environment ever drifts from the manifest.

## The killer feature, part 2: author once, run on any harness

You tuned your setup in Claude Code. Now you want to try OpenCode — or your
teammate runs Codex, or CI uses Gemini. **Containerize your Claude setup and take
it with you.** `acon export` renders a persona into the target harness's own
config layout and prints an **honest translation report** of what crossed the
boundary intact and what was lost.

```console
$ acon export coder --harness opencode

Persona: coder  (translated → opencode)   coder:0.1.0
  instructions:  AGENTS.md
  skills:        5 -> skills/ (SKILL.md)
  subagents:     2 -> agent/*.md (mode: subagent)   ~translated
  mcp:           "mcp" object (3)                    ~translated
  permissions:   allow/deny -> permission map        ~translated
  dropped   permissions: permission_mode/sandbox nuance has no OpenCode analog

Exported to ./acon-export/opencode/coder
  OPENCODE_CONFIG_DIR=… OPENCODE_DISABLE_PROJECT_CONFIG=1 opencode
```

Claude Code is the canonical **source**; every other harness is an **export
target**. Translation is never silently lossy — the report tells you exactly which
artifacts mapped 1:1, which were re-encoded, and which the target simply cannot
represent. The uncontaminated-reviewer withholding still holds: a withheld skill is
physically absent in OpenCode, Codex, and Gemini too, not just Claude.

### Supported harnesses

| Harness | Activation | Instructions | MCP | Skills | Subagents |
| --- | --- | --- | --- | --- | --- |
| **Claude Code** (source) | `CLAUDE_CONFIG_DIR` | `CLAUDE.md` | `mcp.json` (strict) | native | native |
| **OpenCode** | `OPENCODE_CONFIG_DIR` | `AGENTS.md` | `opencode.json` | native | `agent/*.md` |
| **Codex** | `CODEX_HOME` | `AGENTS.md` | `config.toml` | `skills/` | `agents/*.toml` |
| **Gemini CLI** | `GEMINI_CLI_HOME` | `GEMINI.md` | `settings.json` | native | `agents/*.md` |
| **Kimi Code** | `KIMI_CODE_HOME` | `AGENTS.md` | `mcp.json` | native | folded to prose |
| **Antigravity** | file placement | `AGENTS.md`+`GEMINI.md` | `mcp_config.json` | native | folded to prose |
| **AGENTS.md** | convention | `AGENTS.md` | — | folded to prose | folded to prose |

`acon harnesses` lists them and shows which are installed on your machine.

## Quickstart

```bash
# Build the CLI (Go 1.25+)
go build -o acon ./cmd/acon

# ...or install it directly
go install github.com/a2ngerer/agent-containers/cmd/acon@latest
```

```bash
# Bind the current workspace; seeds a _base persona from your existing
# .claude/ + CLAUDE.md (skills, subagents, MCP and all). Nothing in your repo
# is altered. acon also auto-detects which harnesses you have installed.
acon init

# Scaffold a specialized container from an embedded template
acon new coder    --template coder
acon new reviewer --template reviewer

# Activate one for Claude Code (the default). Prints the launch command.
acon use coder

# ...or activate it for a different harness
acon use coder --harness opencode

# Set a workspace-wide default harness so you can drop the flag
acon config harness opencode

# Take a whole persona to another harness, anywhere on disk
acon export reviewer --harness codex --out ~/work/other-repo
```

## Versioning

Every persona has its own immutable timeline — snapshot, inspect, tag, roll back.

```bash
acon snapshot reviewer -m "tighten the security-audit checklist"
acon log      reviewer            # snapshot history, newest first
acon tag      reviewer 1.0.0      # SemVer tag on the newest snapshot
acon rollback reviewer <snapshot> # restore a prior state as a new snapshot
acon diff     coder reviewer      # capability delta between two personas
```

## Sharing

Push the whole persona repo to any git remote — with a **secret-safe guard** that
scans before every push and aborts if a key, token, or `settings.local.json`
slipped in.

```bash
acon push git@github.com:acme/agent-personas.git
acon init --from git@github.com:acme/agent-personas.git   # teammate onboards
acon pull
```

## Command reference

| Command | What it does |
| --- | --- |
| `init [--from <remote>]` | Bind the workspace; seed `_base` (skills/subagents/MCP enumerated), or clone a shared repo |
| `new <name> [--template coder\|reviewer] [--extends] [--from]` | Create a persona |
| `use <persona> [--harness <id>] [--exec]` | Activate a persona for this workspace |
| `export <persona> --harness <id> [--out <dir>]` | Materialize a persona's config for another harness |
| `harnesses` | List supported harnesses and which are installed |
| `config [harness <id>]` | Show settings; get/set the workspace default harness |
| `deactivate` | Clear the active persona |
| `list` · `status` · `show <persona>` | Inspect personas and the active one |
| `snapshot <persona> -m <msg>` | Record an immutable snapshot |
| `log <persona>` · `diff <a> <b>` | History and capability delta |
| `tag <persona> <semver>` · `rollback <persona> <snap>` | Tag / restore |
| `edit <persona>` · `rm <persona>` | Edit `persona.toml` / remove |
| `push [remote]` · `pull [remote]` · `clone <remote>` | Share over git (secret-scanned) |
| `verify <persona>` | Re-attest that the materialized Claude env matches the manifest |

## How it works

- **Persona** — a versioned bundle: instructions (`CLAUDE.md`), selected
  skills/subagents/MCP servers, setting sources, and an enforcement block
  (permission mode, tool allow/deny). Personas can **layer** on top of `_base`.
- **Storage** — personas live in a hidden, git-backed object store with their own
  domain model on top. Git is an implementation detail behind a `StorageEngine`
  interface, not something you interact with directly.
- **Harness adapters** — the neutral persona manifest is rendered by a per-harness
  adapter (`internal/harness`) that knows that harness's config layout and
  activation mechanism. Claude Code is the reference adapter; the others translate
  and report lossiness. Withheld skills are physically absent in every target.
- **Activation** — composing a persona materializes a self-contained config dir and
  points the harness at it (an env var for five of six harnesses; file placement
  for Antigravity). For Claude, `verify` enforces that the materialized env matches
  the manifest, byte for byte.
- **Isolation** — materialization rejects symlinks and path-traversal names, so a
  shared persona cannot smuggle in host files or a project's MCP servers.

> `acon` is the **Agent Containers** CLI. Each persona is a container you swap in
> for the task at hand and carry to whatever harness you run: snapshot it, tag it,
> share it, roll it back, export it — just like code.

## Project layout

```text
cmd/acon              CLI entrypoint
internal/domain       persona / snapshot / enforcement types
internal/storage      git-backed StorageEngine
internal/environment  workspace binding, CloneInto
internal/compose      layering + capability diff
internal/materialize  render a persona into a Claude CLAUDE_CONFIG_DIR
internal/enforce      permission set + drift verification
internal/harness      harness adapters: materialize + launch + translate per harness
internal/safecopy     untrusted-safe file copy (symlink-rejecting)
internal/activate     compose -> materialize -> verify -> launch
internal/share        push / pull / clone + secret scanning
internal/cli          cobra commands
docs/                 design spec and milestone plans
```

## Status

Alpha. The core — versioning, layering, activation, the enforced uncontaminated
reviewer, secret-safe sharing, and cross-harness export — works and is covered by
tests. Interfaces may still change before a tagged release.

## Contributing

Issues and pull requests are welcome. Run `go test ./...` before submitting; the
suite is the contract.

## License

[MIT](LICENSE) © 2026 Alexander Angerer
