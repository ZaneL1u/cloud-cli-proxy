---
name: gsd-surface
description: "Toggle which skills are surfaced — apply a profile, list, or disable a cluster without reinstall"
---

<cursor_skill_adapter>
## A. Skill Invocation
- This skill is invoked when the user mentions `gsd-surface` or describes a task matching this skill.
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
Manage the runtime skill surface without reinstall. Reads/writes `/Users/zaneliu/Projects/open-source/cloud-cli-proxy/.cursor/skills/.gsd-surface.json`
(sibling to `.gsd-profile`) and re-stages the active commands/gsd directory in place.

Sub-commands: list · status · profile · disable · enable · reset
</objective>

## Sub-command routing

Parse the first token of {{GSD_ARGS}}:

| Token | Action |
|---|---|
| `list` | Show enabled + disabled clusters and skills |
| `status` | Alias for `list` plus token cost summary |
| `profile <name>` | Write `baseProfile` and re-stage |
| `profile <n1>,<n2>` | Composed profiles (comma-separated, no spaces) |
| `disable <cluster>` | Add cluster to `disabledClusters`, re-stage |
| `enable <cluster>` | Remove cluster from `disabledClusters`, re-stage |
| `reset` | Delete `.gsd-surface.json`, return to install-time profile |
| *(none)* | Treat as `list` |

---

## list / status

Call `listSurface(runtimeConfigDir, manifest, CLUSTERS)` from
`get-shit-done/bin/lib/surface.cjs`. Display:

```
Enabled (N skills, ~T tokens):
  core_loop:   new-project  discuss-phase  plan-phase  execute-phase  help  update
  audit_review: …
  …

Disabled:
  utility:  health  stats  settings  …

Token cost: ~T (budget cap ~500 tokens for 200k context @ 1%)
```

For `status` also append:

```
Base profile:   standard  (from .gsd-surface.json)
Install profile: standard  (from .gsd-profile)
```

---

## profile \<name\>

1. Read current surface: `readSurface(runtimeConfigDir)` → if null, seed from `readActiveProfile(runtimeConfigDir)`.
2. Set `surfaceState.baseProfile = name`.
3. `writeSurface(runtimeConfigDir, surfaceState)`.
4. Resolve and re-apply: `applySurface(runtimeConfigDir, commandsDir, agentsDir, manifest, CLUSTERS)`.
5. Confirm: "Surface updated to profile `<name>`. N skills enabled."

---

## disable \<cluster\>

Valid cluster names: `core_loop`, `audit_review`, `milestone`, `research_ideate`,
`workspace_state`, `docs`, `ui`, `ai_eval`, `ns_meta`, `utility`.

1. Validate cluster name against `Object.keys(CLUSTERS)`.
2. Read or initialize surface state.
3. Add cluster to `surfaceState.disabledClusters` (deduplicate).
4. `writeSurface` → `applySurface`.
5. Confirm: "Disabled cluster `<cluster>`. N skills removed from surface."

---

## enable \<cluster\>

1. Read surface state; if null, nothing to enable — print "No surface delta active."
2. Remove cluster from `surfaceState.disabledClusters`.
3. `writeSurface` → `applySurface`.
4. Confirm: "Enabled cluster `<cluster>`. N skills added back to surface."

---

## reset

1. Check if `.gsd-surface.json` exists.
2. Delete it.
3. Re-apply using only `readActiveProfile(runtimeConfigDir)` (install-time profile).
4. Confirm: "Surface reset to install-time profile `<name>`."

---

## runtimeConfigDir resolution

```bash
# Cursor
RUNTIME_CONFIG_DIR=/Users/zaneliu/Projects/open-source/cloud-cli-proxy/.cursor/skills

# Resolve commandsDir and agentsDir
COMMANDS_DIR=/Users/zaneliu/Projects/open-source/cloud-cli-proxy/.cursor/commands/gsd
AGENTS_DIR=/Users/zaneliu/Projects/open-source/cloud-cli-proxy/.cursor/agents
```

All paths can be overridden by reading the `CLAUDE_CONFIG_DIR` env var if set.

---

## Error handling

- Unknown cluster name → list valid cluster names, exit without writing.
- Unknown profile name → list known profiles (`core`, `standard`, `full`), exit.
- Missing `surface.cjs` → prompt: "Run `npm i -g get-shit-done` to reinstall GSD."

<execution_context>
Surface state file: `/Users/zaneliu/Projects/open-source/cloud-cli-proxy/.cursor/skills/.gsd-surface.json`
Install profile marker: `/Users/zaneliu/Projects/open-source/cloud-cli-proxy/.cursor/skills/.gsd-profile`
Engine module: `/Users/zaneliu/Projects/open-source/cloud-cli-proxy/.cursor/get-shit-done/bin/lib/surface.cjs`
Cluster definitions: `/Users/zaneliu/Projects/open-source/cloud-cli-proxy/.cursor/get-shit-done/bin/lib/clusters.cjs`
</execution_context>
