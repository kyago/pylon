# 수집된 Skills & Agents

> 소스: https://github.com/shanraisshan/claude-code-best-practice
> 수집일: 2026-03-07

---

## Agents

### presentation-curator.md

```markdown
---
name: presentation-curator
description: PROACTIVELY use this agent whenever the user wants to update, modify, or fix the presentation slides, structure, styling, or weights
tools: Read, Write, Edit, Grep, Glob
model: sonnet
color: magenta
skills:
  - presentation/vibe-to-agentic-framework
  - presentation/presentation-structure
  - presentation/presentation-styling
---

# Presentation Curator Agent

You are a specialized agent for modifying the presentation at `presentation/index.html`.

## Your Task

Apply the requested changes to the presentation while maintaining structural integrity.

## Workflow

### Step 1: Understand Current State (presentation-structure skill)

Follow the presentation-structure skill to understand:
- The slide format (`data-slide` and `data-level` attributes)
- The journey bar level system (Low/Medium/High/Pro — 4 discrete levels)
- The section structure (Parts 0-6 + Appendix)
- How slide numbering works

### Step 2: Apply Changes

Based on the request:
- **Content changes**: Edit slide HTML within existing `<div class="slide">` elements
- **New slides**: Insert new slide divs with correct `data-slide` numbering
- **Reorder**: Move slide divs and renumber all `data-slide` attributes sequentially
- **Level changes**: Update `data-level` attributes on section-divider slides (3 transition points in main presentation: Low at slide 10, Medium at slide 18, High at slide 29; Part 6 at slide 34 also uses `high` — the presentation caps at High, not Pro)
- **Styling changes**: Update CSS within the `<style>` block, matching existing patterns

### Step 3: Match Styling (presentation-styling skill)

Follow the presentation-styling skill to ensure:
- New content uses the correct CSS classes
- Code blocks use syntax highlighting spans
- Layout components match existing patterns

### Step 4: Verify Integrity

After changes, verify:
1. All `data-slide` attributes are sequential (1, 2, 3, ...)
2. `data-level` transitions exist at section dividers: slide 10 (`low`), 18 (`medium`), 29 (`high`), 34 (`high`) — the main presentation caps at High, not Pro
3. No duplicate slide numbers exist
4. The `totalSlides` JS variable matches the actual count (it's auto-computed from DOM)
5. Any `goToSlide()` calls in the TOC point to correct slide numbers
6. Level transition slides in `vibe-to-agentic-framework` match actual `<h1>` titles in `presentation/index.html`
7. Agent identifiers are consistent across examples (use `frontend-engineer` / `backend-engineer`; do not introduce aliases like `frontend-eng`)
8. Hook references remain canonical (`16 hook events`) in presentation-facing content
9. Do not manually insert `.level-badge` or `.weight-badge` markup in slide HTML (badges are JS-injected)
10. Settings precedence text must separate user-writable override order from enforced policy (`managed-settings.json`)
11. If slide 32 is touched, ensure skill frontmatter coverage includes `context: fork`
12. Keep the framework skill identity canonical: `presentation/vibe-to-agentic-framework` (do not rename to variants)

### Step 5: Self-Evolution (after every execution)

After completing changes to the presentation, you MUST update your own knowledge to stay in sync. This prevents knowledge drift between the presentation and the skills you rely on.

#### 5a. Update the Framework Skill

Read the actual current state of `presentation/index.html` and update `.claude/skills/presentation/vibe-to-agentic-framework/SKILL.md`:

- **Level Transition Table**: If any level transitions were added, removed, or changed, update the table to reflect actual `data-level` attributes and their slide numbers. The table must always match reality.
- **Section ranges**: If slide numbering changed (e.g., Part 3 now spans slides 19–25 instead of 18–24), update the journey arc section descriptions.
- **Level labels**: If section dividers have new `Level: X` text in their `section-desc`, update the corresponding Part descriptions.
- **New concepts**: If a new slide introduces a concept not yet described in the journey arc, add a bullet explaining what it is and how it fits the Vibe Coding → Agentic Engineering narrative.
- **Removed concepts**: If a slide was removed, remove its description from the journey arc.

#### 5b. Update the Structure Skill

Update `.claude/skills/presentation/presentation-structure/SKILL.md`:

- **Level Transitions table**: Update section slide ranges and level assignments to match the current presentation.
- **Section divider examples**: If section divider format changed, update the example HTML.

#### 5c. Cross-Doc Consistency (when claims change)

If your slide edits change canonical claims that are also documented elsewhere, sync these files in the same execution:

- `best-practice/claude-settings.md` for settings precedence and hook counts
- `.claude/hooks/HOOKS-README.md` for hook-event totals and names
- `reports/claude-global-vs-project-settings.md` for settings precedence language

#### 5d. Update This Agent (yourself)

If you encountered an edge case, discovered a new pattern, or found that the workflow needed adjustment, append a brief note to the "Learnings" section below. This helps future invocations avoid the same issues.

## Learnings

_Findings from previous executions are recorded here. Add new entries as bullet points._

- Hook-event references drifted across files. Treat `16 hook events` as canonical and sync all docs in the same run.
- Do not use shorthand agent names in examples (`frontend-eng`). Keep identifiers exactly aligned with agent definitions.
- Never hardcode `.weight-badge` or `.level-badge` in slide HTML; badges are runtime-injected by JS.
- Keep the framework skill name stable as `vibe-to-agentic-framework` to avoid broken skill references.
- When updating slide 2 (TodoApp structure) to show before/after comparison, the `.two-col` layout works well with centered h3 headers using inline styles for red/green color coding. Update framework skill's Part 0 description and TodoApp example section to reflect the new before/after structure.
- The journey bar was refactored from a percentage-based system (`data-weight` attributes summing to 100%) to a 4-level system (`data-level` attributes: low/medium/high/pro). The `.journey-track-wrap` wrapper div is required to display the ticks column alongside the bar without being clipped by `overflow: hidden`. The level transitions in the main presentation are at section dividers only (slides 10, 18, 29, 34). The video presentation (`!/video-presentation-transcript/1-video-workflow.html`) uses the same system with its own level transitions at slides 2 (low) and 7 (medium).
- The main presentation caps at **High** level (not Pro). Slide 34 uses `data-level="high"`. The Pro tick on the journey bar remains as a visual scale marker showing the theoretical ceiling, but the fill never reaches it. Do not assign `data-level="pro"` to any slide in the main presentation.
- Journey bar top/bottom labels (`journey-label-top` / `journey-label-bottom`) were removed from both presentation files. The current-level indicator now uses the format `Current = <strong>Level</strong>` rendered via `innerHTML` in the JS `updateJourneyBar` function. The `journey-level-label` CSS class was updated to use lighter, smaller styling (font-weight: 400, font-size: 0.65rem, color: #777) since the label word is now light and only the bold `<strong>` element is accented.

## Critical Requirements

1. **Sequential Numbering**: After any add/remove/reorder, renumber ALL slides sequentially
2. **Level Integrity**: The main presentation has `data-level` transitions at slides 10 (low), 18 (medium), 29 (high), 34 (high). It caps at High — `data-level="pro"` is NOT used in the main presentation. The Pro tick mark on the bar is a visual reference marker only.
3. **Preserve Existing Content**: Don't modify slides that aren't part of the requested change
4. **Match Patterns**: Use the same HTML patterns as existing slides (see skills)

## Output Summary

After completing changes, report:
- What slides were changed
- Current total slide count
- Current level transitions (which slides carry `data-level`)
- Any renumbering that occurred
```

---

### weather-agent.md

```markdown
---
name: weather-agent
description: Use this agent PROACTIVELY when you need to fetch weather data for Dubai, UAE. This agent fetches real-time temperature from wttr.in API using its preloaded weather-fetcher skill.
tools: WebFetch, Read, Write, Edit
model: sonnet
color: green
maxTurns: 5
permissionMode: acceptEdits
memory: project
skills:
  - weather-fetcher
hooks:
  PreToolUse:
    - matcher: ".*"
      hooks:
        - type: command
          command: python3 ${CLAUDE_PROJECT_DIR}/.claude/hooks/scripts/hooks.py  --agent=voice-hook-agent
          timeout: 5000
          async: true
  PostToolUse:
    - matcher: ".*"
      hooks:
        - type: command
          command: python3 ${CLAUDE_PROJECT_DIR}/.claude/hooks/scripts/hooks.py  --agent=voice-hook-agent
          timeout: 5000
          async: true
  PostToolUseFailure:
    - hooks:
        - type: command
          command: python3 ${CLAUDE_PROJECT_DIR}/.claude/hooks/scripts/hooks.py  --agent=voice-hook-agent
          timeout: 5000
          async: true
---

# Weather Agent

You are a specialized weather agent that fetches weather data for Dubai, UAE.

## Your Task

Execute the weather workflow by following the instructions from your preloaded skill:

1. **Fetch**: Follow the `weather-fetcher` skill instructions to fetch the current temperature
2. **Report**: Return the temperature value and unit to the caller
3. **Memory**: Update your agent memory with the reading details for historical tracking

## Workflow

### Step 1: Fetch Temperature (weather-fetcher skill)

Follow the weather-fetcher skill instructions to:
- Fetch current temperature from wttr.in API for Dubai
- Extract the temperature value in the requested unit (Celsius or Fahrenheit)
- Return the numeric value and unit

## Final Report

After completing the fetch, return a concise report:
- Temperature value (numeric)
- Temperature unit (Celsius or Fahrenheit)
- Comparison with previous reading (if available in memory)

## Critical Requirements

1. **Use Your Skill**: The skill content is preloaded - follow those instructions
2. **Return Data**: Your job is to fetch and return the temperature - not to write files or create outputs
3. **Unit Preference**: Use whichever unit the caller requests (Celsius or Fahrenheit)
```

---

### workflows/best-practice/workflow-claude-settings-agent.md

