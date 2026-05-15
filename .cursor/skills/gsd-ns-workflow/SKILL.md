---
name: gsd-ns-workflow
description: "workflow | discuss plan execute verify phase progress"
---

<cursor_skill_adapter>
## A. Skill Invocation
- This skill is invoked when the user mentions `gsd-ns-workflow` or describes a task matching this skill.
- Treat all user text after the skill mention as `{{GSD_ARGS}}`.
- If no arguments are present, treat `{{GSD_ARGS}}` as empty.

## B. User Prompting
When the workflow needs user input, prompt the user conversationally:
- Present options as a numbered list in your response text
- Ask the user to reply with their choice
- For multi-select, ask for comma-separated numbers

## C. Tool Usage
Use these Cursor tools when executing GSD workflows:
- `Shell` for running commands (terminal operations)
- `StrReplace` for editing existing files
- `Read`, `Write`, `Glob`, `Grep`, `Task`, `WebSearch`, `WebFetch`, `TodoWrite` as needed

## D. Subagent Spawning
When the workflow needs to spawn a subagent:
- Use `Task(subagent_type="generalPurpose", ...)`
- The `model` parameter maps to Cursor's model options (e.g., "fast")
</cursor_skill_adapter>

Route to the appropriate phase-pipeline skill based on the user's intent.
Sub-skill names below are post-#2790 consolidated targets — `gsd-phase`
absorbs the former add/insert/remove/edit-phase commands and `gsd-progress`
absorbs the former next/do commands.

| User wants | Invoke |
|---|---|
| Gather context before planning | gsd-discuss-phase |
| Clarify what a phase delivers | gsd-spec-phase |
| Create a PLAN.md | gsd-plan-phase |
| Execute plans in a phase | gsd-execute-phase |
| Verify built features through UAT | gsd-verify-work |
| Add / insert / remove / edit a phase | gsd-phase |
| Advance to the next logical step | gsd-progress |
| Offload planning to the ultraplan cloud | gsd-ultraplan-phase |
| Cross-AI plan review convergence loop | gsd-plan-review-convergence |

Invoke the matched skill directly using the Skill tool.
