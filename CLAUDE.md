# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this repo is

This repository is the **source of the `pylon` binary**, not a pylon workspace. `pylon` is a Go CLI that
prepares a workspace and then hands off to Claude Code. It is a thin *launcher*: `pylon` (no subcommand)
regenerates a `.claude/` directory + `CLAUDE.md` from `.pylon/` and then **replaces its own process** with
`claude` via `syscall.Exec` (`internal/cli/launch.go`). All actual orchestration happens inside the Claude
Code TUI via slash commands and bash scripts — the Go code never runs the pipeline itself.

Do not confuse the two `CLAUDE.md` files: *this* one guides development of the binary; the workspace
`CLAUDE.md` is generated at runtime and has no effect on the binary when edited.

The workspace root prompt is **generated deterministically from the embedded manual**. `ensureRootAgentFiles`
(`internal/cli/launch_agentsmd.go`) writes workspace `CLAUDE.md` as a deterministic `@AGENTS.md` import
marker on every launch. Inside `AGENTS.md`, pylon owns only the **managed block** delimited by
`<!-- pylon:begin -->` / `<!-- pylon:end -->`: `buildAgentsBlock` fills it with the embedded manual
(`internal/cli/reference/pylon-usage.md`) verbatim plus workspace facts Go knows (project list — never the
root path, which would churn the file when one directory is reached through different paths), and the
block is rewritten whenever its bytes differ — there is no version stamp and no session authoring. Edits
inside the block are reverted; content outside the block is user-owned and never modified, and a user's own
`AGENTS.md` (no block) keeps its content with the block prepended so Codex sees it within the default
instruction size limit (keep the manual well under 32KiB). A malformed marker pair (missing, reversed, or
duplicated whole-line markers) aborts launch/init/doctor before any file is touched; uninstall skips the
file with a warning and proceeds. A block is replaced without a backup only when `isPylonGeneratedBlock`
recognizes it as pylon's own output (this generator's manual title + facts heading and no stamp, or the old
unauthored stub's heading **and** notice sentence); anything else — notably a guide a session authored under
the old `<!-- pylon-usage-version: N -->` scheme — is copied to the next free `AGENTS.md.pylon-bak[.N]` first,
and callers report that exact file name. The stamp regex and old stub strings survive only as these
migration recognizers — the generated block must never contain them. To change what the root agent
is told, edit the manual; the next launch propagates it. Go never calls an LLM.

## Build / test / lint

```bash
make build   # go build -ldflags "-X main.version=..." -o bin/pylon ./cmd/pylon
make test    # go test ./... -race -count=1   (CI runs exactly this)
make lint    # golangci-lint run ./...        (golangci-lint v2, config: .golangci.yml)
make install # go install to $(go env GOPATH)/bin
```

`make lint` needs **golangci-lint v2** (`go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2`).
CI installs it the same way rather than downloading a release binary: prebuilt binaries are built with an
older Go than this repo's `toolchain go1.26.0` and refuse to run. `.golangci.yml` restores the v1 default
exclusions, drops errcheck for `_test.go` setup calls, disables QF1012, and enables the gofmt formatter —
so `make lint` also fails on formatting drift (`golangci-lint fmt` fixes it).

Run a single test / package:

```bash
go test ./internal/cli/ -run TestInit -race -count=1
go test ./internal/memory/... -race
```

`version` is injected via ldflags (`main.version`); a plain `go build` yields `dev`.

## Architecture

### Embedded resources are the source of truth for workspace content

The agents, skills, pipeline commands, and bash scripts that end up in a user's `.pylon/` are **Go-embedded
from `internal/cli/`** (`//go:embed` directives in `init_cmd.go`, `launch_commands.go`, `launch_hooks.go`):

- `internal/cli/agents/*.md`   → 20 agent definitions
- `internal/cli/commands/*.md`  → `/pl:*` slash commands
- `internal/cli/scripts/bash/*.sh` → atomic pipeline steps
- `internal/cli/skills/*.md`    → agent skills
- `internal/cli/reference/*.md` → the pylon usage manual, copied verbatim into the `AGENTS.md` block
- `internal/cli/hooks.json`     → Claude Code session hooks

**To change agent behavior, pipeline steps, or slash commands, edit these embedded files** — not any
`.pylon/builtin/` copy. `pylon init`, every launch, and `pylon doctor` / `pylon update` write them to
`.pylon/builtin/` (see below).

### `.pylon/` (source of truth, git-tracked) vs `.claude/` (generated, git-ignored)

In a *runtime workspace*, `.pylon/` holds config + resources and `.claude/` is regenerated on every launch:
CLAUDE.md, `.claude/agents/` symlinks into `.pylon/builtin/agents/` and `.pylon/agents/`,
`.claude/commands/`, and `.claude/settings.json` hooks. `generateClaudeDir` (`launch.go`) is the generator.
`runLaunch` also appends `.claude/`, `CLAUDE.md`, `AGENTS.md`, `.pylon/logs/`, `.pylon/builtin/` to `.gitignore`.

