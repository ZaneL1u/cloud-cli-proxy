---
name: gsd-from-gsd2
description: "Import a GSD-2 (.gsd/) project back to GSD v1 (.planning/) format"
---

<cursor_skill_adapter>
## A. Skill Invocation
- This skill is invoked when the user mentions `gsd-from-gsd2` or describes a task matching this skill.
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
Reverse-migrate a GSD-2 project (`.gsd/` directory) back to GSD v1 (`.planning/`) format.

Maps the GSD-2 hierarchy (Milestone → Slice → Task) to the GSD v1 hierarchy (Milestone sections in ROADMAP.md → Phase → Plan), preserving completion state, research files, and summaries.

**CJS-only:** `from-gsd2` is not on the `gsd-sdk query` registry; call `gsd-tools.cjs` as shown below (see `docs/CLI-TOOLS.md`).
</objective>

<process>

1. **Locate the .gsd/ directory** — check the current working directory (or `--path` argument):
   ```bash
   node ".cursor/get-shit-done/bin/gsd-tools.cjs" from-gsd2 --dry-run
   ```
   If no `.gsd/` is found, report the error and stop.

2. **Show the dry-run preview** — present the full file list and migration statistics to the user. Ask for confirmation before writing anything.

3. **Run the migration** after confirmation:
   ```bash
   node ".cursor/get-shit-done/bin/gsd-tools.cjs" from-gsd2
   ```
   Use `--force` if `.planning/` already exists and the user has confirmed overwrite.

4. **Report the result** — show the `filesWritten` count, `planningDir` path, and the preview summary.

</process>

<notes>
- The migration is non-destructive: `.gsd/` is never modified or removed.
- Pass `--path <dir>` to migrate a project at a different path than the current directory.
- Slices are numbered sequentially across all milestones (M001/S01 → phase 01, M001/S02 → phase 02, M002/S01 → phase 03, etc.).
- Tasks within each slice become plans (T01 → plan 01, T02 → plan 02, etc.).
- Completed slices and tasks carry their done state into ROADMAP.md checkboxes and SUMMARY.md files.
- GSD-2 cost/token ledger, database state, and VS Code extension state cannot be migrated.
</notes>
