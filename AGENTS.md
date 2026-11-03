# AGENTS.md

This file guides AI coding agents (Claude Code via the `@AGENTS.md` import in `CLAUDE.md`, Codex, and others)
when working on code in this repository.

## What this repo is

This repository is the **source of the `pylon` binary**, not a pylon workspace. `pylon` is a small Go CLI that sets
up a workspace root over several repositories: the root `AGENTS.md`, the project list, a few agents and slash
commands, and a simple markdown memory. It does not start sessions or run anything: the user opens Claude Code, Codex
or Paseo in the workspace, and the session reads `AGENTS.md`. Nothing is injected; the manual tells the session where
to look when it needs something. Go never calls an LLM.

**Direction.** A personal tool, kept to workspace setup and simple memory. Before adding anything, check that it
solves a problem actually hit and that Claude Code, Codex or Paseo does not already cover it. Compose existing tools
by default and build only what needs the multi-repo workspace scope: keep the data as plain markdown, and connect tools
through the CLI and files only. Stage-by-stage execution belongs to the separate Paseo plugin `kyago/paseo-todo-board`.

Do not confuse this file with the workspace `AGENTS.md`/`CLAUDE.md`: *this* one guides development of the binary; the
workspace files are generated and have no effect on the binary when edited.

## Build / test / lint

```bash
make build   # go build -ldflags "-X main.version=..." -o bin/pylon ./cmd/pylon
make test    # go test ./... -race -count=1   (CI runs exactly this)
make lint    # golangci-lint run ./...        (golangci-lint v2, config: .golangci.yml)
make install # go install to $(go env GOPATH)/bin
```

`make lint` needs **golangci-lint v2** (`go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2`).
CI installs it the same way rather than downloading a release binary: prebuilt binaries are built with an older Go
than this repo's `toolchain go1.26.0` and refuse to run. `.golangci.yml` restores the v1 default exclusions, drops
errcheck for `_test.go` setup calls, disables QF1012, and enables the gofmt formatter — so `make lint` also fails on
formatting drift (`golangci-lint fmt` fixes it).

`version` is injected via ldflags (`main.version`); a plain `go build` or `go install` yields `dev`.

## Architecture

### Workspace root files (`internal/cli/launch_agentsmd.go`)

`CLAUDE.md` is a deterministic `@AGENTS.md` import marker. Inside `AGENTS.md`, pylon owns only the **managed block**
delimited by `<!-- pylon:begin -->` / `<!-- pylon:end -->`: `buildAgentsBlock` fills it with the embedded manual
(`internal/cli/reference/pylon-usage.md`) verbatim plus the project list, and the block is rewritten whenever its bytes
differ. Edits inside the block are reverted; content outside the block is user-owned and never modified, and a user's
own `AGENTS.md` (no block) keeps its content with the block prepended so Codex sees it within the default instruction
size limit (keep the manual well under 32KiB). A malformed marker pair aborts init/doctor before any file is touched;
uninstall skips the file with a warning and proceeds. A block is replaced without a backup only when
`isPylonGeneratedBlock` recognizes it as pylon's own output (manual title + facts heading); anything else is copied to
the next free `AGENTS.md.pylon-bak[.N]` first, and callers report that exact file name. To change what the root agent
is told, edit the manual; the next `pylon doctor` propagates it. `add-project` and `delete-project` refresh the project
list themselves.

### Embedded resources are the source of truth

`//go:embed` from `internal/cli/`: `agents/*.md` (dev, code-reviewer, critic), `commands/*.md` (`/pl:cleanup`,
`/pl:add-project`), `reference/*.md` (the manual). Edit these, not any `.pylon/builtin/` copy.