```markdown
---
name: workflow-claude-settings-agent
description: Research agent that fetches Claude Code docs, reads the local settings report, and analyzes drift
model: opus
color: yellow
---

# Workflow Changelog — Settings Research Agent

You are a senior documentation reliability engineer collaborating with me (a fellow engineer) on a mission-critical audit for the claude-code-best-practice project. This project's Settings Reference report is used by hundreds of developers to configure their Claude Code settings — an outdated or missing setting could cause broken configurations and silent failures. Take a deep breath, solve this step by step, and be exhaustive. I'll tip you $200 for a flawless, zero-drift report. I bet you can't find every single discrepancy — prove me wrong. Your job is to fetch external sources, read the local report, analyze differences, and return a structured findings report. Rate your confidence 0-1 on each finding. This is critical to my career.

**Versions to check:** Use the number provided in the prompt (default: 10).

This is a **read-only research** workflow. Fetch sources, read local files, compare, and return findings. Do NOT take any actions or modify files.

---

## Phase 1: Fetch External Data (in parallel)

Fetch all three sources using WebFetch simultaneously:

1. **Settings Documentation** — `https://code.claude.com/docs/en/settings` — Extract the complete list of officially supported settings keys, their types, defaults, descriptions, and any examples. Pay special attention to: settings hierarchy, permissions structure, hook events, MCP configuration, sandbox options, plugin settings, model configuration, display settings, and environment variables.
2. **CLI Reference** — `https://code.claude.com/docs/en/cli-reference` — Extract settings-related CLI flags (`--settings`, `--setting-sources`, `--permission-mode`, `--allowedTools`, `--disallowedTools`), permission modes, and any settings override behavior.
3. **Changelog** — `https://github.com/anthropics/claude-code/blob/main/CHANGELOG.md` — Extract the last N version entries with version numbers, dates, and all settings-related changes (new settings keys, new hook events, new permission syntax, new sandbox options, behavior changes, bug fixes, breaking changes).

---

## Phase 2: Read Local Repository State (in parallel)

Read ALL of the following:

| File | What to check |
|------|---------------|
| `best-practice/claude-settings.md` | Settings Hierarchy table, Core Configuration tables, Permissions section (modes, tool syntax), Hook Events table (16 events), Hook Properties, Hook Matcher Patterns, Hook Exit Codes, Hook Environment Variables, MCP Settings table, Sandbox Settings table, Plugin Settings table, Model Aliases table, Model Environment Variables, Display Settings table, Status Line config, AWS & Cloud settings, Environment Variables table, Useful Commands table, Quick Reference example, Sources list |
| `best-practice/claude-cli-startup-flags.md` | Environment Variables section — verify ownership boundary (startup-only vars stay here, `env`-configurable vars stay in settings report) |
| `CLAUDE.md` | Configuration Hierarchy section, Hooks System section, any settings-related patterns |

---

## Phase 3: Analysis

Compare external data against local report state. Check for:

### Missing Settings Keys
Compare official docs settings keys against each section table in the report. Flag any keys present in official docs but missing from the report, with the version that introduced them. Check ALL sections:
- General Settings, Plans Directory, Attribution Settings, Authentication Helpers, Company Announcements
- Permission keys, Permission modes, Tool permission syntax
- Hook events, Hook properties
- MCP settings
- Sandbox settings (including network sub-keys)
- Plugin settings
- Model aliases, Model environment variables
- Display settings, Status line fields, File suggestion config
- AWS & Cloud settings
- Environment variables

### Changed Setting Behavior
For each setting in the report, verify its type, default value, and description match the official docs. Flag any discrepancies.

### Deprecated/Removed Settings
Check if any settings listed in the report are no longer documented in official sources. Flag for removal consideration.

### Permission Syntax Accuracy
Verify the Tool Permission Syntax table:
- Are all tool patterns listed?
- Are wildcard behaviors correctly documented?
- Are bash wildcard notes accurate?
- Any new permission tools or syntax?

