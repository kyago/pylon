# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

<!-- SPECKIT START -->
For additional context about technologies to be used, project structure,
shell commands, and other important information, read the current plan
<!-- SPECKIT END -->

## What this repo is

This repository is the **source of the `pylon` binary**, not a pylon workspace. `pylon` is a Go CLI that
prepares a workspace and then hands off to Claude Code. It is a thin *launcher*: `pylon` (no subcommand)
regenerates a `.claude/` directory + `CLAUDE.md` from `.pylon/` and then **replaces its own process** with
`claude` via `syscall.Exec` (`internal/cli/launch.go`). All actual orchestration happens inside the Claude
Code TUI via slash commands and bash scripts — the Go code never runs the pipeline itself.

Do not confuse the two `CLAUDE.md` files: *this* one guides development of the binary; the workspace
`CLAUDE.md` is generated at runtime by `buildRootCLAUDEMD` (`internal/cli/launch_claudemd.go`) and is the
root agent's system prompt. Editing a generated workspace file has no effect on the binary.

## Build / test / lint

```bash
make build   # go build -ldflags "-X main.version=..." -o bin/pylon ./cmd/pylon
make test    # go test ./... -race -count=1   (CI runs exactly this)
make lint    # golangci-lint run ./...        (no repo config; uses golangci-lint defaults)
make install # go install to $(go env GOPATH)/bin
```

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
- `internal/cli/hooks.json`     → Claude Code session hooks

**To change agent behavior, pipeline steps, or slash commands, edit these embedded files** — not any
`.pylon/` copy. `pylon init` writes them out; `pylon doctor` / `pylon update` reconcile existing
workspaces against the embedded versions.

### `.pylon/` (source of truth, git-tracked) vs `.claude/` (generated, git-ignored)

In a *runtime workspace*, `.pylon/` holds config + resources and `.claude/` is regenerated on every launch:
CLAUDE.md, `.claude/agents/` symlinks into `.pylon/agents/`, `.claude/commands/`, and
`.claude/settings.json` hooks. `generateClaudeDir` (`launch.go`) is the generator; it bootstraps missing
`.pylon/` files from embeds but never overwrites user customizations. `runLaunch` also appends `.claude/`,
`CLAUDE.md`, `.pylon/logs/` to `.gitignore`.

### `doctor` is the reconciliation engine, shared with launch

Desired-state for `.claude/commands/` is computed by `buildDesiredClaudeCommands` and applied by both
`generateClaudeDir` and `pylon doctor`, so the launch path and the maintenance path can never drift. When
adding/removing a built-in command, update the embedded set and both consumers stay in sync automatically.

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
