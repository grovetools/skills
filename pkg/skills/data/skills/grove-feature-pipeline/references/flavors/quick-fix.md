# Quick fix

Use only when the defect and ownership are narrow, the change is reversible, no design or human safety gate is needed, and one implementation child can own the mutation. Escalate to `agent-verified-feature` before spawning mutation work if these assumptions fail.

`flow_pipeline` consumes `assets/pipelines/quick-fix.yml` as the executable stage graph. This reference supplies coordinator policy only.

## Stage policy

| Stage | Assignment policy |
|---|---|
| `diagnose` | One read-only child reproduces or source-verifies the cause and states the minimal fix boundary. Use `assets/prompts/explore.md` with quick-fix scope. |
| `implement` | One domain specialist implements only the joined diagnosis and reports changed paths plus risks. |
| `validate` | A separate child or independently executed validation checks the regression and focused package tests; require exact commands and results. |
| `review` | Independently inspect the actual diff for scope creep and regressions. Use `assets/prompts/review-code.md`. |
| `final-audit` | Reconcile the diagnosis, diff, validation, and review. Use `assets/prompts/final-audit.md`. |

Artifact acceptance and retry behavior are defined tersely in `../stage-contract.md` and `../recovery-policy.md`; load the relevant reference only when enforcing or recovering that contract. A failed diagnosis, broadened mutation, or unresolved review concern requires recorded escalation, not silent expansion of this flavor.
