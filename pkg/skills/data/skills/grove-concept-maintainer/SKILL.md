---
name: grove-concept-maintainer
description: Keeps the grovetools concept library (concept docs + cx context presets) aligned with code reality. Use after implementing features, when docs drift from code, to deepen a seeded stub into a full concept, or to add a concept for a new subsystem.
domain: grove-concept
requires: []
---

You are the Guardian of Conceptual Integrity. Concepts pair a doc (`overview.md`) with a loadable cx context preset; both must describe what the code *is*, not what it was or will be. Code is truth.

## Trigger

Invoke this skill when:
- **Post-implementation**: a feature/refactor landed that touches a documented subsystem
- **Drift noticed**: a concept's prose, key locations, or preset no longer match the code
- **Deepening**: upgrading a `status: stub` seed into a full concept (includes context refinement)
- **New subsystem**: a distinct new mechanism needs a concept + preset pair

## Library Layout

```
<notebook>/workspaces/<ws>/concepts/<id>/
├── concept-manifest.yml   # id, title, description, status (stub|active), related_* arrays
├── overview.md            # frontmatter + Role / Key locations / Related concepts / ## Context
├── implementation.md      # file map
└── research.json          # {inventory, analyst} survey data — seed material for deepening
<notebook>/workspaces/<ws>/context/presets/concept-<id>.rules
```

Run `nb`/`cx` from the matching repo dir (`cx rules where` to confirm); the ecosystem-root workspace runs from the ecosystem root and its presets are alias-form only. Load a preset with `cx rules load <path>` or reference it from any repo as `@a:<ws>::concept-<id>`.

## Workflow 1: Post-Change Update

### 1. Find affected concepts
```bash
nb concept search "<keyword>" --ecosystem --files-only --json
nb concept list --ecosystem --json
cat $(nb concept path <id>)/overview.md
```

### 2. Update the docs
- Fix `overview.md` Role prose and Key locations against the new code (verify every cited path with `ls`, every symbol with `grep -n`)
- Update `implementation.md` file map
- History/why goes in a linked `*-history.md` note, never in overview.md

### 3. Re-verify the preset
Run the Preset Verification SOP (below). If file/token counts changed, re-stamp the overview's `## Context` line with the measured numbers.

### 4. Update links
```bash
nb concept link concept <id> <ws>:<other-id>     # new relations, both directions
nb concept link plan <id> <plan-ref>             # implementation plans
```

### 5. Refresh the architecture map
Concept maps (concepts scaffolded by `nb concept map`, e.g. `grovetools:grove-architecture`) model repo/subsystem structure and relationships in LikeC4 — they drift like prose does, and `validate` only catches syntax, not a relationship that quietly stopped being true. If the change added/removed a repo, moved a responsibility between repos, or altered who-calls-whom at the subsystem level:
```bash
cat $(nb concept path <map-id>)/overview.md        # the map plan + relationship inventory
# edit src/*.c4 (relationship lines cite concepts — keep citations current)
nb concept map validate <map-id> --file src/<edited>.c4
```
Skip this step for changes contained inside one repo that don't move any seam. Deeper map work (new views, decomposition) is the `grove-concept-mapper` skill's job, not this one's.

## Workflow 2: Deepen a Stub

### 1. Load the seed material
```bash
cat $(nb concept path <id>)/research.json | jq .   # paid-for survey analysis — start here, don't re-research
cx rules load <notebook>/workspaces/<ws>/context/presets/concept-<id>.rules && cx generate
```

### 2. Deep-read the scoped code and rewrite
- Rewrite `overview.md` body as full prose: mechanism, data flow, invariants, diagrams where they earn their place. Keep the `## Context` section.
- Rewrite `implementation.md` with line-level accuracy; remove the seed-map caveat.
- Every claim verified against code — stubs tolerate assembly, full concepts do not.

### 3. Refine the context
This is where context judgment lives: prune oversized scopes (target 20–150k), narrow whole-package pulls to the files that matter (grep the consumer's imports — often 2 of 11 subpackages are actually used), fix anything the seed pass deferred. Run the Preset Verification SOP; re-stamp.

### 4. Flip status
```bash
# concept-manifest.yml: status: stub -> status: active
grep -rl "status: stub" <notebook>/workspaces/*/concepts/*/concept-manifest.yml | wc -l   # remaining debt
```

## Workflow 3: New Concept

```bash
cd <repo> && nb concept new "<Title>" --id <stable-id> --json   # --id pins the dir name; never let the title slug become the id
```
Then: write overview.md (use the seed structure for a quick stub, or full prose), implementation.md, the preset, run the Preset Verification SOP, and link relations. Set `status:` honestly (`stub` if assembled, `active` if code-verified).

## Preset Verification SOP

Rules files have sharp edges; all of these were found empirically:

1. **Short-form aliases only**: `@a:<repo>/<path>` or `@a:<repo>::<preset>` — never `@a:<ecosystem>:<repo>/...`. The ecosystem prefix pins the primary checkout and breaks in worktrees; short form resolves context-aware from cwd.
2. **Rule order matters across the alias boundary**: a `!*_test.go` placed *before* `@a:` directory lines does not filter them — later rules win. Generic exclusions go *after* all `@a:` lines; exact-path test re-includes (when tests are deliberately in scope) go last.
   **And the trailing generic exclude is still not enough for imported presets**: when the preset is consumed via `@a:<ws>::concept-<id>` from another repo, the generic negation re-roots to the home repo and stops filtering alias files. Presets with dir-level `@a:` lines need alias-scoped negations too — `!@a:<repo>/<dir>/**/*_test.go` per aliased directory. Prove it both ways: `cx stats` on the preset directly AND on a one-line `@a:<ws>::concept-<id>` rules file from another repo must agree.
3. **`cx lint` is blind to dead `@a:` lines** — it reports clean even when a cross-repo path matches nothing. Verify each `@a:` line contributes: put the single line in a temp rules file and check `cx stats --rules-file` shows >0 files; fix or drop dead lines.
4. **Prove exclusions with the file list**: `cx list --rules-file <preset> | grep '_test\.'` — stats totals hide individual leaks.
5. **Compile + cap**:
   ```bash
   cx lint --rules-file <preset>      # errors fail; zero-match warnings on ! lines are harmless
   cx stats --rules-file <preset>     # must match >0 files; keep under ~200k tokens (target 20–150k)
   ```
   Estimate before pulling a big package: `find <dir> -name '*.go' | xargs wc -c` ÷ 4000 ≈ k-tokens (within ~7% of cx).
6. **Re-stamp**: the overview `## Context` line carries the measured `(~Nk tokens, M files)` — update it whenever the preset changes.
7. Cross-repo *conventions* ("pidfile conventions", "grove.yml format") are not paths — point at them from implementation.md, don't fake a pattern.

## Success Criteria

- Overview/implementation describe current code; every cited path exists, every symbol greps
- Preset compiles >0 files, under cap, no unintended test files, no dead `@a:` lines, measured numbers stamped
- `status:` reflects reality (stub = assembled from survey, active = code-verified)
- New/changed relations linked via `nb concept link`
- Architecture maps still `validate` clean and their relationship lines still match reality (when a seam moved)
- Code repos untouched — this skill only writes inside the notebook

## Key Insight

The preset is half the concept. A correct doc with a stale context still sends agents to the wrong files — verify the context with the same rigor as the prose, and always re-stamp the measured numbers so readers can trust the `## Context` line.
