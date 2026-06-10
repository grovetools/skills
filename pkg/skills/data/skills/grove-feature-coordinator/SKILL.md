---
name: grove-feature-coordinator
description: Coordinator SOP for driving features from inbox notes through planning chats to implementation agents across a Grove ecosystem. Use when orchestrating multi-job flow plans.
domain: grove-feature
---

> **ROLE**: Coordinator
> **REQUIRED SUB-SKILLS**: None
> **DELEGATION RULE**: Orchestrate planning and agent dispatch; delegate implementation to agents. Implement directly only for small (1-3 file) changes.

# Feature Coordinator

This is the workflow for a coordinator session that takes inbox items through to implementation across a Grove ecosystem.

## Core Workflow

### 1. Read inbox items and triage

Read notes from the workspace inbox to understand what needs doing:

```bash
ls <notebook>/workspaces/<workspace>/inbox/
cat <notebook>/workspaces/<workspace>/inbox/<note>.md
```

### 2. Promote inbox items to chat jobs (preferred)

Use `nb promote` to turn inbox items into plan jobs:

```bash
nb promote /path/to/inbox/note.md --plan /path/to/plan-dir
```

This creates a numbered chat job, moves the note to `in_progress/`, and links them via frontmatter. After promoting, inline the note content into the job file and write the chat prompt.

Alternatively, create chat jobs manually:

```bash
flow plan add <plan-dir> --title "feature-name" --type chat --model <chat-model> --template chat
```

This creates a numbered job file (e.g. `03-feature-name.md`).

### 3. Write the spec/prompt in the job file

Edit the job file to add the problem description, current state analysis, and design questions. The template provides the system prompt; you append the feature-specific content below `<!-- grove: {"template": "chat"} -->`.

### 4. Curate context with cx

Each job gets an auto-created rules file at `rules/<job>.rules`. Edit it directly:

```
# Write rules like:
#   @a:<worktree>:<repo>/internal/app/app.go
#   @a:<worktree>:<repo>/pkg/embed/
#   !**/*_test.go

# Verify context for the job
cx stats --job <plan-dir>/<job>.md
```

Context rules use `@a:<worktree>:<path>` for cross-repo includes, `@grep:` for content filtering, and `!` for exclusions.

### 5. Run the chat job

```bash
flow plan run <plan-dir>/03-feature-name.md
```

This sends the spec + curated context to the chat model. The response is appended to the job file. Review the response and iterate by appending user feedback and re-running if needed.

### 6. Create implementation jobs

Once the chat plan is satisfactory, create an interactive_agent impl job with the chat as a dependency:

```bash
flow plan add <plan-dir> --title "impl-feature-name" --type interactive_agent --model <agent-model> -d 03-feature-name.md -p "Implement the plan from the dependency job."
```

The `-d` flag sets the chat job as a dependency, so the agent gets the full chat transcript as context. No need to curate separate context for impl agents — they read the codebase directly.

### 7. Run implementation agents

```bash
flow plan run <plan-dir>/04-impl-feature-name.md
```

Agents run in tmux sessions. Multiple agents can run in parallel **only if they touch separate repos**. Same-repo agents would conflict on git state.

### 8. Review and commit

After an agent completes, review its changes. The agent typically commits within the submodule. Then bump the submodule in the parent:

```bash
cd <worktree>
git add <submodule>
git commit -m "chore: bump <submodule> for <feature>"
```

### 9. Build verification

```bash
grove build --json   # Full ecosystem build; all submodules must pass
```

## Key Commands Reference

