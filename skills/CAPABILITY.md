# AI Capabilities (Current Runtime)

The assistant can be used through natural conversation.
Operational slash commands are defined in COMMAND_LISTS.md.

## 1) WhatsApp Messaging and Contact Operations

- Send a WhatsApp message when recipient and message are explicitly available.
- Accept recipient as either:
	- numeric identifier (example: 6281234567890), or
	- full JID (example: 867051314767696@lid).
- Normalize and validate recipient identifiers before send.
- Get user/contact information by explicit target number or JID.
- Broadcast one-way announcement messages.

## 2) On-Hold Message Safety Flow

- If target recipient differs from active chat sender identity, message is not sent immediately.
- Message is stored as on-hold and marked with structured status:
	- status: on_hold
	- on_hold: true
- Assistant can check pending status using database-backed pending lookup.
- Pending send/cancel operational flow is handled via slash commands (see COMMAND_LISTS.md).

## 3) Web and Time Utilities

- Web search using Tavily for external and up-to-date information.
- Search results include source URLs so references can be surfaced in responses.
- Current time retrieval for time-aware responses.

## 4) Terminal and Worker Execution

- simple_exec:
	- Executes one-shot non-interactive shell commands.
	- Returns real stdout/stderr output (with safe truncation for very large output).
	- Rejects complex command patterns and routes them to worker flow.

- run_worker:
	- Handles multi-step engineering tasks, code/file workflows, and iterative troubleshooting.
	- Uses Claude agent runtime with scoped tools: Bash, Read, Write, Edit, Glob, LS, TodoWrite, TodoRead.
	- Supports persistent working directory context.
	- Sends step progress updates back to the active WhatsApp chat during execution.
	- Loads skill guidance from local or global skill paths when available.

## 5) Skills Runtime Integration

- Skills CLI (`npx skills`) is available in both Docker services (`wa-agent` and `brain`).
- `find-skills` is preinstalled in container runtime (global scope).
- Runtime can discover skill guidance from these paths:
	- project-local: `.agents/skills/find-skills/SKILL.md`
	- global: `~/.agents/skills/find-skills/SKILL.md`
- Repository lock metadata is used when present: `skills-lock.json`.
- Skill-assisted discovery is intended for capability lookup and recommendation workflows.

## 6) Conversation and Command Guard Behavior

- Carries valid context across turns within the same conversation.
- Corrects malformed command attempts (example: !help, .ping) to slash-command format guidance.
- Uses command menu as source of truth for command suggestions.

## 7) Safety and Operating Constraints

- No send action when recipient is name-only alias without numeric/JID confirmation.
- For send action, assistant asks for missing required fields instead of guessing.
- On tool failure, assistant reports the real error and does not fabricate results.
- Clarification attempts are bounded; unresolved ambiguity is aborted with explanation.
- Destructive actions require explicit user confirmation.
- Default worker workspace is playground/ unless user explicitly requests another path.

## 8) Current Limits and Notes

- Historical pending-message retrieval exists but is marked as reserved/future-oriented.
- Web search requires TAVILY_API_KEY to be configured.
- Worker execution depends on configured model/runtime availability.
- Current default bundled skill is `find-skills`; additional skills are not auto-bundled.
- No dedicated slash commands exist yet for `skills find/add/update/check`; use terminal/worker flow.
- Skill install/update changes in Docker require image rebuild/recreate to be reflected in running containers.
- Scheduler-related workflows are not yet exposed as a dedicated public command flow in this capability list.