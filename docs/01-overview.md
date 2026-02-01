`skills` is a command-line tool for managing and distributing context-aware instructions (skills) to local AI agent runtimes. It aggregates prompt fragments from multiple sources—embedded defaults, user configurations, and workspace notebooks—and synchronizes them to provider-specific directories.

## Core Mechanisms

**Tiered Discovery**: The tool resolves skills by querying sources in a specific precedence order. A skill defined in a higher-precedence source overrides one with the same name from a lower source:
1.  **Project Notebook**: Skills defined in the current project's `nb` workspace (`.../notebooks/nb/workspaces/<project>/skills/`).
2.  **Ecosystem Notebook**: Skills defined in the parent ecosystem's notebook.
3.  **User Configuration**: Skills stored in `~/.config/grove/skills/` (or `XDG_CONFIG_HOME`).
4.  **Built-in**: Default skills embedded directly in the `skills` binary.

**Provider Abstraction**: `skills` normalizes the installation targets for supported agents. It reads a standardized `SKILL.md` format (containing YAML frontmatter and Markdown instructions) and writes it to the filesystem location required by the specific runtime (e.g., `.claude/skills` for Claude Code or `.opencode/skill` for OpenCode).

**Ecosystem Synchronization**: When executed from an ecosystem root with the `--ecosystem` flag, the tool iterates through all child projects defined in the workspace. It pushes relevant skills to each project's configuration directory, ensuring consistent agent behavior across a monorepo or multi-project environment.

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

