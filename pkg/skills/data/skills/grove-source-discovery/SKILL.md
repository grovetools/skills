---
name: grove-source-discovery
description: Progressive Grove architecture-to-source discovery using concepts, context preview, memory, grep, and narrow reads. Use when tracing an unfamiliar Grove subsystem or planning changes across repositories.
metadata:
  protocol: "concepts-context-memory-source-v1"
---

# Grove Source Discovery

Use progressive disclosure; do not load a preset's contents automatically.

1. Search `grove_concepts` with short ownership/boundary terms.
2. Show at most one or two relevant concepts. Form a small map of owners, boundaries, and likely call paths.
3. If the concept names a canonical preset and file cost is uncertain, call `grove_context preview` with `workspace:concept-id`. Preview reports composition and cost, not relevance; stop if it is broad rather than loading it wholesale.
4. Search `grove_memory` with a targeted symbol, behavior, or failure question. Use `type`/`scope` only when they reduce noise.
5. Inspect at most one or two promising opaque locators. If results disagree or degradation is reported, refine the query instead of wandering.
6. Use `rg` for exact symbols and call sites, then `read` narrow current source ranges and relevant tests. Code is authoritative; indexed chunks and concepts may lag it.

Stop and refine when concept results remain broad, two concept shows do not establish ownership, memory returns weak/repeated hits, or a context preview exceeds the task's likely scope. Read a complete file only when exact editing or quotation requires it.
