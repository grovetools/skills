`skills` is a command-line tool for managing and distributing context-aware instructions (skills) to local AI agent runtimes. It aggregates prompt fragments from multiple sources—embedded defaults, user configurations, and workspace notebooks—and synchronizes them to provider-specific directories.

## Core Mechanisms

**Tiered Discovery**: The tool resolves skills by querying sources in a specific precedence order. A skill defined in a higher-precedence source overrides one with the same name from a lower source:
1.  **Project Notebook**: Skills defined in the current project's `nb` workspace (`.../notebooks/nb/workspaces/<project>/skills/`).
2.  **Ecosystem Notebook**: Skills defined in the parent ecosystem's notebook.
3.  **User Configuration**: Skills stored in `~/.config/grove/skills/` (or `XDG_CONFIG_HOME`).
4.  **Built-in**: Default skills embedded directly in the `skills` binary.

**Provider Abstraction**: `skills` normalizes the installation targets for supported agents. It reads a standardized `SKILL.md` format (containing YAML frontmatter and Markdown instructions) and writes it to the filesystem location required by the specific runtime (e.g., `.claude/skills` for Claude Code or `.opencode/skill` for OpenCode).

**Ecosystem Synchronization**: When executed from an ecosystem root with the `--ecosystem` flag, the tool iterates through all child projects defined in the workspace. It pushes relevant skills to each project's configuration directory, ensuring consistent agent behavior across a monorepo or multi-project environment.

**Seeding Scope**: Each layer of the configuration cascade declares where its own skills are seeded, via the `scope` key in its `[skills]` block. This bounds the ecosystem fan-out described above so that shared skills need not be copied into every member repository.

## Configuration

The `[skills]` block in `grove.toml` declares a workspace's active skill set:

```toml
[skills]
use = ["explain-with-analogy", "grove-maintainer"]
providers = ["claude", "codex"]   # default: ["claude"]
scope = "all"                     # or "ecosystem-root"; default: "all"
```

`LoadSkillsConfig` merges five layers — the global base, `[skills.ecosystems.<name>]`, the ecosystem's `grove.toml`, `[skills.projects.<name>]`, and the project's own `grove.toml` — into one effective set per workspace.

`scope` is a property of the block that declares it, not of the merged result. Every layer carries its own, and it is evaluated against the workspace being synced *before* that layer is merged in:

*   **`all`** (default): the block reaches every workspace that inherits it — the ecosystem root **and** each of its member repositories.
*   **`ecosystem-root`**: the block reaches only ecosystem roots and ecosystem worktrees, plus standalone projects (which are their own root, with no higher-level copy to fall back on). Members of an ecosystem are skipped.

`ecosystem-root` exists for the ecosystem-top workflow. Under `all`, an agent working at the top of an ecosystem loads the same skill set once per module it touches, because every member repository carries its own identical `.claude/skills` copy. Narrowing the shared layers seeds them once, at the root, while repository-specific declarations are untouched:

```toml
# ~/.config/grove/grove.toml — ecosystem-wide skills, seeded once at the root
[skills]
use = ["grove-skill-builder", "grove-skill-guide"]
scope = "ecosystem-root"

# still seeded into the member repository itself
[skills.projects.flow]
use = ["flow-builder"]
```

When every layer that would apply to a workspace is scoped away, `sync --prune` clears that workspace's previously seeded copies — in its worktrees as well — and reports the empty result as *scoped to the ecosystem root* rather than as nothing configured. An unrecognized `scope` value is a configuration error, not a silent fallback to the default.

## Supported Providers

`skills` manages configurations for the following local runtimes:

*   **Claude Code**: Installs to `.claude/skills/` (Project scope) or `~/.claude/skills/` (User scope).
*   **Codex**: Installs to `.codex/skills/` or `/etc/codex/skills/` (Admin scope).
*   **OpenCode**: Installs to `.opencode/skill/`.

## Features

*   **`skills list`**: Displays available skills and their origin source (e.g., `builtin`, `user`, `project`).
*   **`skills install`**: Installs a specific skill to a target scope and provider. Validates the `SKILL.md` frontmatter to ensure required fields (`name`, `description`) exist.
*   **`skills sync`**: Performs a bulk installation of all discoverable skills.
    *   **`--here`**: Syncs all skills to the current directory's Git root (useful for worktrees).
    *   **`--ecosystem`**: Distributes skills to all projects within the current ecosystem.
    *   **`--prune`**: Removes skills from the destination that no longer exist in the source.
*   **`skills remove`**: Deletes an installed skill from the specified scope.

