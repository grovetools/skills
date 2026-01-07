---
name: skill-creator
description: Guides agents through creating new skills for the grove-skills system, covering structure, installation, and best practices.
---

You are an expert in creating skills for the grove-skills ecosystem. Your goal is to help users create new, well-structured skills that agents can use effectively.

## What is a Skill?

A skill is a reusable prompt/instruction set that extends an agent's capabilities for specific tasks. Skills are installed into agent providers (Claude Code, Codex, OpenCode) and become available as specialized knowledge or behaviors.

## Skill Structure

Every skill must have a `SKILL.md` file as its entry point. The structure is:

```
skill-name/
├── SKILL.md          # Required: Main skill definition with frontmatter
└── (optional files)  # Additional resources, examples, templates
```

### SKILL.md Format

```markdown
---
name: skill-name
description: Brief description of what this skill does and when to use it.
---

# Skill content goes here

Instructions, examples, workflows, and any other content the agent needs.
```

### Frontmatter Requirements

| Field | Required | Constraints |
|-------|----------|-------------|
| `name` | Yes | Lowercase alphanumeric with single hyphens (e.g., `my-skill-name`). Max 64 chars. Must match directory name. |
| `description` | Yes | What the skill does and when to invoke it. Max 1024 chars. |

**Name format examples:**
- Valid: `cx-builder`, `from-note-planner`, `tend-tester`
- Invalid: `CX_Builder`, `my--skill`, `skill_name`, `123skill`

## Skill Types and Patterns

### 1. Interactive Session Skills
Guide agents through multi-step workflows with user collaboration.

**Example pattern:** `spec-builder`, `cx-builder`

```markdown
---
name: feature-planner
description: An interactive session to plan feature implementation with the user.
---

You are now in an interactive session to plan a new feature.

## Workflow

### 1. Gather Requirements
- Read existing documentation
- Ask clarifying questions

### 2. Explore Codebase
- Search for similar patterns
- Identify integration points

### 3. Draft Plan
- Document decisions
- Create actionable steps

## Tips
- Start broad, then narrow
- Ask "why" to understand motivation
```

### 2. Task Execution Skills
Provide instructions for completing specific technical tasks.

**Example pattern:** `tend-tester`, `recipe-writer`

```markdown
---
name: test-writer
description: Instructs agents to write comprehensive tests for new features.
---

You are an expert in writing tests for this project.

## Your Task

When writing tests:

1. **Analyze existing tests** - Review patterns in `tests/` directory
2. **Choose test location** - New file or extend existing
3. **Implement tests** - Follow project conventions
4. **Verify** - Run tests to ensure they pass

## Best Practices

### Assertions
Use descriptive assertion messages:
```go
// Good
v.Contains("shows version info", output, "v1.0")

// Avoid
v.Contains("check", output, "text")
```
```

### 3. Reference/Guide Skills
Provide technical reference material for specific domains.

**Example pattern:** `logging-guide`

```markdown
---
name: api-reference
description: Guide for using the project's API with examples and best practices.
---

You need to use the project API. Here's how it works.

## Quick Start

```go
client := api.NewClient()
result, err := client.Query(ctx, params)
```

## Common Patterns

### Pattern 1: Basic Query
...

### Pattern 2: Batch Operations
...

## Troubleshooting

### "Connection refused"
Check that the server is running...
```

### 4. Trigger-Response Skills
Simple skills that respond to specific phrases or contexts.

**Example pattern:** `explain-with-analogy`

```markdown
---
name: cooking-analogy
description: Explains code using cooking analogies. Use when user says "cooking analogy".
---

When the user asks for a "cooking analogy", explain the code as:

"Think of this code like a recipe. The ingredients (inputs) go through preparation steps (functions), following the recipe instructions (logic), to produce a finished dish (output)."
```

## Installation Locations

Skills can be installed at three precedence levels:

| Source | Location | Precedence | Use Case |
|--------|----------|------------|----------|
| **Notebook** | `{notebook}/workspaces/{workspace}/skills/` | Highest | Team-specific skills |
| **User** | `~/.config/grove/skills/` | Medium | Personal skills |
| **Built-in** | Embedded in grove-skills binary | Lowest | Default skills |

### Installing Skills

```bash
# Install a specific skill
grove-skills skills install skill-name --scope user

# Install to project
grove-skills skills install skill-name --scope project

# Install all available skills
grove-skills skills install all

# Sync all skills (useful for teams)
grove-skills skills sync --scope project

# Sync across ecosystem
grove-skills skills sync --ecosystem --scope project
```

