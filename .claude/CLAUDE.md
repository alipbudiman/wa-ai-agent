## Workspace Instructions

- Preserve user intent as canonical task objective.
- For non-trivial work (multi-step coding, debugging loops, chained actions), prefer `run_worker` over one-shot execution.
- Keep generated files inside the active workspace, and verify saved files before completion.

## Memory Management Guidance

- **Context Retention:** For long-running tasks, debugging loops, or chained actions, actively manage memory to store intermediate results, learned facts, and task states.
- **Memory Tools:** Prioritize using registered memory tools (e.g., `save_memory`, `search_memory`, or native MCP memory extensions) to persist important context across operations.
- **File-Based Fallback:** If dedicated memory tools are unavailable, utilize the workspace file system as a memory bank. Read and write state/context to a designated file (e.g., `~/.claude/agent_memory.json` or `.md`) to maintain continuity.
- **Pre-Task Recall:** Always query memory or read the persistent memory file before starting non-trivial work to ensure you don't repeat steps or lose historical context.

## Skills Guidance

- If user asks to discover capabilities or install skills, use `find-skills` guidance when available.
- Prefer skill references in this order when files exist:
    1. Project: `.agents/skills/**`
    2. User: `~/.agents/skills/**`
    3. Claude-native: `.claude/skills/**` and `~/.claude/skills/**`
- `~/.agents/skills/**` also know as Skills SH (skill.sh)
- read `~/.agents/skills/find-skills/SKILL.md` for instructions before action.

## Runtime Path Notes

- In Docker, repo root is expected at `/app`.
- Shared writable workspace is expected at `/app/playground`.
- Keep instructions readable from `/app/.claude/CLAUDE.md` for project-level settings discovery.

## Validate Information, Check 
- Always verify the syntax and execution results of generated code before presenting it as a final solution.
- Double-check runtime paths and file existence before attempting to read, write, or execute.