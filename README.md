<div align="center">

# Agent Containers

### Docker for your Claude Code agents.

**Version, swap, and share the config layer that shapes how your agent thinks —
`CLAUDE.md` plus `.claude/` — as isolated, named personas.**

[![Website](https://img.shields.io/badge/website-live-d97757.svg)](https://agent-containers.alexanderangerer.com)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](go.mod)
[![Status](https://img.shields.io/badge/status-alpha-orange.svg)]()
[![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Linux-lightgrey.svg)]()

</div>

---

## Why

Claude Code's behaviour is not just the model — it is everything in `CLAUDE.md`
and `.claude/`: the skills it can call, the subagents it can spawn, the MCP
servers it can reach, the permissions it holds. Today that lives as **one global
blob** per machine. You cannot cleanly swap *"a coder loaded with build skills"*
for *"a reviewer that deliberately lacks them"*, you cannot version those setups,
and you cannot hand them to a teammate.

**Agent Containers** treats that config layer as a versioned, swappable,
shareable **persona** — a container. One workspace, many interchangeable agents,
each tuned for one job. Switch with a single command. Snapshot it like code.
Push it to your team.

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
            versioned  ·  layered  ·  shareable persona containers
```

Activation is done by pointing `CLAUDE_CONFIG_DIR` at a materialized container
**outside** your repository. Your workspace's own `.claude/` is never modified,
so containers can coexist with whatever is already there.

## The killer feature: an uncontaminated reviewer

If the same agent that wrote the code also reviews it, its judgment is shaped by
the very skills and habits that produced the bug. Agent Containers lets you build
a reviewer that is **provably stripped** of the coder's powers — no build skills,
no write access — and then *attests* it:

```console
$ acon new reviewer --template reviewer
$ acon verify reviewer

Persona: reviewer  (uncontaminated)   reviewer:0.1.0
  Skills:   code-review, security-audit
  Withheld  skills [build, codegen, refactor]  (deliberately removed)
  Denied:   Write, Edit, NotebookEdit
  Settings: user+project
```

The withholding is enforced, not advisory: write tools are denied through
`permissions.deny`, and only allow-listed skills are materialized. `verify`
fails loudly if the materialized environment ever drifts from the manifest.

## Quickstart

```bash
# Build the CLI (Go 1.26+)
go build -o acon ./cmd/acon

# ...or install it directly
go install github.com/a2ngerer/agent-containers/cmd/acon@latest
```

```bash
# Bind the current workspace; seeds a _base persona from your existing
# .claude/ and CLAUDE.md. Nothing in your repo is altered.
acon init

# Scaffold a specialized container from an embedded template
acon new coder    --template coder
acon new reviewer --template reviewer

# Activate one. Prints the launch command (or use --exec to start Claude directly).
acon use coder
#   -> CLAUDE_CONFIG_DIR=~/.acon/cache/<hash>/coder claude
#   Start (or restart) Claude Code in this directory to use this environment.

# Swap to the reviewer for the review pass
acon use reviewer
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
slipped in. A persona that leaks a credential is an instant trust killer, so the
scan is mandatory, not optional.

```bash
# Maintainer: publish the team's personas
acon push git@github.com:acme/agent-personas.git

# Teammate: onboard a fresh workspace straight from the shared repo
acon init --from git@github.com:acme/agent-personas.git
acon pull         # later: fetch updates
```

## Command reference

| Command | What it does |
| --- | --- |
| `init [--from <remote>]` | Bind the workspace; seed `_base` locally, or clone a shared repo |
| `new <name> [--template coder\|reviewer] [--extends] [--from]` | Create a persona |
| `use <persona> [--exec]` | Activate a persona for this workspace |
| `deactivate` | Clear the active persona |
| `list` · `status` · `show <persona>` | Inspect personas and the active one |
| `snapshot <persona> -m <msg>` | Record an immutable snapshot |
| `log <persona>` · `diff <a> <b>` | History and capability delta |
| `tag <persona> <semver>` · `rollback <persona> <snap>` | Tag / restore |
| `edit <persona>` · `rm <persona>` | Edit `persona.toml` / remove |
| `push [remote]` · `pull [remote]` · `clone <remote>` | Share over git (secret-scanned) |
| `verify <persona>` | Re-attest that the materialized env matches the manifest |

## How it works

- **Persona** — a versioned bundle: a `CLAUDE.md`, selected skills/subagents/MCP
  servers, setting sources, and an enforcement block (permission mode, tool
  allow/deny). Personas can **layer** on top of `_base`.
- **Storage** — personas live in a hidden, git-backed object store with their own
  domain model on top. Git is an implementation detail behind a `StorageEngine`
  interface, not something you interact with directly.
- **Activation** — composing a persona materializes a self-contained
  `CLAUDE_CONFIG_DIR` and points Claude Code at it. Withheld skills are physically
  absent; denied tools are blocked by settings; `verify` enforces the match.
- **Isolation** — materialization rejects symlinks and path-traversal names, and
  forces strict MCP isolation, so a shared persona cannot smuggle in host files
  or a project's MCP servers.

> `acon` is the **Agent Containers** CLI — at heart, git for your Claude config.
> Each persona is a container you swap in for the task at hand: snapshot it, tag
> it, share it, roll it back, just like code.

## Project layout

```text
cmd/acon        CLI entrypoint
internal/domain       persona / snapshot / enforcement types
internal/storage      git-backed StorageEngine
internal/environment  workspace binding, CloneInto
internal/compose      layering + capability diff
internal/materialize  render a persona into a CLAUDE_CONFIG_DIR
internal/enforce      permission set + drift verification
internal/activate     compose -> materialize -> verify -> launch
internal/share        push / pull / clone + secret scanning
internal/cli          cobra commands
docs/                 design spec and milestone plans
```

## Status

Alpha. The core — versioning, layering, activation, the enforced uncontaminated
reviewer, and secret-safe sharing — works and is covered by tests. Interfaces may
still change before a tagged release.

## Contributing

Issues and pull requests are welcome. Run `go test ./...` before submitting; the
suite is the contract.

## License

[MIT](LICENSE) © 2026 Alexander Angerer
