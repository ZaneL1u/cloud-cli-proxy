---
name: gsd-next
description: "Automatically advance to the next logical step in the GSD workflow"
---

<cursor_skill_adapter>
## A. Skill Invocation
- This skill is invoked when the user mentions `gsd-next` or describes a task matching this skill.
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

<objective>
Detect the current project state and automatically invoke the next logical GSD workflow step.
No arguments needed — reads STATE.md, ROADMAP.md, and phase directories to determine what comes next.

Designed for rapid multi-project workflows where remembering which phase/step you're on is overhead.

Supports `--force` flag to bypass safety gates (checkpoint, error state, verification failures, and prior-phase completeness scan).

Before routing to the next step, scans all prior phases for incomplete work: plans that ran without producing summaries, verification failures without overrides, and phases where discussion happened but planning never ran. When incomplete work is found, shows a structured report and offers three options: defer the gaps to the backlog and continue, stop and resolve manually, or force advance without recording. When prior phases are clean, routes silently with no interruption.
</objective>

<execution_context>
@.cursor/get-shit-done/workflows/next.md
</execution_context>

<process>
Execute the next workflow from @.cursor/get-shit-done/workflows/next.md end-to-end.
</process>
