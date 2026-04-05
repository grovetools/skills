---
name: skill-inject
description: Inject a new sequence of skills into your current session to handle a specific workflow.
domain: grove-flow
---

You have been asked to inject and execute a skill sequence.

## Execution Instructions

1. Identify the skill(s) you need to inject based on the user's request.
2. Run the following command in your shell to generate the execution plan:
   ```
   flow skill sequence <skill1>,<skill2> --inline
   ```
3. Read the XML output emitted by the command.
4. **Create a TODO list** in your scratchpad exactly as prescribed by the `<skill_sequence>` block in the output.
5. **Read the `<skill_content>`** block provided in the output to understand the rules and system instructions for the skills you are about to execute.
6. Begin working through the TODO list in order.
   - Follow the completion protocols exactly as stated in the output (e.g., using `flow artifact complete` if instructed).
   - If no artifact protocol is present, mark items done on your internal TODO list.
