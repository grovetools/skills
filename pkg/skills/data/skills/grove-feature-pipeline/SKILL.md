---
name: grove-feature-pipeline
description: Coordinates resumable Pi feature pipelines with flow_pipeline and first-class flow_subjob children. Use when a Flow recipe or user selects an agent-verified-feature or quick-fix flavor.
domain: grove-feature
requires: []
---

# Feature Pipeline Coordinator

Coordinate; do not reproduce the specialist SOPs executed by children.

## Start or resume

1. Classify the task as `agent-verified-feature` or `quick-fix` (use the requested flavor unless it is unsafe).
2. Load **exactly one** matching flavor reference and **exactly one** matching YAML asset:

| Flavor | Policy | Executable pipeline |
|---|---|---|
| `agent-verified-feature` | `references/flavors/agent-verified-feature.md` | `assets/pipelines/agent-verified-feature.yml` |
| `quick-fix` | `references/flavors/quick-fix.md` | `assets/pipelines/quick-fix.yml` |

Do not preload the other flavor. The selected YAML is machine-consumed executable input to `flow_pipeline`, never advisory prose.

3. Resolve the selected asset relative to this `SKILL.md` directory and pass its absolute path as `pipeline_path` to `flow_pipeline init` (or pass its exact bytes as `pipeline_yaml`). A bare `assets/...` path would resolve from the worktree cwd and is not reliable. If state exists, use `flow_pipeline status` and resume it rather than reinitializing or guessing.
4. Repeatedly ask `flow_pipeline eligible` for work. For each eligible non-gate stage, compose the assignment from its flavor policy, create the first-class child with `flow_subjob create` (passing the stage's `skill` when declared), then immediately persist the returned child ID with `flow_pipeline record_spawn`. If launch fails after creation, record that same child and recover it—never create a duplicate.
5. Continue useful eligible work while children run. On readiness, call `flow_subjob join` before reading its report or artifacts, then pass the joined report's relative artifact map to `flow_pipeline record_join`. A `single` stage closes on that record; after every registered child of a `fanout` or `adaptive` stage is recorded terminal, close the roster with `flow_pipeline barrier`.
6. Apply `skip`, `approve`, and `retry` only through `flow_pipeline`. Call `flow_pipeline finish` and emit its final digest only after all required stages and cleanup are terminal.

## Invariants

- Never consume a child report before joining it; never fabricate a missing report.
- Never duplicate a stage because launch status is unclear—inspect persisted state first.
- Every mutation stage must have and complete a validation successor.
- Human gates require the human; never auto-approve them.
- Run applicable cleanup even after failure.
- Delegate stage work to the selected specialist skill or stage prompt. Keep orchestration and recorded policy decisions here.