| Command | Purpose |
|---------|---------|
| `flow plan add <dir> --title X --type chat --model M --template chat` | Create a chat job |
| `flow plan add <dir> --title X --type interactive_agent --model M -d dep.md -p "..."` | Create an impl job |
| `flow plan context set <job-file>` | Save current cx rules to a job |
| `flow plan run <job-file>` | Run a job |
| `flow plan status <dir> --json` | Check job statuses (JSON) |
| `flow complete <slug> <job-file>` | Mark a job as completed |
| `flow agent list` | List all running agents |
| `flow agent read <slug> <job>` | Read agent's terminal output |
| `flow agent status <slug> <job>` | Check if agent is idle/busy |
| `flow agent send <slug> <job> "message"` | Send input to an agent |
| `cx generate` | Regenerate context from rules |
| `cx stats` | Check context file/token counts |
| `cx stats --job <job-file>` | Check a job's saved context |
| `grove build --json` | Full ecosystem build |
| `notify <channel> "message"` | Send a status update notification (channels: system, ntfy, signal, ha) |
| `nb list` | List inbox items in current workspace |
| `nb list --json` | List with full paths (JSON) |
| `nb new "title" -t inbox --no-edit` | Create inbox item (pipe body via stdin) |
| `nb promote <note-path> --plan <plan-dir>` | Promote a note to a job in an existing plan |
| `flow plan demote <job-file>` | Demote a job back to an nb inbox note |

## Creating Notes / Filing Tickets

Notes are workspace-scoped. `nb new` creates a note in the **current workspace** (resolved from cwd). To file a note in a specific workspace, `cd` into that workspace's repo first.

```bash
# Create a note in the current workspace (interactive editor)
nb new "bug title"

# Create with content piped from stdin (no editor)
echo "description here" | nb new "bug title" -t inbox --no-edit

# File in a specific workspace by cd'ing first
cd /path/to/<repo> && echo "content" | nb new "title" -t inbox --no-edit

# Note types (directories under the workspace notes folder):
#   inbox (default), completed, issues, concepts
nb new "title" -t issues --no-edit
```

Notes land at `<notebook>/workspaces/<workspace>/inbox/<timestamp>-<title>.md` with YAML frontmatter (id, title, tags, repository, branch, created, modified).

## Note ↔ Plan Job Lifecycle

Notes move through directories that track their state: `inbox/` → `in_progress/` → `completed/`. Jobs and notes are linked bidirectionally via frontmatter.

**Full lifecycle:**

```
inbox/bug.md         → nb promote --plan X   → in_progress/bug.md + plan job (pending)
in_progress/bug.md   → flow complete job.md  → completed/bug.md   + job (completed)
in_progress/bug.md   → flow plan demote job  → inbox/bug.md       + job (abandoned)
```

**Promote note → job in existing plan:**

```bash
# Default: creates a chat job, moves note to in_progress/, links via frontmatter
nb promote /path/to/note.md --plan /path/to/plan-dir

# Specify job type and template
nb promote note.md --plan /path/to/plan --type headless_agent --template chat

# Cross-workspace (note in one workspace, plan in another)
nb promote <notebook>/workspaces/<ws-a>/inbox/bug.md \
  --plan <notebook>/workspaces/<ws-b>/plans/<plan-dir>

# Resolve --plan relative to a workspace's plans/ directory
nb promote note.md --plan <plan-name> --workspace /path/to/ecosystem/worktree
```

**Promote note → new plan:**

```bash
flow plan init my-feature --from-note /path/to/note.md --recipe standard-feature
```

**Complete a job (moves linked note to completed/):**

```bash
flow complete <slug> <job-file>
# If job has note_ref, automatically moves the note from in_progress/ to completed/
```

**Demote job → nb inbox note:**

```bash
# Moves note from in_progress/ back to inbox/, marks job as abandoned
flow plan demote /path/to/plan-dir/03-stale-job.md

# Override target workspace
flow plan demote job.md --workspace /path/to/workspace
```

**TUI shortcuts** (for interactive use):
- `P` in nb TUI — promote note to new plan (launches flow plan init wizard)
- `J` in nb TUI — promote note to job in existing plan (plan picker overlay)
- `D` in flow plan status — demote job to nb inbox note

**When to use each:**
- **Promote to existing plan** (`nb promote --plan`): Single bug/feature that belongs in an active plan. Quick triage.
- **Promote to new plan** (`flow plan init --from-note`): Large feature needing its own plan with multiple phases.
- **Demote** (`flow plan demote`): Park stale jobs as inbox notes without deleting plan history.

**Note directories:**
- `inbox/` — new, untriaged notes
- `in_progress/` — promoted to a plan job, actively being worked
- `completed/` — job finished, note moved here automatically by `flow complete`
- `.archive/` — legacy location (older promotes used this instead of `in_progress/`)