### Listing Available Skills

```bash
grove-skills skills list
```

Output shows skill name and source:
```
SKILL              SOURCE
cx-builder         notebook
spec-builder       notebook
explain-with-analogy  builtin
```

## Writing Effective Skills

### 1. Clear Purpose
Define when and why the skill should be used in the description.

**Good:**
```yaml
description: An interactive session to curate context for a new feature using the cx tool.
```

**Avoid:**
```yaml
description: A skill for doing things with context.
```

### 2. Structured Workflows
Break complex tasks into numbered steps with clear headings.

```markdown
## Workflow

### 1. Understand Requirements
- Read the specification
- Identify key components

### 2. Explore Existing Code
- Search for patterns
- Review similar implementations

### 3. Implement Solution
- Follow project conventions
- Add appropriate tests
```

### 3. Concrete Examples
Include actual commands, code snippets, and expected outputs.

```markdown
### Finding Notes

```bash
nb search "partial-note-name" --json
```

This returns:
```json
{
  "title": "note-title",
  "path": "/full/path/to/note.md"
}
```
```

### 4. Decision Guidance
Help agents make good choices with clear criteria.

```markdown
**Should tests be included?**
- **Exclude tests** when: Writing new features or fixing implementation bugs
- **Include tests** when: The spec mentions test behavior or test failures
```

### 5. Error Handling
Explain what to do when things go wrong.

```markdown
## Error Handling

If any step fails:
- Report the error clearly
- Don't proceed automatically
- Ask the user how to proceed:
  - Retry the failed step
  - Skip and continue
  - Abort the operation
```

### 6. Important Notes Section
Highlight critical information that's easy to miss.

```markdown
## Important Notes

- Always use `-y` flag with `flow plan finish` for non-interactive mode
- Plans are archived, not deleted - results are preserved
- Each plan creates a separate worktree branch
```

## Skill Validation

Skills are validated on installation:

1. **Frontmatter format** - Must start with `---` and end with `---`
2. **Required fields** - `name` and `description` must be present
3. **Name format** - Lowercase alphanumeric with single hyphens
4. **Name matching** - Frontmatter name must match directory name
5. **Length limits** - name ≤ 64 chars, description ≤ 1024 chars

### Skip Validation (Development)

```bash
grove-skills skills install skill-name --skip-validation
```

## Creating Your Skill: Step by Step

1. **Choose a location:**
   - Notebook skills: `{notebook}/workspaces/{workspace}/skills/{skill-name}/`
   - User skills: `~/.config/grove/skills/{skill-name}/`

2. **Create the directory:**
   ```bash
   mkdir -p ~/.config/grove/skills/my-skill
   ```

3. **Create SKILL.md:**
   ```bash
   cat > ~/.config/grove/skills/my-skill/SKILL.md << 'EOF'
   ---
   name: my-skill
   description: Brief description of what this skill does.
   ---

   # My Skill

   Instructions for the agent...
   EOF
   ```

4. **Install and test:**
   ```bash
   # List to verify it's discovered
   grove-skills skills list

   # Install to your project
   grove-skills skills install my-skill --scope project
   ```

5. **Use the skill in an agent session**

## Common Mistakes to Avoid

1. **Vague descriptions** - Be specific about when to use the skill
2. **Missing structure** - Use headers and numbered steps for complex workflows
3. **No examples** - Always include concrete commands and code snippets
4. **Assuming context** - Explain prerequisites and required tools
5. **Too much content** - Focus on actionable guidance, not comprehensive docs

## Template: New Skill Scaffold

```markdown
---
name: skill-name
description: [What this skill does] and [when to use it].
---

You are [role/expertise]. Your goal is to [primary objective].

## Overview

Brief explanation of the skill's purpose.

## Workflow

### 1. [First Step]
- Action items
- Commands to run

### 2. [Second Step]
- Action items
- Commands to run

## Examples

### Example 1: [Scenario]
[Concrete example with commands and outputs]

## Tips

- Helpful advice
- Common patterns
- Things to watch out for

## Troubleshooting

### [Common Issue]
[Solution]
```

## Summary

Creating effective skills:
1. Use clear, specific descriptions
2. Structure content with headings and numbered steps
3. Include concrete examples with actual commands
4. Provide decision guidance for complex choices
5. Handle errors gracefully
6. Test the skill before sharing
