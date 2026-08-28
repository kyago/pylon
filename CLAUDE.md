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

The workspace root prompt is **AI-authored, not hardcoded in Go**. `ensureRootAgentFiles`
(`internal/cli/launch_agentsmd.go`) writes workspace `CLAUDE.md` as a deterministic `@AGENTS.md` import
marker on every launch. Inside `AGENTS.md`, pylon owns only the **managed block** delimited by
`<!-- pylon:begin -->` / `<!-- pylon:end -->`: the block is re-bootstrapped **only when missing or
stale**, content outside the block is user-owned and never modified, and a user's own `AGENTS.md`
(no block) keeps its content with the block prepended so Codex sees it within the default instruction
size limit. A malformed marker pair (missing, reversed, or
duplicated whole-line markers) aborts launch/init/doctor before any file is touched; uninstall skips the
file with a warning and proceeds. Staleness is a version stamp comparison inside the
block: `<!-- pylon-usage-version: N -->` is stale when absent, unparseable, or below `pylonUsageVersion`.
The launched claude session authors the real guide inside the block on its first turn,
reading `.pylon/reference/pylon-usage.md` (the embedded manual) plus the actual workspace. Go never calls
an LLM. Bump `pylonUsageVersion` **only** when the embedded manual changes — it is the sole trigger that
forces re-authoring, and it is independent of pylon's CalVer release version.

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

- `internal/cli/agents/*.md`   → 38 agent definitions
- `internal/cli/commands/*.md`  → `/pl:*` slash commands
- `internal/cli/scripts/bash/*.sh` → atomic pipeline steps
- `internal/cli/skills/*.md`    → agent skills
- `internal/cli/reference/*.md` → the pylon usage manual the session reads to author `AGENTS.md`
- `internal/cli/hooks.json`     → Claude Code session hooks

**To change agent behavior, pipeline steps, or slash commands, edit these embedded files** — not any
`.pylon/` copy. `pylon init` writes them out; `pylon doctor` / `pylon update` reconcile existing
workspaces against the embedded versions.

### `.pylon/` (source of truth, git-tracked) vs `.claude/` (generated, git-ignored)

In a *runtime workspace*, `.pylon/` holds config + resources and `.claude/` is regenerated on every launch:
CLAUDE.md, `.claude/agents/` symlinks into `.pylon/agents/`, `.claude/commands/`, and
`.claude/settings.json` hooks. `generateClaudeDir` (`launch.go`) is the generator. `runLaunch` also appends
`.claude/`, `CLAUDE.md`, `AGENTS.md`, `.pylon/logs/` to `.gitignore`.

**Ownership contract for `.pylon/` resources** (`syncPylonResources` in `doctor.go`, shared by launch and
doctor): a file in `.pylon/{agents,skills,commands,scripts/bash,reference}/` **whose name ships with the binary is
pylon-owned** — it is refreshed to the embedded content on every launch and on `pylon doctor`, so editing
one in place is not a supported customization and the edit is reverted. `pylon doctor` lists the reverted
files by name; the launch path writes the same notice to stderr, but the Claude Code TUI takes over the
terminal immediately afterwards, so treat doctor as the reliable reporting channel.
Any **other** file in those directories is user-owned and is never written or removed: that is the
sanctioned way to customize, via `pylon add-agent` / `pylon add-skill` or simply a new filename.
Config (`config.yml`), domain knowledge, memory, history and runtime state are user data and are never
overwritten. Only the workspace-root `.pylon/` is synced — per-project `<project>/.pylon/` directories
are untouched.

Adding an embedded resource or changing its content reaches existing workspaces automatically, with no
migration step. **Deleting one does not**: the sync iterates embedded names, so a resource dropped from
the binary lingers in `.pylon/` forever. Removing a built-in for real still needs a migration step.

### `doctor` is the reconciliation engine, shared with launch

Desired-state for `.claude/commands/` is computed by `buildDesiredClaudeCommands` and applied by both
`generateClaudeDir` and `pylon doctor`, so the launch path and the maintenance path can never drift. When
adding a built-in command, update the embedded set and both consumers stay in sync automatically.
Removal is the exception: `desired` is computed from `.pylon/commands/`, which the sync never prunes, so a
command deleted from the embeds survives in both `.pylon/commands/` and `.claude/commands/`.

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