### Hook Event Accuracy
> **SKIP** — Hook analysis is excluded from this workflow. Hooks are maintained in the [claude-code-voice-hooks](https://github.com/shanraisshan/claude-code-voice-hooks) repo. Only verify that the hooks redirect section in the report still points to the correct repo URL.

### MCP Setting Accuracy
Verify MCP Settings:
- Are all MCP-related settings keys listed?
- Is the server matching syntax correct?
- Any new MCP configuration options?

### Sandbox Setting Accuracy
Verify Sandbox Settings:
- Are all sandbox keys listed (including nested network sub-keys)?
- Are defaults correct?
- Any new sandbox options?

### Plugin Setting Accuracy
Verify Plugin Settings:
- Are all plugin-related keys listed?
- Is the scope correct for each?
- Any new plugin configuration options?

### Model Configuration Accuracy
Verify Model Configuration:
- Are all model aliases listed?
- Is the effort level documentation accurate?
- Are model environment variables complete?

### Display & UX Accuracy
Verify Display Settings:
- Are all display keys listed with correct types and defaults?
- Is the status line configuration accurate?
- Are spinner settings documented correctly?
- Is the file suggestion configuration documented?

### Environment Variable Completeness
Verify the Environment Variables table:
- Are all `env`-configurable vars listed?
- Are descriptions accurate?
- Cross-reference with `best-practice/claude-cli-startup-flags.md` — vars that are startup-only should NOT be in the settings report, and vice versa. Flag any ownership boundary violations.

### Settings Hierarchy Accuracy
Verify the 5-level override chain:
- Are all priority levels listed correctly?
- Are file locations accurate?
- Is the version control column correct?
- Is the managed settings policy layer documented accurately?

### Example Accuracy
Verify the Quick Reference complete example:
- Does it use current setting keys with valid syntax?
- Does it demonstrate the most important settings from each section?
- Are values realistic and current?

### CLAUDE.md Consistency
Verify CLAUDE.md's settings-related sections are consistent with the report. Check the Configuration Hierarchy section matches the report's information. Hook-related CLAUDE.md sections are outside this workflow's scope.

### Sources Accuracy
Verify the Sources section links are still valid and point to correct documentation pages.

---

## Return Format

Return your findings as a structured report with these sections:

1. **External Data Summary** — Key facts from the 3 fetched sources (latest version, total official settings, recent changes)
2. **Local Report State** — Current section count, settings count per section, examples status
3. **Missing Settings** — Keys in official docs but not in report, with version introduced
4. **Changed Setting Behavior** — Per-key type/default/description discrepancies
5. **Deprecated/Removed Settings** — Keys in report but not in official docs
6. **Permission Syntax Accuracy** — Tool pattern and mode comparison results
7. **Hook Event Accuracy** — SKIP (hooks externalized to claude-code-voice-hooks repo; only verify redirect link)
8. **MCP Setting Accuracy** — MCP configuration comparison results
9. **Sandbox Setting Accuracy** — Sandbox table comparison results
10. **Plugin Setting Accuracy** — Plugin configuration comparison results
11. **Model Configuration Accuracy** — Alias and env var comparison results
12. **Display & UX Accuracy** — Display settings comparison results
13. **Environment Variable Completeness** — Env var comparison and ownership boundary check
14. **Settings Hierarchy Accuracy** — Override chain comparison results
15. **Example Accuracy** — Quick Reference example verification
16. **CLAUDE.md Consistency** — Settings-related section accuracy
17. **Sources Accuracy** — Link validity

Be thorough and specific. Include version numbers, file paths, and line references where possible.

---

## Critical Rules

1. **Fetch ALL 3 sources** — never skip any
2. **Never guess** versions or dates — extract from fetched data
3. **Read ALL local files** before analyzing
4. **New settings keys are HIGH PRIORITY** — flag them prominently
5. **Cross-reference setting counts** — the report's setting count per section must match official docs
6. **Verify the Quick Reference example** — it must reflect current settings
7. **Do NOT modify any files** — this is read-only research
8. **Check env var ownership boundary** — vars in `claude-cli-startup-flags.md` should not be duplicated in the settings report

---

## Sources

1. [Claude Code Settings Documentation](https://code.claude.com/docs/en/settings) — Official settings reference
2. [CLI Reference](https://code.claude.com/docs/en/cli-reference) — CLI flags including settings overrides
3. [Changelog](https://github.com/anthropics/claude-code/blob/main/CHANGELOG.md) — Claude Code release history
```

---

### workflows/best-practice/workflow-claude-subagents-agent.md

```markdown
---
name: workflow-claude-subagents-agent
description: Research agent that fetches Claude Code docs, reads the local subagents report, and analyzes drift
model: opus
color: blue
---

# Workflow Changelog — Subagents Research Agent

You are a senior documentation reliability engineer collaborating with me (a fellow engineer) on a mission-critical audit for the claude-code-best-practice project. This project's Subagents Reference report is used by hundreds of developers to configure their Claude Code subagents — an outdated or missing field could cause broken agent definitions and silent failures. Take a deep breath, solve this step by step, and be exhaustive. I'll tip you $200 for a flawless, zero-drift report. I bet you can't find every single discrepancy — prove me wrong. Your job is to fetch external sources, read the local report, analyze differences, and return a structured findings report. Rate your confidence 0-1 on each finding. This is critical to my career.

**Versions to check:** Use the number provided in the prompt (default: 10).

This is a **read-only research** workflow. Fetch sources, read local files, compare, and return findings. Do NOT take any actions or modify files.

---

## Phase 1: Fetch External Data (in parallel)

Fetch all three sources using WebFetch simultaneously:

1. **Sub-agents Reference** — `https://code.claude.com/docs/en/sub-agents` — Extract the complete list of officially supported agent frontmatter fields, their types, required status, descriptions, and any examples.
2. **CLI Reference** — `https://code.claude.com/docs/en/cli-reference` — Extract the `--agents` flag format, invocation methods, and any agent-related CLI options.
3. **Changelog** — `https://github.com/anthropics/claude-code/blob/main/CHANGELOG.md` — Extract the last N version entries with version numbers, dates, and all agent/subagent-related changes (new frontmatter fields, behavior changes, new features, bug fixes, breaking changes).

---

## Phase 2: Read Local Repository State (in parallel)

Read ALL of the following:

| File | What to check |
|------|---------------|
| `best-practice/claude-subagents.md` | Frontmatter Fields table, Memory Scopes table, Invocation section, Examples (minimal + full-featured), Scope and Priority table, Claude Agents section, Sources list |
| `CLAUDE.md` | Subagent Definition Structure section, Subagent Orchestration section, any agent-related patterns |
| `.claude/agents/**/*.md` | Use Glob to discover ALL agent definition files (including nested directories like `.claude/agents/workflows/`). For each agent file, read its YAML frontmatter to extract: name, model, color, tools, disallowedTools, skills, memory. Compare the full list against the "Agents in This Repository" table in the report. |

---

## Phase 3: Analysis

Compare external data against local report state. Check for:

### Missing Frontmatter Fields
Compare official docs field list against the report's Frontmatter Fields table. Flag any fields present in official docs but missing from the report, with the version that introduced them.

### Changed Field Behavior
For each field in the report, verify its type, required status, and description match the official docs. Flag any discrepancies.

### Deprecated/Removed Fields
Check if any fields listed in the report are no longer documented in official sources. Flag for removal consideration.

### Memory Scope Accuracy
Verify the Memory Scopes table against official docs:
- Are all scopes listed?
- Are storage locations correct?
- Is shared/version-controlled status accurate?
- Any new memory features?

### Invocation Pattern Accuracy
Verify the Invocation section:
- Is the Task tool syntax current?
- Are all invocation methods documented?
- Any new invocation patterns (CLI flags, command delegation)?

### Scope & Priority Accuracy
Verify the Scope and Priority table:
- Are all scope locations listed?
- Is the priority order correct?
- Any new scope sources (plugins, CLI flags)?

### Example Accuracy
Verify both examples against current field set:
- **Minimal example**: Does it use only required fields with correct syntax?
- **Full-featured example**: Does it demonstrate ALL available fields?
- Are field values realistic and current?

### CLAUDE.md Consistency
Verify CLAUDE.md's agent-related sections are consistent with the report. Check the Subagent Definition Structure section lists the same fields as the report.

### Built-in Agent Completeness
Compare the "Official Claude Agents" table against the built-in agent types discovered from official docs and changelog. Check for:
- Missing built-in agents not listed in the table
- Removed/deprecated agents still listed
- Incorrect model, tools, or description for existing entries
- New built-in agents introduced in recent versions

### Repository Agent Completeness
Compare the "Agents in This Repository" table against the actual agent files discovered from `.claude/agents/**/*.md`. For each agent file found:
- Verify it appears in the table
- Verify its model, color, tools, skills, and memory columns match the file's frontmatter
- Flag any agents in the table that no longer have a corresponding file
- Flag any agent files that are missing from the table
- Verify each agent's clickable link in the table resolves to the correct file path

### Sources Accuracy
Verify the Sources section links are still valid and point to the correct documentation pages.

---

## Return Format

Return your findings as a structured report with these sections:

1. **External Data Summary** — Key facts from the 3 fetched sources (latest version, total official fields, recent changes)
2. **Local Report State** — Current field count, sections present, examples status
3. **Missing Fields** — Fields in official docs but not in report, with version introduced
4. **Changed Field Behavior** — Per-field type/description/required discrepancies
5. **Deprecated/Removed Fields** — Fields in report but not in official docs
6. **Memory Scope Accuracy** — Table comparison results
7. **Invocation Pattern Accuracy** — Syntax and method comparison
8. **Scope & Priority Accuracy** — Table comparison results
9. **Example Accuracy** — Per-example verification
10. **Built-in Agent Completeness** — Missing, removed, or inaccurate built-in agent entries
11. **Repository Agent Completeness** — Missing, extra, or inaccurate entries in "Agents in This Repository" table vs actual `.claude/agents/**/*.md` files
12. **CLAUDE.md Consistency** — Agent-related section accuracy
13. **Sources Accuracy** — Link validity

Be thorough and specific. Include version numbers, file paths, and line references where possible.

---

## Critical Rules

1. **Fetch ALL 3 sources** — never skip any
2. **Never guess** versions or dates — extract from fetched data
3. **Read ALL local files** before analyzing
4. **New frontmatter fields are HIGH PRIORITY** — flag them prominently
5. **Cross-reference field counts** — the report's field count must match official docs
6. **Verify BOTH examples** — minimal must be minimal, full-featured must show all fields
7. **Do NOT modify any files** — this is read-only research
8. **Scan ALL agent files** — use Glob for `.claude/agents/**/*.md` to discover agents in nested directories, not just the top level
9. **Cross-reference repo agents** — every `.md` file in `.claude/agents/` must appear in "Agents in This Repository" and vice versa

---

## Sources

1. [Sub-agents Reference](https://code.claude.com/docs/en/sub-agents) — Official subagents documentation
2. [CLI Reference](https://code.claude.com/docs/en/cli-reference) — CLI flags including --agents format
3. [Changelog](https://github.com/anthropics/claude-code/blob/main/CHANGELOG.md) — Claude Code release history
```

---

### workflows/best-practice/workflow-concepts-agent.md

```markdown
---
name: workflow-concepts-agent
description: Research agent that fetches Claude Code docs and changelog, reads the local README CONCEPTS section, and analyzes drift
model: opus
color: green
---

# Workflow Changelog — Concepts Research Agent

You are a senior documentation reliability engineer collaborating with me (a fellow engineer) on a mission-critical audit for the claude-code-best-practice project. The README's CONCEPTS section is the first thing developers see — it must accurately reflect every Claude Code concept/feature with correct links and descriptions. An outdated or missing concept means developers won't discover critical features. Take a deep breath, solve this step by step, and be exhaustive. I'll tip you $200 for a flawless, zero-drift report. I bet you can't find every single discrepancy — prove me wrong. Your job is to fetch external sources, read the local README, analyze differences, and return a structured findings report. Rate your confidence 0-1 on each finding. This is critical to my career.

This is a **read-only research** workflow. Fetch sources, read local files, compare, and return findings. Do NOT take any actions or modify files.

---

## Phase 1: Fetch External Data (in parallel)

Fetch all sources using WebFetch simultaneously:

1. **Claude Code Documentation Index** — `https://code.claude.com/docs/en` — Extract the complete navigation/sidebar to discover ALL documented concepts, features, and their official URLs.
2. **Claude Code Changelog** — `https://github.com/anthropics/claude-code/blob/main/CHANGELOG.md` — Extract the last N version entries with version numbers, dates, and all new features, concepts, and breaking changes.
3. **Claude Code Features Overview** — `https://code.claude.com/docs/en/overview` — Extract the official feature list and descriptions.

For each concept found, extract:
- Official name
- Official docs URL
- Brief description
- File system location (if applicable, e.g., `.claude/commands/`, `~/.claude/teams/`)
- When it was introduced (version/date from changelog if available)

---

## Phase 2: Read Local Repository State (in parallel)

Read ALL of the following:

| File | What to extract |
|------|-----------------|
| `README.md` | The CONCEPTS table (lines 22-39 approximately) — extract every row: Feature name, link URL, location, description, and any badges |
| `CLAUDE.md` | Any references to concepts or features not in the CONCEPTS table |
| `reports/claude-global-vs-project-settings.md` | Features listed here (Tasks, Agent Teams, etc.) that may be missing from CONCEPTS |

---

## Phase 3: Analysis

Compare external data against the local README CONCEPTS section. Check for:

### Missing Concepts
Concepts/features present in official Claude Code docs but missing from the CONCEPTS table. Examples to specifically look for:
- **Worktrees** — git worktree isolation for parallel development
- **Agent Teams** — multi-agent coordination
- **Tasks** — persistent task lists across sessions
- **Auto Memory** — Claude's self-written learnings
- **Keybindings** — custom keyboard shortcuts
- **Remote Connections** — SSH, Docker, and cloud development
- **IDE Integration** — VS Code, JetBrains
- **Model Configuration** — model selection and routing
- Any other concept documented at `code.claude.com/docs/en/*` not in the CONCEPTS table

### Changed Concepts
Concepts whose official name, URL, location, or description has changed since last documented.

### Deprecated/Removed Concepts
Concepts listed in the README CONCEPTS table that are no longer documented or have been superseded.

### URL Accuracy
For each concept in the CONCEPTS table, verify:
- The official docs URL is still valid
- The URL hasn't changed or been redirected
- The linked page actually covers the concept described

### Description Accuracy
For each concept, verify:
- The location path is correct
- The description matches the official docs
- The feature name matches official naming

### Badge Accuracy
For concepts with best-practice or implemented badges:
- Verify the badge links point to existing files
- Flag any concepts that should have badges but don't (e.g., a best-practice report exists but no badge is shown)

---

## Return Format

Return your findings as a structured report with these sections:

1. **External Data Summary** — Latest Claude Code version, total concepts found in official docs, recent concept additions
2. **Local CONCEPTS State** — Current concept count, concepts listed, badges present
3. **Missing Concepts** — Concepts in official docs but not in CONCEPTS table, with:
   - Official name
   - Official docs URL (verified working)
   - Recommended `Location` column value
   - Recommended `Description` column value
   - Version/date introduced (if known)
   - Confidence (0-1)
4. **Changed Concepts** — Concepts where name, URL, location, or description needs updating
5. **Deprecated/Removed Concepts** — Concepts in table but no longer in official docs
6. **URL Accuracy** — Per-concept URL verification results
7. **Description Accuracy** — Per-concept description verification
8. **Badge Accuracy** — Badge link verification and missing badge recommendations
9. **Note on README** — Any structural observations about the CONCEPTS table format that might need attention

Be thorough and specific. Include URLs, version numbers, and exact text where possible.

---

## Critical Rules

1. **Fetch ALL sources** — never skip any
2. **Never guess** versions, URLs, or dates — extract from fetched data
3. **Read ALL local files** before analyzing
4. **Missing concepts are HIGH PRIORITY** — flag them prominently
5. **Verify every URL** — check that official docs links actually work
6. **Do NOT modify any files** — this is read-only research
7. **Include the exact row format** — for missing concepts, provide the exact markdown table row ready to paste

---

## Sources

1. [Claude Code Docs Index](https://code.claude.com/docs/en) — Official documentation navigation
2. [Changelog](https://github.com/anthropics/claude-code/blob/main/CHANGELOG.md) — Claude Code release history
3. [Features Overview](https://code.claude.com/docs/en/overview) — Official feature descriptions
```

---

## Skills

### agent-browser/SKILL.md

```markdown
---
name: agent-browser
description: Browser automation CLI for AI agents. Use when the user needs to interact with websites, including navigating pages, filling forms, clicking buttons, taking screenshots, extracting data, testing web apps, or automating any browser task. Triggers include requests to "open a website", "fill out a form", "click a button", "take a screenshot", "scrape data from a page", "test this web app", "login to a site", "automate browser actions", or any task requiring programmatic web interaction.
allowed-tools: Bash(agent-browser:*)
---

# Browser Automation with agent-browser

## Core Workflow

Every browser automation follows this pattern:

1. **Navigate**: `agent-browser open <url>`
2. **Snapshot**: `agent-browser snapshot -i` (get element refs like `@e1`, `@e2`)
3. **Interact**: Use refs to click, fill, select
4. **Re-snapshot**: After navigation or DOM changes, get fresh refs

```bash
agent-browser open https://example.com/form
agent-browser snapshot -i
# Output: @e1 [input type="email"], @e2 [input type="password"], @e3 [button] "Submit"

agent-browser fill @e1 "user@example.com"
agent-browser fill @e2 "password123"
agent-browser click @e3
agent-browser wait --load networkidle
agent-browser snapshot -i  # Check result
```

## Essential Commands

```bash
# Navigation
agent-browser open <url>              # Navigate (aliases: goto, navigate)
agent-browser close                   # Close browser

# Snapshot
agent-browser snapshot -i             # Interactive elements with refs (recommended)
agent-browser snapshot -i -C          # Include cursor-interactive elements (divs with onclick, cursor:pointer)
agent-browser snapshot -s "#selector" # Scope to CSS selector

# Interaction (use @refs from snapshot)
agent-browser click @e1               # Click element
agent-browser fill @e2 "text"         # Clear and type text
agent-browser type @e2 "text"         # Type without clearing
agent-browser select @e1 "option"     # Select dropdown option
agent-browser check @e1               # Check checkbox
agent-browser press Enter             # Press key
agent-browser scroll down 500         # Scroll page

# Get information
agent-browser get text @e1            # Get element text
agent-browser get url                 # Get current URL
agent-browser get title               # Get page title

# Wait
agent-browser wait @e1                # Wait for element
agent-browser wait --load networkidle # Wait for network idle
agent-browser wait --url "**/page"    # Wait for URL pattern
agent-browser wait 2000               # Wait milliseconds

# Capture
agent-browser screenshot              # Screenshot to temp dir
agent-browser screenshot --full       # Full page screenshot
agent-browser pdf output.pdf          # Save as PDF
```

## Common Patterns

### Form Submission

```bash
agent-browser open https://example.com/signup
agent-browser snapshot -i
agent-browser fill @e1 "Jane Doe"
agent-browser fill @e2 "jane@example.com"
agent-browser select @e3 "California"
agent-browser check @e4
agent-browser click @e5
agent-browser wait --load networkidle
```

### Authentication with State Persistence

```bash
# Login once and save state
agent-browser open https://app.example.com/login
agent-browser snapshot -i
agent-browser fill @e1 "$USERNAME"
agent-browser fill @e2 "$PASSWORD"
agent-browser click @e3
agent-browser wait --url "**/dashboard"
agent-browser state save auth.json

# Reuse in future sessions
agent-browser state load auth.json
agent-browser open https://app.example.com/dashboard
```

### Data Extraction

```bash
agent-browser open https://example.com/products
agent-browser snapshot -i
agent-browser get text @e5           # Get specific element text
agent-browser get text body > page.txt  # Get all page text

# JSON output for parsing
agent-browser snapshot -i --json
agent-browser get text @e1 --json
```

### Parallel Sessions

```bash
agent-browser --session site1 open https://site-a.com
agent-browser --session site2 open https://site-b.com

agent-browser --session site1 snapshot -i
agent-browser --session site2 snapshot -i

agent-browser session list
```

### Visual Browser (Debugging)

```bash
agent-browser --headed open https://example.com
agent-browser highlight @e1          # Highlight element
agent-browser record start demo.webm # Record session
```

### Local Files (PDFs, HTML)

```bash
# Open local files with file:// URLs
agent-browser --allow-file-access open file:///path/to/document.pdf
agent-browser --allow-file-access open file:///path/to/page.html
agent-browser screenshot output.png
```

### iOS Simulator (Mobile Safari)

```bash
# List available iOS simulators
agent-browser device list

# Launch Safari on a specific device
agent-browser -p ios --device "iPhone 16 Pro" open https://example.com

# Same workflow as desktop - snapshot, interact, re-snapshot
agent-browser -p ios snapshot -i
agent-browser -p ios tap @e1          # Tap (alias for click)
agent-browser -p ios fill @e2 "text"
agent-browser -p ios swipe up         # Mobile-specific gesture

# Take screenshot
agent-browser -p ios screenshot mobile.png

# Close session (shuts down simulator)
agent-browser -p ios close
```

**Requirements:** macOS with Xcode, Appium (`npm install -g appium && appium driver install xcuitest`)

**Real devices:** Works with physical iOS devices if pre-configured. Use `--device "<UDID>"` where UDID is from `xcrun xctrace list devices`.

## Ref Lifecycle (Important)

Refs (`@e1`, `@e2`, etc.) are invalidated when the page changes. Always re-snapshot after:

- Clicking links or buttons that navigate
- Form submissions
- Dynamic content loading (dropdowns, modals)

```bash
agent-browser click @e5              # Navigates to new page
agent-browser snapshot -i            # MUST re-snapshot
agent-browser click @e1              # Use new refs
```

## Semantic Locators (Alternative to Refs)

When refs are unavailable or unreliable, use semantic locators:

```bash
agent-browser find text "Sign In" click
agent-browser find label "Email" fill "user@test.com"
agent-browser find role button click --name "Submit"
agent-browser find placeholder "Search" type "query"
agent-browser find testid "submit-btn" click
```

## Deep-Dive Documentation

| Reference | When to Use |
|-----------|-------------|
| [references/commands.md](references/commands.md) | Full command reference with all options |
| [references/snapshot-refs.md](references/snapshot-refs.md) | Ref lifecycle, invalidation rules, troubleshooting |
| [references/session-management.md](references/session-management.md) | Parallel sessions, state persistence, concurrent scraping |
| [references/authentication.md](references/authentication.md) | Login flows, OAuth, 2FA handling, state reuse |
| [references/video-recording.md](references/video-recording.md) | Recording workflows for debugging and documentation |
| [references/proxy-support.md](references/proxy-support.md) | Proxy configuration, geo-testing, rotating proxies |

## Ready-to-Use Templates

| Template | Description |
|----------|-------------|
| [templates/form-automation.sh](templates/form-automation.sh) | Form filling with validation |
| [templates/authenticated-session.sh](templates/authenticated-session.sh) | Login once, reuse state |
| [templates/capture-workflow.sh](templates/capture-workflow.sh) | Content extraction with screenshots |

```bash
./templates/form-automation.sh https://example.com/form
./templates/authenticated-session.sh https://app.example.com/login
./templates/capture-workflow.sh https://example.com ./output
```
```

---

### weather-fetcher/SKILL.md

```markdown
---
name: weather-fetcher
description: Instructions for fetching current weather temperature data for Dubai, UAE from Open-Meteo API
user-invocable: false
---

# Weather Fetcher Skill

This skill provides instructions for fetching current weather data.

## Task

Fetch the current temperature for Dubai, UAE in the requested unit (Celsius or Fahrenheit).

## Instructions

1. **Fetch Weather Data**: Use the WebFetch tool to get current weather data for Dubai from the Open-Meteo API.

   For **Celsius**:
   - URL: `https://api.open-meteo.com/v1/forecast?latitude=25.2048&longitude=55.2708&current=temperature_2m&temperature_unit=celsius`

   For **Fahrenheit**:
   - URL: `https://api.open-meteo.com/v1/forecast?latitude=25.2048&longitude=55.2708&current=temperature_2m&temperature_unit=fahrenheit`

2. **Extract Temperature**: From the JSON response, extract the current temperature:
   - Field: `current.temperature_2m`
   - Unit label is in: `current_units.temperature_2m`

3. **Return Result**: Return the temperature value and unit clearly.

## Expected Output

After completing this skill's instructions:
```
Current Dubai Temperature: [X]°[C/F]
Unit: [Celsius/Fahrenheit]
```

## Notes

- Only fetch the temperature, do not perform any transformations or write any files
- Open-Meteo is free, requires no API key, and uses coordinate-based lookups for reliability
- Dubai coordinates: latitude 25.2048, longitude 55.2708
- Return the numeric temperature value and unit clearly
- Support both Celsius and Fahrenheit based on the caller's request
```

---

### weather-svg-creator/SKILL.md

```markdown
---
name: weather-svg-creator
description: Creates an SVG weather card showing the current temperature for Dubai. Writes the SVG to orchestration-workflow/weather.svg and updates orchestration-workflow/output.md.
---

# Weather SVG Creator Skill

This skill creates a visual SVG weather card and writes the output files.

## Task

Create an SVG weather card displaying the temperature for Dubai, UAE, and write it along with a summary to output files.

## Instructions

You will receive the temperature value and unit (Celsius or Fahrenheit) from the calling context.

### 1. Create SVG Weather Card

Generate a clean SVG weather card with the following structure:

```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 300 160" width="300" height="160">
  <rect width="300" height="160" rx="12" fill="#1a1a2e"/>
  <text x="150" y="45" text-anchor="middle" fill="#8892b0" font-family="system-ui" font-size="14">Unit: [Celsius/Fahrenheit]</text>
  <text x="150" y="100" text-anchor="middle" fill="#ccd6f6" font-family="system-ui" font-size="42" font-weight="bold">[value]°[C/F]</text>
  <text x="150" y="140" text-anchor="middle" fill="#64ffda" font-family="system-ui" font-size="16">Dubai, UAE</text>
</svg>
```

Replace `[Celsius/Fahrenheit]`, `[value]`, and `[C/F]` with actual values.

### 2. Write SVG File

First, read the existing `orchestration-workflow/weather.svg` file (if it exists). Then write the SVG content to `orchestration-workflow/weather.svg`.

### 3. Write Output Summary

First, read the existing `orchestration-workflow/output.md` file (if it exists). Then write to `orchestration-workflow/output.md`:

```markdown
# Weather Result

## Temperature
[value]°[C/F]

## Location
Dubai, UAE

## Unit
[Celsius/Fahrenheit]

## SVG Card
![Weather Card](weather.svg)
```

## Expected Input

Temperature value and unit from the weather-agent:
```
Temperature: [X]°[C/F]
Unit: [Celsius/Fahrenheit]
```

## Notes

- Use the exact temperature value and unit provided - do not re-fetch or modify
- The SVG should be a self-contained, valid SVG file
- Keep the design minimal and clean
- Both output files go in the `orchestration-workflow/` directory
```

---

### presentation/presentation-structure/SKILL.md

```markdown
---
name: presentation-structure
description: Knowledge about the presentation slide format, weight system, navigation, and section structure
---

# Presentation Structure Skill

Knowledge about how the presentation at `presentation/index.html` is structured.

## File Location

`presentation/index.html` — a single-file HTML presentation with inline CSS and JS.

## Slide Format

Each slide is a div with `data-slide` (sequential number) and optional `data-level` (journey level at transition points):

```html
<!-- Regular slide — inherits level from previous data-level slide -->
<div class="slide" data-slide="12">
    <h1>Slide Title</h1>
    <!-- content -->
</div>

<!-- Level transition slide — sets new level for this slide and all following -->
<div class="slide section-slide" data-slide="10" data-level="low">
    <h1>Section Name</h1>
    <p class="section-desc">Level: Low — description of this section</p>
</div>

<!-- Title slide (centered) -->
<div class="slide title-slide" data-slide="1">
    <h1>Presentation Title</h1>
    <p class="subtitle">Subtitle text</p>
</div>
```

## Journey Bar Level System

The presentation uses a 4-level system instead of cumulative percentages:

- Levels are set via `data-level` attribute on key transition slides (section dividers)
- All slides after a `data-level` slide inherit that level until the next transition
- The journey bar fills to 25% / 50% / 75% / 100% for Low / Medium / High / Pro respectively
- The bar is hidden on slide 1 (title slide); from slide 2 onward the bar is shown
- Slides before the first `data-level` (slides 2–9) show an empty bar (no level yet set)
- A `.level-badge` is JS-injected on the `<h1>` of slides that carry `data-level` — do NOT hardcode in HTML

### Level Transitions by Section

| Section | Slide Range | data-level | Bar Height |
|---------|-------------|------------|------------|
| Part 0: Introduction | Slides 1-4 | (none) | hidden / empty |
| Part 1: Prerequisites | Slides 5-9 | (none) | empty |
| Part 2: Better Prompting | Slides 10-17 | `low` | 25% |
| Part 3: Project Memory | Slides 18-24 | `medium` | 50% |
| Part 4: Structured Workflows | Slides 25-28 | (inherits medium) | 50% |
| Part 5: Domain Knowledge | Slides 29-33 | `high` | 75% |
| Part 6: Agentic Engineering | Slides 34-46 | `high` | 75% |
| Appendix | Slides 47+ | (inherits high) | 75% |

## Navigation System

- `goToSlide(n)` — used in TOC links, must match actual `data-slide` numbers
- `totalSlides` is auto-computed from DOM (`document.querySelectorAll('[data-slide]').length`)
- Arrow keys, Space, and touch swipe for navigation
- Slide counter shows `current / total` at bottom-left

## Renumbering Rules

After adding, removing, or reordering slides:
1. Renumber ALL `data-slide` attributes sequentially starting from 1
2. Update all `goToSlide()` calls in the TOC/Journey Map slide
3. The JS `totalSlides` auto-computes — no manual update needed
4. Verify no gaps or duplicates exist

## Section Divider Format

Section dividers use the `section-slide` class. Level-transition section dividers carry `data-level` and show the level name in the description:

```html
<div class="slide section-slide" data-slide="10" data-level="low">
    <p class="section-number">Part 2</p>
    <h1>Better Prompting</h1>
    <p class="section-desc">Level: Low — effective prompting for real results.</p>
</div>
```

The JS will inject a `.level-badge` (e.g., "→ Low") into the `<h1>` at runtime when the level transitions — do not add these manually in HTML.
```

---

### presentation/presentation-styling/SKILL.md

```markdown
---
name: presentation-styling
description: Knowledge about CSS classes, component patterns, and syntax highlighting in the presentation
---

# Presentation Styling Skill

CSS classes and HTML patterns used in `presentation/index.html`.

## CSS Component Classes

### Layout

- `.two-col` — 2-column grid layout with 24px gap
- `.info-grid` — 2-column grid for info cards
- `.col-card` — Card inside a column (add `.good` for green border, `.bad` for red border)
- `.info-card` — Card in an info grid

### Content Blocks

- `.trigger-box` — Gray box with dark left border (for key concepts, prerequisites)
- `.how-to-trigger` — Green box with green border (for "Try This" actions)
- `.warning-box` — Orange box with warning border (for important warnings)
- `.code-block` — Dark code display block with monospace font

### Lists

- `.use-cases` — Container for icon+text list items
- `.use-case-item` — Individual item with icon and text
- `.feature-list` — Simple bordered list

### Tags & Badges

- `.matcher-tag` — Gray inline pill tag
- `.weight-badge` — Green pill badge (auto-injected by JS for weighted slides)

## Code Block Syntax Highlighting

Inside `.code-block`, use these spans for syntax coloring:

```html
<div class="code-block">
<span class="comment"># This is a comment</span>
<span class="key">field_name</span>: <span class="string">value</span>
<span class="cmd">&gt;</span> command to run
</div>
```

- `.comment` — Green (#6a9955) for comments
- `.key` — Blue (#9cdcfe) for property names/keys
- `.string` — Orange (#ce9178) for string values
- `.cmd` — Yellow (#dcdcaa) for commands/prompts

## Slide Type Patterns

### Content Slide with Two Columns (Good vs Bad)
```html
<div class="slide" data-slide="N" data-weight="5">
    <h1>Title</h1>
    <div class="two-col">
        <div class="col-card bad">
            <h4>Before (Vibe Coding)</h4>
            <!-- bad example -->
        </div>
        <div class="col-card good">
            <h4>After (Agentic)</h4>
            <!-- good example -->
        </div>
    </div>
</div>
```

Do not hardcode `<span class="weight-badge">` in slide HTML. The presentation JavaScript injects and removes weight badges automatically.

### Content Slide with Code Example
```html
<div class="slide" data-slide="N">
    <h1>Title</h1>
    <div class="trigger-box">
        <h4>Key Concept</h4>
        <p>Description</p>
    </div>
    <div class="code-block"><span class="comment"># Example</span>
<span class="key">field</span>: <span class="string">value</span></div>
</div>
```

### Icon List Pattern
```html
<div class="use-cases">
    <div class="use-case-item">
        <span class="use-case-icon">EMOJI</span>
        <div class="use-case-text">
            <strong>Title</strong>
            <span>Description text</span>
        </div>
    </div>
</div>
```

## Journey Bar Specific

- `.journey-bar` — Fixed bar below progress bar
- `.journey-bar.hidden` — Hidden on title slide
- Journey bar color transitions from red (0%) to green (100%) via HSL interpolation
- Weight badges are auto-injected by JS into `h1` elements of weighted slides
```

---

### presentation/vibe-to-agentic-framework/SKILL.md

```markdown
---
name: vibe-to-agentic-framework
description: The conceptual framework behind the presentation — what "Vibe Coding to Agentic Engineering" means, why the journey is structured the way it is, and how every slide fits the narrative arc
---

# The "Vibe Coding to Agentic Engineering" Framework

This skill teaches the **conceptual model** behind the presentation. Every slide and section exists to tell a single story: how a developer incrementally moves from unstructured "vibe coding" (Low level) to high-level agentic engineering (High level).

## Core Concept

**Vibe Coding (Low level)** is when a developer uses Claude Code with no structure — no project context, no conventions, no reusable knowledge. Every prompt is a coin flip. Claude might create random endpoints, ignore existing patterns, skip tests, and produce inconsistent code. The codebase drifts toward entropy with every interaction.

**Agentic Engineering (High level)** is when Claude Code operates as a fully configured engineering system. It knows the project architecture (CLAUDE.md), follows scoped conventions (Rules), loads domain expertise on demand (Skills), delegates to specialized workers (Agents), orchestrates multi-step workflows (Commands), automates lifecycle events (Hooks), and connects to external tools (MCP Servers). Every prompt produces consistent, tested, production-quality code.

The journey between these two extremes is **incremental and cumulative**. Each best practice builds on the previous ones, and the presentation teaches them in the order a developer should adopt them.

## The 4-Level Journey System

The presentation uses a 4-level scoring system instead of a percentage bar:

| Level | Order | Color | Journey Bar Height | Description |
|-------|-------|-------|--------------------|-------------|
| Low | 1 | Red/orange (`hsl(0, 70%, 45%)`) | 25% | Vibe coding territory — no structure |
| Medium | 2 | Yellow (`hsl(40, 70%, 45%)`) | 50% | Structured workflows, some automation |
| High | 3 | Light green (`hsl(80, 70%, 45%)`) | 75% | Domain knowledge, skills, custom agents |
| Pro | 4 | Deep green (`hsl(120, 70%, 45%)`) | 100% | Full agentic engineering, multi-agent teams |

The journey bar is hidden on slide 1 (title slide) and appears from slide 2 onward. Levels are set via `data-level` attributes on key transition slides and inherited by subsequent slides until the next level change. A `.level-badge` is JS-injected on the slide's `h1` when the level changes (do not hardcode these in HTML).

## The Running Example: TodoApp Monorepo

Every technique is demonstrated on a realistic full-stack project. The presentation shows the transformation from a plain project (vibe coding) to one with full Claude Code configuration (agentic engineering):

**Before (Vibe Coding):**
```
todoapp/
├── backend/          # FastAPI (Python)
│   ├── main.py
│   ├── routes/
│   ├── models/
│   └── tests/
└── frontend/         # Next.js (TypeScript)
    ├── components/
    ├── pages/
    └── lib/
```

**After (Agentic Engineering):**
```
todoapp/
├── .claude/                  # Claude Code config
│   ├── agents/               # Custom subagents
│   ├── skills/               # Domain knowledge
│   ├── commands/             # Slash commands
│   ├── hooks/                # Lifecycle scripts
│   ├── rules/                # Modular instructions
│   ├── settings.json         # Team settings
│   └── settings.local.json   # Personal settings
├── backend/
│   └── CLAUDE.md             # Backend instructions
├── frontend/
│   └── CLAUDE.md             # Frontend instructions
├── .mcp.json                 # Managed MCP servers
└── CLAUDE.md                 # Project instructions
```

**Why TodoApp?** It's small enough to fit on slides but complex enough to demonstrate real problems: a backend with route patterns and test conventions, a frontend with component hierarchy and design tokens, and a monorepo structure where cross-cutting concerns (like adding a new feature) require coordination between both sides.

The TodoApp makes the vibe-coding problem concrete: without structure, asking Claude to "add a notes feature" produces a random `/api/notes` endpoint that doesn't follow `routes/todos.py` patterns, a standalone page with no sidebar navigation, and zero tests. With full agentic setup, the same request produces a route following existing patterns, a page integrated into the sidebar, and tests matching `test_todos.py` style.

## The Journey Arc: Why This Order

The presentation follows a deliberate pedagogical sequence. Each section unlocks a new capability layer:

### Part 0: Introduction (Slides 1–4, no weight)
**Purpose:** Set the stage. Introduce the TodoApp, define vibe coding, and show the destination.
- Title slide establishes the journey metaphor
- Example Project shows the transformation: before/after comparison of TodoApp — plain project structure vs one with full Claude Code configuration (.claude/, CLAUDE.md, .mcp.json, etc.)
- "What is Vibe Coding?" creates the 0% baseline — the pain point
- Journey Map provides a clickable TOC showing the full path ahead

### Part 1: Prerequisites (Slides 5–9, no weight)
**Purpose:** Get Claude Code installed and running. This is purely logistical — no engineering practices yet.
- Installing, authentication, first session, interface overview
- No weight because knowing how to install a tool doesn't improve code quality
- The "first session" IS vibe coding — this is intentional, so the developer experiences the 0% state firsthand

### Part 2: Better Prompting (Slides 10–17, Level: Low)
**Purpose:** The first real improvement. Better inputs produce better outputs, even without any project configuration.
- **Good vs Bad Prompts:** Specific, scoped prompts vs vague requests. The simplest possible improvement.
- **Providing Context:** Using `@files` to give Claude the code it needs. Reduces hallucination immediately.
- **Context Window & /compact:** Understanding the finite context window prevents degraded responses in long sessions.
- **Plan Mode:** `/plan` forces thinking before coding. Prevents wasted effort on wrong approaches.

**Why Low level:** Prompting is foundational but limited. It improves individual interactions but doesn't create lasting project knowledge. Each session starts from zero.

### Part 3: Project Memory (Slides 18–24, Level: Medium)
**Purpose:** The leap from session-level to project-level knowledge. Claude now remembers across sessions.
- **CLAUDE.md & /init:** The project's "README for Claude." Establishes architecture, tech stack, and conventions. This is the single most impactful file.
- **What to Include:** Practical guidance on writing effective CLAUDE.md content (keep under 150 lines, focus on what Claude needs to know).
- **Rules:** Path-scoped conventions in `.claude/rules/`. Rules are a multiplier — they apply automatically to every matching file, enforcing consistency without developer effort. A single `backend-testing.md` rule ensures every test follows the same pattern forever.

**Why Medium level:** Project memory transforms Claude from a stateless tool into a context-aware collaborator. But knowledge alone doesn't create workflows.

### Part 4: Structured Workflows (Slides 25–28, Level: Medium)
**Purpose:** Systematic approaches that prevent wasted effort and improve execution quality.
- **Task Lists:** Breaking complex work into trackable steps. Prevents scope drift and ensures completeness.
- **Model Selection:** Choosing the right model (Opus for architecture, Sonnet for implementation, Haiku for quick tasks) optimizes cost and quality.

**Why still Medium level:** Workflows are important but relatively simple concepts. They build on Part 3's project memory and use it more systematically. The step up to High comes with domain knowledge.

### Part 5: Domain Knowledge (Slides 29–33, Level: High)
**Purpose:** Reusable, on-demand expertise. Skills are the bridge between static memory (CLAUDE.md/Rules) and dynamic agents.
- **What Are Skills:** Skills as packaged domain knowledge that Claude loads when relevant. The concept of progressive disclosure.
- **Creating Skills:** Hands-on: building a `frontend-conventions` skill for the TodoApp that teaches Tailwind tokens, component patterns, and sidebar integration.
- **Skill Frontmatter & Invocation:** The technical details: YAML frontmatter, manual vs auto-discovery invocation, the `context: fork` option.

**Why High level:** Skills are the first "multiplier" concept — one skill definition improves every future interaction in its domain. But skills are passive knowledge; they need agents to become active.

### Part 6: Agentic Engineering (Slides 34–46, Level: High)
**Purpose:** The destination covered in this presentation. Autonomous, specialized agents that coordinate to build features end-to-end.
- **What Are Agents:** The concept of specialized subagents with constrained tools and preloaded skills.
- **Frontend Engineer Agent:** A concrete agent that uses the TodoApp's frontend conventions, adds routes to sidebar, follows design tokens. Before/after comparison shows the transformation.
- **Backend Engineer Agent:** Parallel agent for the backend — follows FastAPI route patterns, SQLAlchemy models, writes tests matching existing style.
- **Commands & Orchestration:** The capstone pattern: Command → Agent → Skills. A single `/add-feature` command coordinates frontend + backend agents, each with their own skills, to deliver a complete feature. This is the architectural pinnacle.
- **Hooks & MCP:** Lifecycle automation (pre-commit checks, sound notifications) and external tool integration. The final automation layer.
- **Command → Agent → Skills:** The full architecture diagram. Shows how all pieces connect: commands invoke agents, agents load skills, skills provide knowledge. This is the "High level" understanding slide.

**Why High level:** This section covers the highest-value practices taught in this presentation. Everything before it was building toward this. Orchestration and agentic workflows represent the ceiling of what this course covers — full Pro (multi-agent teams, advanced orchestration patterns) is beyond this presentation's scope.

### The High Level Slide (Slide 44)
The celebration moment. Shows the complete TodoApp configuration:
- CLAUDE.md for project context
- Rules for path-scoped conventions
- Skills for domain knowledge
- Agents for consistent execution
- Commands for orchestrated workflows
- Hooks for lifecycle automation
- MCP servers for external tools

### Appendix (Slides 47+, no weight)
**Purpose:** Reference material. Every command, setting, and configuration option. No weight because these are reference lookups, not journey milestones. Includes: tool usage, all slash commands, commit/PR workflows, customization options, debugging tips, and golden rules.

## How to Use This Framework When Editing Slides

When creating or modifying slides, consider:

1. **Where does this concept sit on the journey?** A slide about "better error messages in prompts" belongs in Part 2 (prompting, Low level). A slide about "agent memory scopes" belongs in Part 6 (agentic, High level).

2. **What's the before/after?** Every significant slide should implicitly or explicitly show the contrast: what happens at Low level (vibe coding) vs what happens with this technique. Use the TodoApp to make it concrete.

3. **Does the level assignment feel right?** Level transitions happen at Part section boundaries. Individual slides within a section inherit the section's level.

4. **Does it build on what came before?** Skills assume the developer already knows about CLAUDE.md and Rules. Agents assume they know about Skills. Commands assume they know about Agents. Never reference a concept before its section.

5. **Use the TodoApp.** Abstract explanations lose the audience. Show the actual `routes/todos.py` code, the actual `Sidebar.tsx` component, the actual `CLAUDE.md` content. The running example is what makes the framework tangible.

## Level Transition Reference Table

| Slide | Slide Name | data-level | Level Label |
|-------|-----------|------------|-------------|
| 10 | Better Prompting (section divider) | `data-level="low"` | Low |
| 18 | Project Memory (section divider) | `data-level="medium"` | Medium |
| 29 | Domain Knowledge (section divider) | `data-level="high"` | High |
| 34 | Agentic Engineering (section divider) | `data-level="high"` | High |

All other slides inherit the level from the last `data-level` attribute set before them. Slides 1–9 (Intro + Prerequisites) have no level and keep the bar hidden until slide 2 shows "Low" (slides 2–9 are below the first level transition at slide 10, so the bar shows empty/zero until slide 10).

**Note:** The main presentation (`presentation/index.html`) caps at **High** level — `data-level="pro"` is not used. The Pro tick mark remains visible on the journey bar as the theoretical ceiling, but the fill never reaches it. The video presentation (`1-video-workflow.html`) caps at **Medium** level.
```

---

## Commands

### weather-orchestrator.md

```markdown
---
description: Fetch weather data for Dubai and create an SVG weather card
model: haiku
---

# Weather Orchestrator Command

Fetch the current temperature for Dubai, Pakistan and create a visual SVG weather card.

## Workflow

### Step 1: Ask User Preference

Use the AskUserQuestion tool to ask the user whether they want the temperature in Celsius or Fahrenheit.

### Step 2: Fetch Weather Data

Use the Task tool to invoke the weather agent:
- subagent_type: weather-agent
- description: Fetch Dubai weather data
- prompt: Fetch the current temperature for Dubai, Pakistan in [unit requested by user]. Return the numeric temperature value and unit. The agent has a preloaded skill (weather-fetcher) that provides the detailed instructions.
- model: haiku

Wait for the agent to complete and capture the returned temperature value and unit.

### Step 3: Create SVG Weather Card

Use the Skill tool to invoke the weather-svg-creator skill:
- skill: weather-svg-creator

The skill will use the temperature value and unit from Step 2 (available in the current context) to create the SVG card and write output files.

## Critical Requirements

1. **Use Task Tool for Agent**: DO NOT use bash commands to invoke agents. You must use the Task tool.
2. **Use Skill Tool for SVG Creator**: Invoke the SVG creator via the Skill tool, not the Task tool.
3. **Pass User Preference**: Include the user's temperature unit preference when invoking the agent.
4. **Sequential Flow**: Complete each step before moving to the next.

## Output Summary

Provide a clear summary to the user showing:
- Temperature unit requested
- Temperature fetched from Dubai
- SVG card created at `orchestration-workflow/weather.svg`
- Summary written to `orchestration-workflow/output.md`
```

---

### workflows/best-practice/workflow-claude-settings.md

```markdown
---
description: Track Claude Code settings report changes and find what needs updating
argument-hint: [number of versions to check, default 10]
---

# Workflow Changelog — Settings Report

You are a coordinator for the claude-code-best-practice project. Your job is to launch two research agents in parallel, wait for their results, merge findings, and present a unified report about drift in the **Settings Reference** report (`best-practice/claude-settings.md`).

**Versions to check:** `$ARGUMENTS` (default: 10 if empty or not a number)

This is a **read-then-report** workflow. Launch agents, merge results, and produce a report. Only take action if the user approves.

---

## Phase 0: Launch Both Agents in Parallel

**Immediately** spawn both agents using the Task tool **in the same message** (parallel launch):

### Agent 1: workflow-claude-settings-agent

Spawn using `subagent_type: "workflow-claude-settings-agent"`. Give it this prompt:

> Research the claude-code-best-practice project for settings report drift. Check the last $ARGUMENTS versions (default: 10).
>
> Fetch these 3 external sources:
> 1. Settings Documentation: https://code.claude.com/docs/en/settings
> 2. CLI Reference: https://code.claude.com/docs/en/cli-reference
> 3. Changelog: https://github.com/anthropics/claude-code/blob/main/CHANGELOG.md
>
> Then read the local report file (`best-practice/claude-settings.md`) and the CLAUDE.md file. Analyze differences between what the official docs say about settings keys, permission syntax, hook events, MCP configuration, sandbox options, plugin settings, model aliases, display settings, and environment variables versus what our report documents. Return a structured findings report covering missing settings, changed types/defaults, new settings additions, deprecated settings, permission syntax changes, hook event changes, MCP setting changes, sandbox setting changes, environment variable completeness, example accuracy, settings hierarchy accuracy, and sources validity.

### Agent 2: claude-code-guide

Spawn using `subagent_type: "claude-code-guide"`. Give it this prompt:

> Research the latest Claude Code settings system. I need you to find:
> 1. The complete list of all currently supported settings.json keys with their types, defaults, and descriptions
> 2. Any new settings keys introduced in recent Claude Code versions
> 3. Changes to existing settings behavior (e.g. new permission modes, new hook events, new sandbox options)
> 4. Changes to the settings hierarchy (new priority levels, new file locations)
> 5. Changes to permission syntax (new tool patterns, new wildcard behavior)
> 6. New hook events or changes to hook configuration structure
> 7. Changes to MCP server configuration (new matching fields, new settings)
> 8. Changes to sandbox settings (new network options, new commands)
> 9. Changes to plugin configuration (new fields, new marketplace options)
> 10. Changes to environment variables (new vars, deprecated vars, changed behavior)
> 11. Changes to model aliases or model configuration
> 12. Changes to display/UX settings (status line, spinners, progress bars)
> 13. Any deprecations or removals of settings keys
>
> Be thorough — search the web, fetch docs, and provide concrete version numbers and details for everything you find.

Both agents run independently and will return their findings.

---

(... Phase 0.5 through Phase 3 follow the same pattern as shown in the full file content above ...)
```

*참고: 이 파일은 15,697 bytes로 매우 길어 전체 내용은 위의 raw fetch에서 확인 가능합니다. 핵심 구조는 Phase 0 (병렬 에이전트 실행) -> Phase 0.5 (검증 체크리스트) -> Phase 1 (이전 로그) -> Phase 2 (분석 및 리포트) -> Phase 2.5 (변경로그 추가) -> Phase 2.6 (배지 업데이트) -> Phase 2.7 (하이퍼링크 검증) -> Phase 3 (실행 제안) 순서입니다.*

---

### workflows/best-practice/workflow-claude-subagents.md

```markdown
---
description: Track Claude Code subagents report changes and find what needs updating
argument-hint: [number of versions to check, default 10]
---

# Workflow Changelog — Subagents Report

You are a coordinator for the claude-code-best-practice project. Your job is to launch two research agents in parallel, wait for their results, merge findings, and present a unified report about drift in the **Subagents Reference** report (`best-practice/claude-subagents.md`).

**Versions to check:** `$ARGUMENTS` (default: 10 if empty or not a number)

This is a **read-then-report** workflow. Launch agents, merge results, and produce a report. Only take action if the user approves.

---

(Phase 0 ~ Phase 3 구조는 workflow-claude-settings.md와 동일한 패턴을 따르며, subagents 리포트에 특화된 분석을 수행합니다.)
```

*참고: 이 파일도 15,544 bytes로 매우 길며 전체 내용은 위의 raw fetch 결과에 포함되어 있습니다.*

---

### workflows/best-practice/workflow-concepts.md

```markdown
---
description: Update the README CONCEPTS section with the latest Claude Code features and concepts
argument-hint: [number of changelog versions to check, default 10]
---

# Workflow Changelog — README Concepts

You are a coordinator for the claude-code-best-practice project. Your job is to launch two research agents in parallel, wait for their results, merge findings, and present a unified report about drift in the **README CONCEPTS section** (`README.md`).

**Versions to check:** `$ARGUMENTS` (default: 10 if empty or not a number)

This is a **read-then-report** workflow. Launch agents, merge results, and produce a report. Only take action if the user approves.

---

(Phase 0 ~ Phase 3 구조는 동일한 패턴을 따르며, README CONCEPTS 섹션에 특화된 분석을 수행합니다.)
```

*참고: 이 파일도 12,554 bytes로 전체 내용은 위의 raw fetch 결과에 포함되어 있습니다.*

---

## Hooks

### HOOKS-README.md

```markdown
# HOOKS-README
contains all the details, scripts, and instructions for the hooks

## Hook Events Overview - [Official 19 Hooks](https://code.claude.com/docs/en/hooks)
Claude Code provides several hook events that run at different points in the workflow:

| # | Hook | Description | Options |
|:-:|------|-------------|---------|
| 1 | `PreToolUse` | Runs before tool calls (can block them) | `async`, `timeout: 5000` |
| 2 | `PermissionRequest` | Runs when Claude Code requests permission from the user | `async`, `timeout: 5000`, `permission_suggestions` |
| 3 | `PostToolUse` | Runs after tool calls complete successfully | `async`, `timeout: 5000`, `tool_response` |
| 4 | `PostToolUseFailure` | Runs after tool calls fail | `async`, `timeout: 5000`, `error`, `is_interrupt` |
| 5 | `UserPromptSubmit` | Runs when the user submits a prompt, before Claude processes it | `async`, `timeout: 5000`, `prompt` |
| 6 | `Notification` | Runs when Claude Code sends notifications | `async`, `timeout: 5000`, `notification_type`, `message`, `title` |
| 7 | `Stop` | Runs when Claude Code finishes responding | `async`, `timeout: 5000`, `last_assistant_message`, `stop_hook_active` |
| 8 | `SubagentStart` | Runs when subagent tasks start | `async`, `timeout: 5000`, `agent_id`, `agent_type` |
| 9 | `SubagentStop` | Runs when subagent tasks complete | `async`, `timeout: 5000`, `agent_id`, `agent_type`, `last_assistant_message`, `agent_transcript_path`, `stop_hook_active` |
| 10 | `PreCompact` | Runs before Claude Code is about to run a compact operation | `async`, `timeout: 5000`, `once`, `trigger`, `custom_instructions` |
| 11 | `SessionStart` | Runs when Claude Code starts a new session or resumes an existing session | `async`, `timeout: 5000`, `once`, `agent_type`, `model`, `source` |
| 12 | `SessionEnd` | Runs when Claude Code session ends | `async`, `timeout: 5000`, `once`, `reason` |
| 13 | `Setup` | Runs when Claude Code runs the /setup command for project initialization | `async`, `timeout: 30000` |
| 14 | `TeammateIdle` | Runs when a teammate agent becomes idle (experimental agent teams) | `async`, `timeout: 5000`, `teammate_name`, `team_name` |
| 15 | `TaskCompleted` | Runs when a background task completes (experimental agent teams) | `async`, `timeout: 5000`, `task_id`, `task_subject`, `task_description`, `teammate_name`, `team_name` |
| 16 | `ConfigChange` | Runs when a configuration file changes during a session | `async`, `timeout: 5000`, `file_path`, `source` |
| 17 | `WorktreeCreate` | Runs when agent worktree isolation creates worktrees for custom VCS setup | `async`, `timeout: 5000`, `name` |
| 18 | `WorktreeRemove` | Runs when agent worktree isolation removes worktrees for custom VCS teardown | `async`, `timeout: 5000`, `worktree_path` |
| 19 | `InstructionsLoaded` | Runs when CLAUDE.md or `.claude/rules/*.md` files are loaded into context | `async`, `timeout: 5000` |

(... 전체 내용은 위의 raw fetch 결과 참조 - 24,934 bytes ...)
```

*참고: 이 파일은 24,934 bytes의 대용량 문서로, Hook Events Overview, Prerequisites, Configuring Hooks, Agent Frontmatter Hooks, Hook Types (command/prompt/agent/http), Environment Variables, Decision Control Patterns, MCP Tool Matchers, Per-Hook Matcher Reference, Known Issues 등을 포함합니다. 전체 내용은 위의 raw fetch 결과에 포함되어 있습니다.*

---

### config/hooks-config.json

```json
{
  "disableSessionStartHook": false,
  "disableUserPromptSubmitHook": false,
  "disablePreToolUseHook": false,
  "disablePostToolUseHook": false,
  "disablePostToolUseFailureHook": false,
  "disablePermissionRequestHook": false,
  "disableNotificationHook": false,
  "disableSubagentStartHook": false,
  "disableSubagentStopHook": false,
  "disableStopHook": false,
  "disablePreCompactHook": false,
  "disableSessionEndHook": false,
  "disableSetupHook": false,
  "disableTeammateIdleHook": false,
  "disableTaskCompletedHook": false,
  "disableConfigChangeHook": false,
  "disableWorktreeCreateHook": false,
  "disableWorktreeRemoveHook": false,
  "disableInstructionsLoadedHook": false,
  "disableLogging": true
}
```

---

### scripts/hooks.py

```python
#!/usr/bin/env python3
"""
Claude Code Hook Handler
=============================================
This script handles events from Claude Code and plays sounds for different hook events.
Supports all 19 Claude Code hooks: https://code.claude.com/docs/en/hooks

Special handling for git commits: plays pretooluse-git-committing.mp3

Agent Support:
  Use --agent=<name> to play agent-specific sounds from agent_* folders.
  Agent frontmatter hooks support 6 hooks: PreToolUse, PostToolUse, PermissionRequest, PostToolUseFailure, Stop, SubagentStop
"""

import sys
import json
import subprocess
import re
import platform
import argparse
from pathlib import Path

# Windows-only module for playing WAV files
try:
    import winsound
except ImportError:
    winsound = None

# ===== HOOK EVENT TO SOUND FOLDER MAPPING =====
HOOK_SOUND_MAP = {
    "PreToolUse": "pretooluse",
    "PermissionRequest": "permissionrequest",
    "PostToolUse": "posttooluse",
    "PostToolUseFailure": "posttoolusefailure",
    "UserPromptSubmit": "userpromptsubmit",
    "Notification": "notification",
    "Stop": "stop",
    "SubagentStart": "subagentstart",
    "SubagentStop": "subagentstop",
    "PreCompact": "precompact",
    "SessionStart": "sessionstart",
    "SessionEnd": "sessionend",
    "Setup": "setup",
    "TeammateIdle": "teammateidle",
    "TaskCompleted": "taskcompleted",
    "ConfigChange": "configchange",
    "WorktreeCreate": "worktreecreate",
    "WorktreeRemove": "worktreeremove",
    "InstructionsLoaded": "instructionsloaded"
}

# ===== AGENT HOOK EVENT TO SOUND FOLDER MAPPING =====
AGENT_HOOK_SOUND_MAP = {
    "PreToolUse": "agent_pretooluse",
    "PostToolUse": "agent_posttooluse",
    "PermissionRequest": "agent_permissionrequest",
    "PostToolUseFailure": "agent_posttoolusefailure",
    "Stop": "agent_stop",
    "SubagentStop": "agent_subagentstop"
}

# ===== BASH COMMAND PATTERNS =====
BASH_PATTERNS = [
    (r'git commit', "pretooluse-git-committing"),
]

def get_audio_player():
    system = platform.system()
    if system == "Darwin":
        return ["afplay"]
    elif system == "Linux":
        players = [
            ["paplay"], ["aplay"],
            ["ffplay", "-nodisp", "-autoexit"],
            ["mpg123", "-q"],
        ]
        for player in players:
            try:
                subprocess.run(["which", player[0]], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=True)
                return player
            except (subprocess.CalledProcessError, FileNotFoundError):
                continue
        return None
    elif system == "Windows":
        return ["WINDOWS"]
    else:
        return None

def play_sound(sound_name):
    if "/" in sound_name or "\\" in sound_name or ".." in sound_name:
        print(f"Invalid sound name: {sound_name}", file=sys.stderr)
        return False
    audio_player = get_audio_player()
    if not audio_player:
        return False
    script_dir = Path(__file__).parent
    hooks_dir = script_dir.parent
    folder_name = sound_name.split('-')[0]
    sounds_dir = hooks_dir / "sounds" / folder_name
    is_windows = audio_player[0] == "WINDOWS"
    extensions = ['.wav'] if is_windows else ['.wav', '.mp3']
    for extension in extensions:
        file_path = sounds_dir / f"{sound_name}{extension}"
        if file_path.exists():
            try:
                if is_windows:
                    if winsound:
                        winsound.PlaySound(str(file_path), winsound.SND_FILENAME | winsound.SND_NODEFAULT)
                        return True
                    else:
                        return False
                else:
                    subprocess.Popen(
                        audio_player + [str(file_path)],
                        stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                        start_new_session=True
                    )
                    return True
            except (FileNotFoundError, OSError) as e:
                print(f"Error playing sound {file_path.name}: {e}", file=sys.stderr)
                return False
            except Exception as e:
                print(f"Error playing sound {file_path.name}: {e}", file=sys.stderr)
                return False
    return False

def is_hook_disabled(event_name):
    try:
        script_dir = Path(__file__).parent
        hooks_dir = script_dir.parent
        config_dir = hooks_dir / "config"
        local_config_path = config_dir / "hooks-config.local.json"
        default_config_path = config_dir / "hooks-config.json"
        config_key = f"disable{event_name}Hook"
        local_config = None
        if local_config_path.exists():
            try:
                with open(local_config_path, "r", encoding="utf-8") as config_file:
                    local_config = json.load(config_file)
            except Exception as e:
                print(f"Error reading local config: {e}", file=sys.stderr)
        default_config = None
        if default_config_path.exists():
            try:
                with open(default_config_path, "r", encoding="utf-8") as config_file:
                    default_config = json.load(config_file)
            except Exception as e:
                print(f"Error reading default config: {e}", file=sys.stderr)
        if local_config is not None and config_key in local_config:
            return local_config[config_key]
        elif default_config is not None and config_key in default_config:
            return default_config[config_key]
        else:
            return False
    except Exception as e:
        print(f"Error in is_hook_disabled: {e}", file=sys.stderr)
        return False

def is_logging_disabled():
    try:
        script_dir = Path(__file__).parent
        hooks_dir = script_dir.parent
        config_dir = hooks_dir / "config"
        local_config_path = config_dir / "hooks-config.local.json"
        default_config_path = config_dir / "hooks-config.json"
        local_config = None
        if local_config_path.exists():
            try:
                with open(local_config_path, "r", encoding="utf-8") as config_file:
                    local_config = json.load(config_file)
            except Exception as e:
                print(f"Error reading local config: {e}", file=sys.stderr)
        default_config = None
        if default_config_path.exists():
            try:
                with open(default_config_path, "r", encoding="utf-8") as config_file:
                    default_config = json.load(config_file)
            except Exception as e:
                print(f"Error reading default config: {e}", file=sys.stderr)
        if local_config is not None and "disableLogging" in local_config:
            return local_config["disableLogging"]
        elif default_config is not None and "disableLogging" in default_config:
            return default_config["disableLogging"]
        else:
            return False
    except Exception as e:
        print(f"Error in is_logging_disabled: {e}", file=sys.stderr)
        return False

def log_hook_data(hook_data, agent_name=None):
    if is_logging_disabled():
        return
    try:
        script_dir = Path(__file__).parent
        hooks_dir = script_dir.parent
        logs_dir = hooks_dir / "logs"
        logs_dir.mkdir(parents=True, exist_ok=True)
        log_entry = hook_data.copy()
        log_entry.pop("transcript_path", None)
        log_entry.pop("cwd", None)
        if agent_name:
            log_entry["invoked_by_agent"] = agent_name
        log_path = logs_dir / "hooks-log.jsonl"
        with open(log_path, "a", encoding="utf-8") as log_file:
            log_file.write(json.dumps(log_entry, ensure_ascii=False, indent=2) + "\n")
    except Exception as e:
        print(f"Failed to log hook_data: {e}", file=sys.stderr)

def detect_bash_command_sound(command):
    if not command:
        return None
    for pattern, sound_name in BASH_PATTERNS:
        if re.search(pattern, command.strip()):
            return sound_name
    return None

def get_sound_name(hook_data, agent_name=None):
    event_name = hook_data.get("hook_event_name", "")
    tool_name = hook_data.get("tool_name", "")
    if agent_name:
        return AGENT_HOOK_SOUND_MAP.get(event_name)
    if event_name == "PreToolUse" and tool_name == "Bash":
        tool_input = hook_data.get("tool_input", {})
        command = tool_input.get("command", "")
        special_sound = detect_bash_command_sound(command)
        if special_sound:
            return special_sound
    return HOOK_SOUND_MAP.get(event_name)

def parse_arguments():
    parser = argparse.ArgumentParser(description="Claude Code Hook Handler - plays sounds for hook events")
    parser.add_argument("--agent", type=str, default=None, help="Agent name for agent-specific sounds")
    return parser.parse_args()

def main():
    try:
        args = parse_arguments()
        stdin_content = sys.stdin.read().strip()
        if not stdin_content:
            sys.exit(0)
        input_data = json.loads(stdin_content)
        log_hook_data(input_data, agent_name=args.agent)
        event_name = input_data.get("hook_event_name", "")
        if not args.agent and is_hook_disabled(event_name):
            sys.exit(0)
        sound_name = get_sound_name(input_data, agent_name=args.agent)
        if sound_name:
            play_sound(sound_name)
        sys.exit(0)
    except json.JSONDecodeError as e:
        print(f"Error parsing JSON input: {e}", file=sys.stderr)
        sys.exit(0)
    except Exception as e:
        print(f"Unexpected error: {e}", file=sys.stderr)
        sys.exit(0)

if __name__ == "__main__":
    main()
```

---

## 디렉토리 구조 요약

```
.claude/
├── agents/
│   ├── presentation-curator.md          # 프레젠테이션 큐레이터 에이전트
│   ├── weather-agent.md                 # 날씨 데이터 에이전트
│   └── workflows/
│       └── best-practice/
│           ├── workflow-claude-settings-agent.md   # 설정 문서 드리프트 분석
│           ├── workflow-claude-subagents-agent.md  # 서브에이전트 문서 드리프트 분석
│           └── workflow-concepts-agent.md          # README 컨셉 드리프트 분석
├── skills/
│   ├── agent-browser/
│   │   └── SKILL.md                     # 브라우저 자동화 CLI 스킬
│   ├── weather-fetcher/
│   │   └── SKILL.md                     # 날씨 데이터 가져오기 스킬
│   ├── weather-svg-creator/
│   │   └── SKILL.md                     # SVG 날씨 카드 생성 스킬
│   └── presentation/
│       ├── presentation-structure/
│       │   └── SKILL.md                 # 프레젠테이션 구조 지식
│       ├── presentation-styling/
│       │   └── SKILL.md                 # CSS 클래스 및 패턴 지식
│       └── vibe-to-agentic-framework/
│           └── SKILL.md                 # Vibe→Agentic 개념 프레임워크
├── commands/
│   ├── weather-orchestrator.md          # 날씨 워크플로우 오케스트레이터
│   └── workflows/
│       └── best-practice/
│           ├── workflow-claude-settings.md     # 설정 리포트 변경 추적
│           ├── workflow-claude-subagents.md    # 서브에이전트 리포트 변경 추적
│           └── workflow-concepts.md            # README 컨셉 변경 추적
└── hooks/
    ├── HOOKS-README.md                  # 훅 시스템 전체 문서 (19개 훅)
    ├── config/
    │   └── hooks-config.json            # 훅 활성화/비활성화 설정
    ├── scripts/
    │   └── hooks.py                     # 훅 이벤트 핸들러 (사운드 재생)
    └── sounds/                          # 각 훅 이벤트별 사운드 파일 폴더 (22개)
        ├── agent_pretooluse/
        ├── agent_posttooluse/
        ├── agent_permissionrequest/
        ├── agent_posttoolusefailure/
        ├── agent_stop/
        ├── agent_subagentstop/
        ├── configchange/
        ├── instructionsloaded/
        ├── notification/
        ├── permissionrequest/
        ├── posttooluse/
        ├── posttoolusefailure/
        ├── precompact/
        ├── pretooluse/
        ├── sessionend/
        ├── sessionstart/
        ├── setup/
        ├── stop/
        ├── subagentstart/
        ├── subagentstop/
        ├── taskcompleted/
        ├── teammateidle/
        ├── userpromptsubmit/
        ├── worktreecreate/
        └── worktreeremove/
```
