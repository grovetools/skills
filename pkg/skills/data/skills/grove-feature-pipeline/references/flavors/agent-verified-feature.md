# Agent-verified feature

Use for non-trivial behavior, cross-file changes, or work whose specification benefits from independent challenge before mutation.

`flow_pipeline` consumes `assets/pipelines/agent-verified-feature.yml` as the executable stage graph. This reference supplies coordinator policy only.

## Stage policy

| Stage | Assignment policy |
|---|---|
| `explore` | Fan out independent read-only explorers. Prefer `grove-source-discovery`; ask each for claims tied to current source. Use `assets/prompts/explore.md`. |
| `spec` | Give one author only joined exploration artifacts and the user request. Require scope, acceptance criteria, risks, and validation. |
| `verify-spec` | Give an independent verifier the joined specification and relevant evidence. Use `assets/prompts/verify-spec.md`; retry only for a failed contract, not disagreement concealed as success. |
| `specification-gate` | Present the specification and verification digest to the human. Record their decision verbatim. |
| `implement` | Select the narrowest implementation specialist for the domain. Adaptive means the coordinator records whether one child or bounded fan-out is safe; never assign overlapping mutation ownership. |
| `review` | Run independently from tests against the implementation report and actual diff. Use `assets/prompts/review-code.md`. |
| `tests` | Require exact commands, results, and unresolved failures. Testing must not rely on the implementer's success claim. |
| `concept-sync` | Skip with a recorded reason when no concept is affected; otherwise delegate to `grove-concept-maintainer`. |
| `final-audit` | Reconcile every accepted criterion with joined evidence. Use `assets/prompts/final-audit.md`. |

Artifact acceptance and retry behavior are defined tersely in `../stage-contract.md` and `../recovery-policy.md`; load the relevant reference only when enforcing or recovering that contract. Do not copy specialist procedures into child prompts—name the specialist skill instead.
