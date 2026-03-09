---
name: grove-system-guide
description: Core operating philosophy and taxonomy for agents in the Grove ecosystem. Read this to understand skill organization and delegation.
domain: grove-system
---

You are an agent operating in the **Grove ecosystem**. This guide explains how skills are organized and how you should delegate work to sub-skills rather than guessing commands.

## The Skill Taxonomy

Skills follow a strict **`{namespace}-{domain}-{role}`** naming convention that defines their scope and responsibilities.

### Namespace (Scope)

The namespace indicates where a skill applies:

| Namespace | Description | Examples |
|-----------|-------------|----------|
| `grove-*` | Universal skills applicable to any repository | `grove-flow-builder`, `grove-cx-builder` |
| `myapp-*` | Skills specific to a particular project | `myapp-api-developer`, `myapp-db-ops` |
| `<project>-*` | Skills specific to other projects | `myapp-api-developer` |

### Domain (Target)

The domain identifies what component or area the skill operates on:

- **Grove domains**: `flow`, `cx`, `refactor`, `feature`, `skill`, `system`
- **Project domains**: `pool`, `cluster`, `manhattan`, `api`, `pipeline`

### Role (What It Does)

The role suffix defines the skill's responsibility:

| Role | Responsibility | Example |
|------|----------------|---------|
| `-coordinator` | Orchestrates multi-step agentic workflows (Plan → Implement → Verify → Commit) | `grove-refactor-coordinator` |
| `-developer` | Standard Operating Procedure (SOP) for building, testing, and iterating on a specific component | `myapp-api-developer` |
| `-ops` | Infrastructure/CLI wrappers that **change state** (create, destroy, deploy) | `myapp-db-ops` |
| `-analyzer` | Observability wrappers that **read state** (logs, metrics, status) | `myapp-metrics-analyzer` |
| `-builder` | Tools that **generate artifacts** (plans, context rules, code) | `grove-flow-builder` |
| `-maintainer` | Updates/evolves existing artifacts, documentation, or skills over time | `grove-concept-maintainer` |
| `-tester` | Writes tests for features or components | `grove-tend-tester` |
| `-debugger` | Diagnoses failures and proposes fixes | `grove-test-debugger` |
| `-guide` | Provides reference documentation for a domain | `grove-logging-guide` |
| `-explorer` | Interactively navigates/explores systems or interfaces | `grove-tui-explorer` |
| `-planner` | Creates plans from specifications or notes | `grove-feature-planner` |

## The Delegation Principle

**CRITICAL**: You must delegate to sub-skills rather than guessing CLI commands.

### Why Delegation Matters

1. **You don't know everything** - Each domain has specific CLI flags, error handling patterns, and best practices that are encoded in dedicated skills.

2. **Maintainability** - When commands change, only the relevant skill needs updating. If you hardcode commands, they become stale.

3. **Composability** - Higher-level skills can be reused across different contexts by swapping out their sub-skills.

### How to Delegate

1. **Discover available skills**:
   ```bash
   skills list
   ```

2. **Visualize skill dependencies**:
   ```bash
   skills tree <skill-name>
   ```

   Example output:
   ```
   grove-refactor-coordinator (Orchestrates multi-phase refactoring)
   ├── grove-flow-builder (Creates execution plans)
   ├── grove-cx-builder (Curates file context)
   └── myapp-api-developer (SOP for testing API changes)
       ├── myapp-db-ops (Creates/destroys test databases)
       └── myapp-metrics-analyzer (Reads logs and metrics)
   ```

3. **Invoke the sub-skill** using the `/skill-name` syntax in your prompt or by reading its SKILL.md content.

### Delegation Rules

| Your Role | You CAN | You CANNOT |
|-----------|---------|------------|
| Coordinator | Invoke developer skills, use builders | Directly run infrastructure commands, guess test procedures |
| Developer | Invoke ops/analyzer skills, run domain-specific build commands | Guess infrastructure provisioning, skip the verification protocol |
| Ops/Analyzer | Execute specific CLI commands for your domain | Make workflow decisions, skip to other phases |

## Skill Dependencies

Skills declare their dependencies using the `requires` field in YAML frontmatter:

```yaml
---
name: myapp-api-developer
description: SOP for building and testing API changes
domain: myapp-api
requires:
  - myapp-db-ops
  - myapp-metrics-analyzer
---
```

When you see a skill with `requires`, you **MUST** read and invoke those sub-skills as part of executing the parent skill's workflow.

## Example: The Refactoring Workflow

Here's how delegation works in practice for a refactoring task:

1. **User invokes** `/grove-refactor-coordinator`

2. **Coordinator** (grove-refactor-coordinator):
   - Runs analysis using `/grove-flow-builder`
   - Curates context using `/grove-cx-builder`
   - Creates implementation job with instruction: "Use `/myapp-api-developer` for verification"

3. **Implementation Agent** reads the prompt and invokes `/myapp-api-developer`

4. **Developer** (myapp-api-developer):
   - Writes the code changes
   - Reads its own SKILL.md, sees `requires: [myapp-db-ops, myapp-metrics-analyzer]`
   - Invokes `/myapp-db-ops` to spin up test database
   - Runs tests
   - Uses `/myapp-metrics-analyzer` to read logs if tests fail
   - Iterates until green
   - Invokes `/myapp-db-ops` to tear down test database

5. **Coordinator** receives success confirmation, commits changes

## Quick Reference

### Finding Skills
```bash
# List all available skills
skills list

# Show skill dependency tree
skills tree <skill-name>

# List with full paths
skills list --path
```

### Skill Locations (Precedence: notebook > user > builtin)
- **Notebook**: `{notebook}/workspaces/{workspace}/skills/`
- **User**: `~/.config/grove/skills/`
- **Built-in**: Embedded in skills binary

### Key Principles

1. **Never guess** - If you don't know a CLI command, find the skill that does
2. **Delegate down** - Coordinators → Developers → Ops/Analyzers, never skip
3. **Read requires** - Always check what sub-skills a skill depends on
4. **Use tree** - Visualize the full dependency graph before starting

## Summary

The Grove skill taxonomy creates a **Directed Acyclic Graph (DAG)** of capabilities. Your job is to traverse this graph correctly:

- **Coordinators** orchestrate workflows
- **Developers** execute domain-specific SOPs
- **Ops/Analyzers/Builders** perform atomic operations

By delegating to the right skills, you ensure your actions are correct, maintainable, and composable.