**Ownership contract for `.pylon/` resources** (`syncBuiltinResources` in `doctor.go`, shared by launch,
init, doctor, sync-agents and cancel): the embedded agents, skills, commands, scripts and manual live in
**`.pylon/builtin/`** (git-ignored), which is pylon-owned. Every sync compares each file with the embeds,
rewrites missing or differing ones (restoring script exec bits) and deletes files the embeds no longer have —
never a wholesale wipe, so concurrent launches converge instead of racing and a damaged builtin heals on the
next launch or doctor. Adding, changing **or deleting** a built-in therefore reaches existing workspaces with
no migration step (`.claude/commands/`, `.claude/agents/` links and injected files, and Codex pointer skills
prune the dropped entries).
The user-owned `.pylon/{agents,skills,commands}/` hold only user resources (`pylon add-agent` /
`pylon add-skill` or a new filename; `scaffoldMarkdownResource` refuses built-in names). **A built-in name
always wins**: a same-named file there (usually a copy left by releases that synced into `.pylon/<kind>/`
directly) is cleared on every sync — deleted when byte-identical to the embed, otherwise moved aside to
`<name>.pylon-bak[.N]` (`pylon doctor` lists each; the launch path's stderr notice is hidden by the TUI). The same
rule covers **retired built-ins**: `retiredBuiltins` (`doctor.go`) maps each removed agent/skill name to the sha256
of its last shipped content, so an untouched copy in `.pylon/<kind>/` is deleted and an edited one is backed up
(`pylon add-agent`/`add-skill` refuse those names). When a built-in is removed, add its name and hash there.
`pylon doctor` also rewrites the pylon hook groups in `.claude/settings.json` from the embedded `hooks.json`, as
launch does. Readers merge both sources —
`workspaceAgents`, `loadAgentSkillMap`, `buildDesiredClaudeCommands`, `generateCodexSkills` — built-in
first. Markdown resources address built-ins as `.pylon/builtin/...` and user agents as `.pylon/agents/...`.
Config (`config.yml`), domain knowledge, memory, history and runtime state are user data and are never
overwritten. Only the workspace-root `.pylon/` is synced — per-project `<project>/.pylon/` directories
are untouched.

### `doctor` is the reconciliation engine, shared with launch

Desired-state for `.claude/commands/` is computed by `buildDesiredClaudeCommands` (built-ins straight from
the embeds, then user commands from `.pylon/commands/`) and applied by both `generateClaudeDir` and
`pylon doctor`, so the launch path and the maintenance path can never drift.

### Package layout

- `cmd/pylon/main.go` — entry point; sets version, calls `cli.Execute()`.
- `internal/cli/` — every Cobra subcommand (`root.go` registers them); `<name>.go` + `<name>_test.go` pairs.
  This is also where embedded resources and the launch/generate logic live.
- `internal/layout/` — **all** `.pylon/` and `.claude/` path construction. Never hand-build these paths;
  use `layout.*` helpers so the directory contract lives in one place.
- `internal/config/` — `config.yml` schema + loading. Only `version` is required; every other field is
  defaulted at load time (`LoadConfig`). `FindWorkspaceRoot` walks up to locate `.pylon/`.
- `internal/memory/` — markdown-file memory store (`<category>/<slug>.md` + `INDEX.md`), token-match search.
- `internal/history/` — directory-snapshot work history under `.pylon/history/pipelines/`.
- `internal/criteria/`, `internal/evaluator/`, `internal/provider/`, `internal/trajectory/` — verification
  gates behind `pylon internal {criteria,evaluator,trajectory,verify}`: immutable criteria snapshots, isolated
  evaluator bundles/verdicts, and create-once task reports / failure records.
- `internal/domain/`, `internal/slug/`, `internal/fsutil/` — supporting utilities.

`resolveRoot()` (`workspace.go`) is the shared workspace-locator used by all commands; reuse it rather than
re-implementing discovery.

## Conventions

- **Go 1.24+** (toolchain go1.26). Dependencies are minimal by design: Cobra (CLI), charmbracelet/huh
  (interactive prompts), yaml.v3. Keep it that way — no heavyweight additions without reason.
- **User-facing strings are Korean**; code identifiers, comments-of-record, and errors wrapped for
  developers may be English. Match the surrounding file.
- Global CLI flags: `--workspace`, `--verbose/-v`, `--json`. Commands that output data should honor `--json`.
- **Versioning is CalVer** `YYYY.M.SEQ` (e.g. `2026.6.3`), *not* semver. Release via `make tag TAG=2026.M.N`
  (validates format + clean tree, pushes tag → goreleaser). Because tags aren't `vX.Y.Z`, `go install @<ver>`
  does not resolve to a release.
