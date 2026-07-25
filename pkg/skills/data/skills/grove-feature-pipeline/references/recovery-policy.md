# Recovery policy

Recover from persisted `flow_pipeline` state; never infer lifecycle from an absent notification.

1. Inspect pipeline status and registered child IDs.
2. If a child is registered, query/status or join it—do not respawn the stage. If `flow_subjob create` returned a child but launch failed, persist that exact ID with `record_spawn` before recovery.
3. After joining, pass the report's relative artifact map to `record_join`. If the result violates its stage contract, record that child with `outcome: failed`; if attempts remain, call `flow_pipeline retry` before creating the replacement child.
4. If attempts are exhausted, a human gate is rejected, or mutation ownership became unsafe, stop successors and report the blocking evidence.
5. Retry only the failed stage unless `flow_pipeline` invalidates successors. Never reuse artifacts from a failed attempt as accepted inputs.
6. Run eligible cleanup after failure. Human gates remain human during recovery.

Transient launch uncertainty is not a failed attempt until persisted state and child status establish that no registered execution can complete.
