---
name: grove-skill-guide
description: Reference for skill structure, templates, naming rules, and frontmatter fields.
domain: grove-skill
requires:
  - grove-system-guide
---

This is the **structural reference** for Grove skills. Use this when creating or updating skills to ensure consistent formatting.

For ecosystem philosophy and delegation principles, see `/grove-system-guide`.

## Naming Convention

Skills follow the **`{namespace}-{domain}-{role}`** pattern:

### Components

| Component | Description | Examples |
|-----------|-------------|----------|
| **Namespace** | Scope of the skill | `grove` (universal), `myapp` (project-specific) |
| **Domain** | Target component/area | `flow`, `cx`, `api`, `db`, `auth` |
| **Role** | Responsibility type | See role table below |

### Role Types

| Role | Use When |
|------|----------|
| `-coordinator` | Orchestrating multi-step workflows |
| `-developer` | Defining SOPs for building/testing/iterating |
| `-ops` | CLI commands that **change state** |
| `-analyzer` | CLI commands that **read state** |
| `-builder` | Creating artifacts |
| `-maintainer` | Updating existing artifacts/docs/skills |
| `-tester` | Writing tests |
| `-debugger` | Diagnosing failures |
| `-guide` | Reference documentation |
| `-explorer` | Interactive navigation/exploration |
| `-planner` | Creating plans from specs |

### Validation Rules

1. All lowercase, hyphen-separated
2. No double hyphens (`--`)
3. Max 64 characters
4. Only alphanumeric and single hyphens

**Valid**: `grove-refactor-coordinator`, `myapp-api-developer`
**Invalid**: `RefactorCoordinator`, `grove--flow-builder`, `api-developer`

## Frontmatter Fields

```yaml
---
name: {namespace}-{domain}-{role}
description: {What it does and when to use it}
domain: {namespace}-{domain}
requires:
  - {sub-skill-1}
  - {sub-skill-2}
---
```

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Must match directory name |
| `description` | Yes | Max 1024 chars |
| `domain` | Recommended | The namespace-domain combo |
| `requires` | If applicable | List of sub-skills |

## Templates

### Minimal Template (from session)

```markdown
---
name: {namespace}-{domain}-{role}
description: {One sentence}
domain: {namespace}-{domain}
requires: []
---

{One sentence role statement}

## Trigger

Invoke this skill when: {condition}

## Workflow

### 1. {Phase}
```bash
{command}
```

### 2. {Phase}
```bash
{command}
```

## Success Criteria

- {Observable outcome}

## Key Insight

{Essential lesson}
```

### Standard Template

```markdown
---
name: {namespace}-{domain}-{role}
description: {Brief description}
domain: {namespace}-{domain}
requires:
  - {dependency}
---

> **ROLE**: {Role type}
> **REQUIRED SUB-SKILLS**: {list or "None"}
> **DELEGATION RULE**: {What to delegate vs. do directly}

# {Skill Name}

{Primary objective}

## Workflow

### 1. {Phase}
- Action items
- Commands or skills to invoke

### 2. {Phase}
- Action items

## Required Sub-Skills

### {sub-skill}
Invoke to: {purpose}

## Examples

### {Scenario}
{Commands and outputs}

## Important Notes

- {Critical info}
```

### Coordinator Template

```markdown
---
name: {namespace}-{domain}-coordinator
description: Orchestrates {domain} workflows from planning to completion.
domain: {namespace}-{domain}
requires:
  - grove-flow-builder
  - grove-cx-builder
  - {namespace}-{domain}-developer
---

> **ROLE**: Coordinator
> **REQUIRED SUB-SKILLS**: grove-flow-builder, grove-cx-builder, {namespace}-{domain}-developer
> **DELEGATION RULE**: Orchestrate workflow, delegate domain operations to developer skill.

# {Domain} Coordinator

## Workflow

### Phase 1: Analysis
- Use `/grove-flow-builder` to create execution plan
- Use `/grove-cx-builder` to curate context

### Phase 2: Implementation
- Create implementation job
- Inject instruction to use `/{namespace}-{domain}-developer`

### Phase 3: Verification
- Implementation agent invokes developer skill
- Wait for completion

### Phase 4: Completion
- Commit changes once verified
```

### Developer Template

```markdown
---
name: {namespace}-{domain}-developer
description: SOP for building and testing {domain} components.
domain: {namespace}-{domain}
requires:
  - {namespace}-{domain}-ops
  - {namespace}-{domain}-analyzer
---

> **ROLE**: Developer
> **REQUIRED SUB-SKILLS**: {namespace}-{domain}-ops, {namespace}-{domain}-analyzer
> **DELEGATION RULE**: Execute build/test SOP, delegate infrastructure to ops/analyzer.

# {Domain} Developer

## Autonomous Validation Protocol

1. **Build**: Run build command
2. **Setup**: Invoke `/{namespace}-{domain}-ops` to create environment
3. **Execute**: Run tests
4. **Monitor**: If fail, invoke `/{namespace}-{domain}-analyzer`
5. **Iterate**: Fix and re-run (don't tear down until passing)
6. **Teardown**: Invoke `/{namespace}-{domain}-ops` to destroy
```

### Ops Template

```markdown
---
name: {namespace}-{domain}-ops
description: Infrastructure operations for {domain}.
domain: {namespace}-{domain}
---

> **ROLE**: Ops
> **REQUIRED SUB-SKILLS**: None
> **DELEGATION RULE**: Execute CLI commands. Do NOT make workflow decisions.

# {Domain} Operations

## Commands

### Create
```bash
{command}
```

### Destroy
```bash
{command}
```

## Common Issues

### {Issue}
**Symptom**: {description}
**Solution**: {fix}
```

### Analyzer Template

```markdown
---
name: {namespace}-{domain}-analyzer
description: Observability for {domain}.
domain: {namespace}-{domain}
---

> **ROLE**: Analyzer
> **REQUIRED SUB-SKILLS**: None
> **DELEGATION RULE**: Read state. Do NOT modify infrastructure.

# {Domain} Analyzer

## Commands

### View Logs
```bash
{command}
```

### Check Status
```bash
{command}
```

## Interpreting Output

### Success Indicators
- {indicator}

### Failure Indicators
- {indicator}
```

### Builder Template

```markdown
---
name: {namespace}-{domain}-builder
description: Generates {domain} artifacts.
domain: {namespace}-{domain}
---

> **ROLE**: Builder
> **REQUIRED SUB-SKILLS**: None
> **DELEGATION RULE**: Create artifacts. Do NOT execute workflows.

# {Domain} Builder

## Artifacts

### {Type}
```bash
{command}
```

## Output Formats

{Description}
```

## Validation Commands

```bash
# Check skill appears in list
skills list

# Verify dependency tree
skills tree {skill-name}

# Install to validate structure
skills install {skill-name} --scope user
```