**Frontmatter linking** (automatic):

```yaml
# In the job file (after promote):
note_ref: /absolute/path/to/workspace/in_progress/note.md

# In the in_progress/completed note:
plan_ref: plan-name/03-job-title.md
```

## Parallel Workflow Pattern

When triaging multiple inbox items in one session:

1. Read all inbox items, pick 2-3 related ones
2. Create chat jobs for all of them
3. Write specs and curate context for each
4. Run all chats (can be parallel — they're read-only)
5. Review responses, iterate if needed
6. Create impl jobs with chat dependencies
7. Run impl agents (parallel only if separate repos)
8. Review, commit, build-verify

## Multi-Phase Pipeline Pattern

For large architectural features that are too risky for a single agent session, break into sequential phases where each phase produces a working build.

**1. Design chat (multi-turn):**
Create a chat job and iterate on the spec with the chat model. Write the problem statement, let the model respond, then append follow-up turns to refine the architecture. Each user turn must be preceded by `<!-- grove: {"template": "chat"} -->`. Curate context with `cx` and save it to the job. The coordinator should verify the model's claims (see "Verify chat plans" below) and push back in follow-up turns before completing the chat.

**2. Create chat planning jobs per phase (not oneshot):**
For each implementation phase, create a **chat** job (not oneshot). Use chat instead of oneshot so you can append corrections and iterate if the first plan isn't right.

**Understanding `depends_on` vs `inline`:**
- `depends_on` controls **execution ordering** — a job won't run until its dependencies are completed.
- `inline: dependencies` controls **how dependency content reaches the LLM** — it embeds the full text of dependency job files directly into the prompt.

**Chat/oneshot jobs** need `--inline=dependencies` because the chat model has no file access — the only way it sees the design spec and prior plans is if their content is inlined into the prompt. Without `inline`, the model would get a prompt saying "read 03-design-chat.md" but have no way to actually read it.

**Headless agent jobs** should NOT use `--inline=dependencies`. Agents have file access tools — they read the dependency files themselves from the filesystem. Inlining would duplicate large transcripts into the prompt, wasting context window. Instead, the agent's prompt should say "Read the spec in 03-design-chat.md first" and let it use its file tools.

Each planning chat gets its own `cx` rules file with the files relevant to that phase. This is important — different phases touch different parts of the codebase, so context should be scoped per phase.

```bash
# Planning chat: inline=dependencies so the chat model sees the spec text
flow plan add <dir> --title "phase1-plan" --type chat --model <chat-model> \
  --template chat --inline=dependencies -d 03-design-chat.md \
  -p "Write a detailed impl plan for Phase 1: ..."
```

Then write cx rules for the job:

```
# rules/04-phase1-plan.md.rules
@a:<worktree>:<repo>/src/components/SearchBar.tsx
@a:<worktree>:<repo>/internal/server/handlers.go
@a:<worktree>:<other-repo>/pkg/relevant_file.go
```

Generate and verify context:

```bash
cx generate --job <plan-dir>/04-phase1-plan.md
cx stats --job <plan-dir>/04-phase1-plan.md
```

**3. Create headless impl jobs per phase:**
For each phase, create a headless agent that depends on the original spec + its planning chat + the previous phase's impl (so it sees the latest code state). No `--inline` — the agent reads deps as files directly.

```bash
# Impl agent: NO inline — agent reads files itself
flow plan add <dir> --title "phase1-impl" --type headless_agent --model <agent-model> \
  -d 03-design-chat.md -d 04-phase1-plan.md \
  -p "Read the original spec in 03-design-chat.md and plan in 04-phase1-plan.md. Implement Phase 1. Working in /path/to/worktree."
```

**4. Chain dependencies for sequential execution:**
Each phase's planning chat depends on the original spec + all prior phases. Each impl depends on the spec + its own planning chat + the previous impl. This ensures:
- Planning jobs see the full history of what was designed and implemented
- Impl agents see their specific plan + the latest code state

Example dependency chain for a 3-phase feature:

```
01 design chat (multi-turn, completed by coordinator)
 └─ 02 phase1 plan (chat, depends: 01, cx: phase1-relevant files)
    └─ 03 phase1 impl (headless, depends: 01, 02)
       └─ 04 phase2 plan (chat, depends: 01, 02, 03, cx: phase2-relevant files)
          └─ 05 phase2 impl (headless, depends: 01, 04, 03)
             └─ 06 phase3 plan (chat, depends: 01, 04, 05, cx: phase3-relevant files)
                └─ 07 phase3 impl (headless, depends: 01, 06, 05)
```

**5. Batch creation by coordinator:**
The coordinator can create all jobs in one session before any are run. This lets the user review the full plan structure. The dependency chain ensures they execute in order. Each chat job's cx rules should be tailored to the files that phase touches — don't give every phase the same broad context.

**6. Execute sequentially:**
Complete the design chat, then run each job one at a time. Review the planning output before kicking off the impl. After each impl lands, review changes and verify the build before proceeding.

**Always run `flow plan run` in the background** so it doesn't block notifications or other coordinator work. The coordinator session needs to stay responsive to user messages while agents are running.

```bash
# BAD — blocks the coordinator until the job finishes
flow plan run <dir>/05-phase1-impl.md

# GOOD — runs in background, coordinator stays responsive
flow plan run <dir>/05-phase1-impl.md &
# or use the tool's run_in_background parameter if available
```

**Key principles:**
- Each phase must produce a compiling, working build (no half-done refactors)
- Planning jobs get curated context (`flow plan context set`) so the chat model sees the right code
- Impl agents don't need curated context — they read the codebase directly
- Later phases' plans benefit from seeing what the previous agent actually changed (vs what was planned)
- The original design spec should be a dependency of every job so the full architectural vision is always available

## Gotchas and Tips

**Chat jobs land in `pending_user` status after the LLM responds.** You must `flow complete <slug> <job>` before dependent impl jobs can run. Without this, the orchestrator blocks the impl job even though the chat is done.

**Don't curate context for impl agents.** They read the codebase directly and get their plan from the dependency job's chat transcript. Context curation is only valuable for chat/oneshot jobs where the LLM has no file access.

**Don't change job frontmatter.** The `type`, `model`, `template`, and `status` fields are set by the user or `flow plan add`. Changing them silently alters job dispatch behavior. Once a job is `running`, don't edit its frontmatter at all — the daemon may re-read the file and get confused. Only edit idle/pending jobs.

**Agent monitoring during long runs:**

```bash
flow agent list                          # See all running agents
flow agent read <slug> <job> | tail -30  # Peek at recent output
flow agent status <slug> <job>           # idle = waiting for input
```

**When `flow plan run` fails but the agent is already running:** The daemon may have already queued the job from an earlier submission. Check `flow agent list` — if the agent shows up, it's working fine regardless of the CLI error.

**Daemon restart policy — per-worktree daemons are safe to kill, the main one is not.**

Each worktree gets its own `groved` daemon, identified by socket name under `~/.local/state/grove/`:

- `groved-<worktree>-<hash>.sock` — worktree-scoped daemon. Safe to kill/restart freely when picking up a fresh build of daemon code from that worktree. No permission needed.
- `groved-<ecosystem>-<hash>.sock` — main daemon driving the user's primary checkout. **Don't kill without explicit permission.** It may have other in-flight work (long-running envs, background jobs, attached sessions) the user cares about.

Identify which daemon by socket before sending a kill signal: `ls ~/.local/state/grove/groved-*.sock` plus `lsof <socket>` to find the PID. When in doubt, ask.

**Commit after every impl landing:** Headless agents often leave changes uncommitted in submodules. After each agent completes and the build passes, commit the affected submodule(s) before running the next agent. This prevents changes from accumulating across many jobs and getting tangled.

```bash
cd <worktree>/<submodule>
git add -A
git commit -m "feat: description of what the agent implemented"
```

Then bump in the parent:

```bash
cd <worktree>
git add <submodule>
git commit -m "chore: bump <submodule> for <feature>"
```

**Build after every impl landing:**

```bash
grove build --json  # All submodules must pass
```

If a submodule fails, check if the agent left uncommitted changes or broke a cross-repo interface.

**Notifications for async work:** When running background tasks, push results via notify so you don't have to poll:

```bash
notify <channel> "impl job completed, build passing"
```

**Chat template markers are required for multi-turn conversations.** Each user turn in a chat job must be preceded by `<!-- grove: {"template": "chat"} -->` on its own line. Without this marker, the daemon won't recognize the new turn and the LLM response won't be appended. The first marker is placed automatically by `flow plan add`, but follow-up turns must be manually prefixed.

**User content must come after the last `<!-- grove: {"template": "chat"} -->` marker.** The daemon looks for user content after the last marker; if there's nothing after it, the job silently completes without sending anything to the LLM. If the trailing marker (which the LLM appends after its response) is the very last line of the file, delete it — your prompt text must be the final content in the file.

**Dependency blocking is fragile.** The orchestrator sometimes rejects jobs even when dependencies show `completed`. Workaround: remove `depends_on` from the impl job frontmatter and use `prepend_dependencies: true` instead — the agent still gets the chat transcript inlined but isn't gated by status.

**Small changes — do them yourself.** For 1-3 file changes (icon swap, page reorder, one-line fixes), implement directly instead of creating a chat→impl pipeline. The overhead of job creation + context curation + agent spin-up isn't worth it for trivial changes.

**Agents proactively implement beyond the plan.** Headless agents often implement more than requested — e.g. a Phase 1 agent may implement Phase 2 and 3 features too. This is generally good, but verify the extra work compiles and doesn't conflict with subsequent phases. If it does, the later planning job (which sees the dependency chain) will note what's already implemented and focus on what remains.

**The coordinator can do multiple roles in one session:**

1. Direct implementation (small changes, bug fixes)
2. Chat planning + context curation (spec work)
3. Agent dispatch + monitoring (impl delegation)
4. Review + iteration (reading agent output, sending feedback)

Mix these based on complexity — don't force everything through the chat→impl pipeline.

## Debugging with Chat Jobs

When a feature doesn't work and the bug isn't obvious, create a **debug chat job** with the symptoms and evidence:

```bash
flow plan add <dir> --title "debug-feature-X" --type chat --model <chat-model> \
  --template chat --inline=dependencies -d <spec-job>.md \
  -p "## Debug: <symptom>. Evidence: <logs>. Possible causes: <list>. Read <files> and trace the flow."
```

Chat models are good at reading code in context and finding root causes — especially for state machine bugs, message routing issues, and race conditions. Feed them:

- The exact log output showing what works and what doesn't
- Your hypotheses (numbered, specific)
- The exact file paths and line ranges to examine

The response usually identifies the bug precisely. Create a follow-up impl job to fix it.

## Verify Chat Plans Before Creating Impl Jobs

**Never trust a chat model response at face value.** The coordinator must verify every claim against the actual codebase before handing off to an impl agent. Chat models frequently misdiagnose root causes — they reason plausibly about code structure but can be wrong about runtime behavior, message flow, and framework internals.

**Verification workflow:**

1. **Spawn an exploration subagent** to check the model's claims. Read the actual functions, trace the call chains, verify the line numbers and signatures the model references. This protects your main context window from bloat while getting ground truth.

2. **Expand context if needed.** If the initial rules file didn't include enough code for the model to reason correctly, add more files to the rules and re-run. Bigger context is better for tricky bugs — the model can't diagnose what it can't see.

3. **Push back in follow-up turns.** When you find the model got something wrong, append a follow-up turn (after the `<!-- grove: {"template": "chat"} -->` marker) with:
   - The specific claim that's wrong and why (cite actual code, line numbers, framework behavior)
   - What you verified independently
   - Concrete questions to redirect the investigation

4. **Iterate until the plan is tight.** Multi-turn conversations with the chat model are cheap. It's far more expensive to send an impl agent off with a flawed plan — the agent wastes a session implementing the wrong fix, or worse, introduces a regression. Two or three turns of refinement is normal for non-trivial bugs.

5. **Only create the impl job when you're confident** the plan correctly identifies the root cause AND the proposed fix is minimal and correct. The coordinator's job is to be the quality gate between the chat model's analysis and the impl agent's execution.

**Example of what goes wrong without verification:** A chat model may claim a Go value receiver causes state loss in a bubbletea TUI — sounds plausible, but tracing the actual event loop in the framework proves it stores the returned model correctly. Without verification, the impl agent wastes time converting to pointer receivers (a no-op change) while the real bug (a UX trap causing premature form submission) goes unfixed.

## Start Chat Investigations with Broad Context, Narrow Later

When creating diagnostic or design chat jobs, **start with the broadest reasonable context** — include entire directories or repos, not just the files you suspect are relevant. Two critical mistakes to avoid:

1. **Don't bias the chat with your hypothesis.** If you pre-narrow context to only the files you think are buggy, the model can only confirm your theory — it can't discover that the bug is actually in a file you excluded. In one real case, context was curated around the three suspected input-handling files for a key routing bug, but the actual root cause was a missing message handler in a panel file that was never included. The model couldn't possibly find it.

2. **Don't over-specify the problem.** Let the model read broadly and form its own understanding. A fresh read of the full codebase often catches things you've become blind to. The coordinator's job is to provide evidence (logs, test output, user reports) and ask open-ended questions, not to pre-diagnose.

**Narrowing is fine later.** After the first chat turn establishes the landscape, subsequent turns can focus on specific files. And for implementation jobs where the plan is already vetted, narrow context is appropriate. But for the initial diagnostic chat — go wide.

Practical approach: use directory-level `@a:<worktree>:<repo>/internal/panels/` rules instead of individual file rules. For cross-cutting bugs (input handling, key routing, message dispatch), include all the relevant subsystems and orchestration layers. The token cost of broad context is cheap compared to the cost of a wrong diagnosis.

## Convert Oneshots to Chats for Iteration

When creating planning jobs, use `type: chat` instead of `type: oneshot` even for initial plans. This lets you append corrections and follow-up questions after the first response without creating a new job. The pattern: the model produces a plan → you review → append corrections after `<!-- grove: {"template": "chat"} -->` → re-run → iterate until satisfied → complete → create impl job.

## cx Context Footguns

**Never glob terraform directories.** Rules like `@a:<repo>/terraform/<module>/` will pull in `.terraform/` provider binaries and state files (easily 200MB+). Always target specific `.tf` files instead:

```
# BAD — pulls in .terraform/ binaries and state files
@a:<repo>/terraform/<module>/

# GOOD — only the tf files you need
@a:<repo>/terraform/<module>/main.tf
@a:<repo>/terraform/<module>/variables.tf
```

Always run `cx stats --job <file>` after generating context to verify the token count is reasonable for the target model.

**cx context scoping.** When curating context with `cx`, the workspace scope matters. Running `cx generate` from a submodule only resolves files within that submodule's workspace. Cross-repo `@a:<worktree>:<repo>/<path>` rules require running from the **parent ecosystem worktree** root. Use `cx rules where` to verify which workspace and rules file cx is using.

## Structured Logging for TUI Debugging

When debugging TUI applications, use `StructuredOnly()` logs to avoid corrupting the terminal display:

```go
ulog := logging.NewUnifiedLogger("component.feature")
ulog.Info("state changed").Field("key", value).StructuredOnly().Log(ctx)
```

View with `core logs --component component.feature -f` in a separate terminal. Use `GROVE_LOG_LEVEL=debug` for verbose output. Throttle noisy logs (e.g. per-frame ticks) with a counter: `if m.count%15 == 0 { log... }`.

## Embedding Standalone TUIs as Pager Tabs

When wrapping a standalone TUI (one that has its own quit handling) as a pager page:

- Intercept `embed.CloseRequestMsg` in the page adapter so quit doesn't propagate
- Add a `Hosted` flag if the inner model needs to suppress quit keys entirely
- Lazy-init in `Focus()` so the TUI isn't constructed until the tab is first visited
- Expose `IsTextInputFocused()`/`IsSaveMode()` for `PageWithTextInput` gating
