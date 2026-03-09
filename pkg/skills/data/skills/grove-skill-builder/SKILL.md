---
name: grove-skill-builder
description: Creates new skills. Use when bootstrapping a skill from a session or from scratch.
domain: grove-skill
requires:
  - grove-skill-guide
---

You create new skills for the Grove ecosystem. For structure and templates, invoke `/grove-skill-guide`.

---

## Workflow A: Creating from a Session

When the user wants to capture a successful agent session as a reusable skill:

### Step 1: Extract Commands

List ONLY the shell commands and tool invocations executed:

```
I found these commands in the session:
1. `make build`
2. `skills list`
3. `flow plan add ...`

Which are essential? Which should be excluded?
```

### Step 2: Ask Clarifying Questions

1. **Key insight?** "What was the most important lesson from this session?"

2. **Trigger?** "When should an agent invoke this skill?"

3. **What to remove?** "Any exploration or dead-ends to exclude?"

4. **Success criteria?** "How does an agent know it's done?"

### Step 3: Determine Taxonomy

Ask for:
- **Namespace**: `grove` (universal) or project name
- **Domain**: What component/area
- **Role**: See `/grove-skill-guide` for role types

### Step 4: Write Minimal Skill

Use the minimal template from `/grove-skill-guide`. Principles:

- **Commands first** - Tight sequence of commands
- **No fluff** - Omit prose that doesn't guide action
- **Concrete** - Exact commands, not descriptions
- **One path** - Happy path only

### Step 5: Review

Show draft and ask:
- "Is this the essence of what worked?"
- "Anything unnecessary?"
- "What's missing?"

---

## Workflow B: Creating from Scratch

When the user wants to create a new skill without an existing session:

### Step 1: Gather Taxonomy

Ask for each component:

1. **Namespace** - `grove` or project name (e.g., `myapp`)
2. **Domain** - Target area (e.g., `api`, `db`, `auth`)
3. **Role** - See `/grove-skill-guide` for the full role table

### Step 2: Identify Dependencies

Ask: "Does this skill depend on other skills?"

```bash
skills list
```

Common patterns:
- Coordinators require: builders, developers
- Developers require: ops, analyzers
- Ops/Analyzers/Builders: typically none

### Step 3: Select Template

Based on the role, use the appropriate template from `/grove-skill-guide`:
- Coordinator Template
- Developer Template
- Ops Template
- Analyzer Template
- Builder Template
- Standard Template (for other roles)

### Step 4: Generate Skill

Create the directory and SKILL.md:

```
{skill-name}/
└── SKILL.md
```

### Step 5: Validate

```bash
skills list
skills tree {skill-name}
skills install {skill-name} --scope user
```

---

## Quick Reference

| From | Use |
|------|-----|
| Successful session | Workflow A |
| New idea | Workflow B |
| Update existing | Use `/grove-skill-maintainer` instead |
