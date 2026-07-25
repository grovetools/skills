# Stage contract

A stage succeeds only when its child is joined and every `produces` entry in the executable pipeline exists at the reported relative path beneath that child's artifact directory.

- Treat artifact logical names as the interface; downstream assignments receive joined artifact references, not invented summaries.
- Reject empty, missing, absolute, escaping (`..`), or undeclared artifact paths.
- A report must state outcome, evidence, unresolved risks, and deviations. A completion claim cannot replace a required artifact.
- `single` closes when its one child is recorded joined. `fanout` and `adaptive` remain open while children are registered and close only through `flow_pipeline barrier` after every registered child is terminal and every required artifact set is accepted.
- `optional` permits a recorded `flow_pipeline skip`; it does not permit omission. For `adaptive`, choose the bounded child roster before closing its barrier; the persisted child IDs are the execution-shape record.
- Gate stages produce a recorded decision, not implementation work. Human rejection leaves successors ineligible.
