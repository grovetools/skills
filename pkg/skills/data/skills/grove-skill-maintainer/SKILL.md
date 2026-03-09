---
name: grove-skill-maintainer
description: Updates existing skills. Use after a session reveals improvements or when skills become stale.
domain: grove-skill
requires:
  - grove-skill-guide
---

You update and evolve existing skills. For structure and templates, invoke `/grove-skill-guide`.

---

## When to Use

- A session revealed a better approach than what's documented
- Commands or flags have changed
- Dependencies (`requires`) need updating
- Skill name doesn't follow taxonomy
- Content has become stale or incorrect

---

## Workflow: Updating from a Session

When a session worked better than the existing skill documents:

### Step 1: Read Current Skill

```bash
skills list --path
```

Read the existing SKILL.md to understand current state.

### Step 2: Extract Session Commands

List commands from the successful session:

```
Session commands:
1. `make build`
2. `flow plan add ...`

Current skill documents:
1. `make compile`  ← different!
2. `flow create ...`  ← outdated!

Which session commands should replace the documented ones?
```

### Step 3: Ask What Changed

1. **What's outdated?** "Which parts of the skill no longer work?"

2. **What's new?** "What did you discover that should be added?"

3. **What's unchanged?** "What should stay exactly as documented?"

### Step 4: Update Minimally

Make surgical edits:
- Replace outdated commands with working ones
- Update flags that changed
- Add new steps only if essential
- Remove steps that are no longer needed

**Do NOT rewrite the entire skill** unless fundamentally broken.

### Step 5: Verify Dependencies

```bash
skills tree {skill-name}
```

Check if `requires` is still accurate:
- Are all required skills still needed?
- Are new dependencies missing?

### Step 6: Validate

```bash
skills install {skill-name} --scope user --force
skills tree {skill-name}
```

---

## Workflow: Fixing Taxonomy

When a skill name doesn't follow `{namespace}-{domain}-{role}`:

### Step 1: Identify Current Name

```bash
skills list --path
```

### Step 2: Determine Correct Name

Using `/grove-skill-guide` role table:
- Namespace: `grove` or project name
- Domain: Component/area
- Role: Responsibility type

### Step 3: Rename

```bash
# Move directory
mv {old-name} {new-name}

# Update frontmatter name field
# Edit SKILL.md: name: {new-name}
```

### Step 4: Update References

Search for old name in other skills:

```bash
rg "{old-name}" ~/.config/grove/skills/
```

Update any skills that reference the old name.

---

## Workflow: Adding Dependencies

When a skill should delegate to sub-skills:

### Step 1: Identify Missing Dependencies

Review the skill's workflow. Does it:
- Execute commands another skill already wraps?
- Duplicate logic from another skill?

### Step 2: Find Candidate Skills

```bash
skills list
skills tree {candidate}
```

### Step 3: Update Requires

Add to frontmatter:

```yaml
requires:
  - {new-dependency}
```

### Step 4: Update Workflow

Replace inline commands with skill invocations:

```markdown
## Before
Run `some-cli create --flags`

## After
Invoke `/{namespace}-{domain}-ops` to create
```

---

## Quick Reference

| Situation | Action |
|-----------|--------|
| Session worked better | Update commands |
| Commands changed | Replace with new syntax |
| Wrong taxonomy | Rename following guide |
| Missing dependency | Add to requires |
| Stale content | Remove or update |
| New skill needed | Use `/grove-skill-builder` instead |