**Ownership contract** (`syncBuiltinResources` in `doctor.go`, shared by init, doctor and `pylon mem inject`):
`.pylon/builtin/` (git-ignored) is pylon-owned. Every sync compares each file with the embeds, rewrites missing or
differing ones and deletes files the embeds no longer have — never a wholesale wipe, so concurrent runs converge.
Adding, changing or deleting a built-in therefore reaches a workspace on its next `pylon doctor`, with
`.claude/commands/` and `.claude/agents/` pruned accordingly. `.pylon/{agents,commands}/` hold user resources; readers
take the built-in first, so a same-named user file is ignored and never modified. Domain knowledge and memory are user
data and are never overwritten. Only the workspace-root `.pylon/` is synced — per-project `<project>/.pylon/`
directories are untouched.

`generateClaudeDir` (`launch.go`) and `pylon doctor` share `buildDesiredClaudeCommands`, so they cannot drift.

### No hooks

Pylon installs no hooks. `removePylonHooks` deletes the `pylon mem inject` SessionStart hook earlier releases wrote,
leaving other hooks alone. The hidden `pylon mem inject` exists only for workspaces that still have that hook: it runs
`refreshWorkspaceAfterUpgrade`, which removes the hook, and clears old hook junk from memory. Its stdout must stay
empty, because a hook's stdout becomes session context. Delete it once no workspace in use has the hook.

### Project discovery contract (`internal/config/workspace.go`)

A workspace subdirectory with `.pylon/` is a project unless it is a **workspace worktree**: a linked git worktree
whose main checkout is non-bare and is either the workspace root or a non-hidden direct subdirectory. Such a worktree is
just another checkout of something already here. Two things must never happen, and changes here are judged against them:

1. **A real project disappears.** Whatever cannot be shown to be a workspace worktree stays a project: main checkout
   outside the workspace, `core.worktree` pointing elsewhere, bare or separate git dir, submodule, a git error, or a main
   repository that moved. The one exception git cannot reveal: a separate git dir literally named `.git` inside the
   workspace is taken for its parent's checkout.
2. **A command destroys user files in a workspace worktree, or claims to have handled one it skipped.** Uninstall
   deletes only its `.pylon/` directory and keeps the worktree; `add-project` and `delete-project` refuse it; doctor
   neither regenerates anything in it nor suggests `add-project` for it.

The verdict must not depend on how the workspace path is spelled (symlink, case, Unicode normalization) or on the
caller's `GIT_*` environment and global git config. Non-goals: cleaning state left from when a worktree counted as a
project, repairing a worktree whose main checkout `uninstall --remove-projects` removed, and worktrees whose main
checkout is nested deeper. A review finding outside these rules is a known limit, not a bug.

### Package layout

- `cmd/pylon/main.go` — entry point; sets version, calls `cli.Execute()`.
- `internal/cli/` — every Cobra subcommand (`root.go` registers them) and the generation logic.
- `internal/layout/` — **all** `.pylon/` and `.claude/` path construction. Never hand-build these paths.
- `internal/config/` — workspace root and project discovery. `FindWorkspaceRoot` walks up to `.pylon/config.yml`, which
  only marks a workspace; its content is not read.
- `internal/memory/` — markdown-file memory store (`<category>/<slug>.md` + `INDEX.md`), token-match search.
- `internal/slug/`, `internal/fsutil/` — supporting utilities.

`resolveRoot()` (`workspace.go`) is the shared workspace-locator used by all commands.

## Conventions

- **Go 1.24+** (toolchain go1.26). Dependencies are minimal by design: Cobra and little else. Keep it that way.
- **User-facing strings are Korean**; code identifiers, comments-of-record, and errors wrapped for developers may be
  English. Match the surrounding file.
- Global CLI flags: `--workspace`, `--verbose/-v`, `--json`. Commands that output data should honor `--json`.
- **Versioning is CalVer** `YYYY.M.SEQ` (e.g. `2026.6.3`), *not* semver. Every push to `main` that passes CI gets the
  next tag and a goreleaser release (the `release` job in `.github/workflows/ci.yml`); there is no manual step. Because
  tags aren't `vX.Y.Z`, `go install @<ver>` does not resolve to a release.
