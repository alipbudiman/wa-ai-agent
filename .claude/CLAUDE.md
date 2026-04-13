## Workspace Instructions

- Preserve user intent as canonical task objective.
- For non-trivial work (multi-step coding, debugging loops, chained actions), prefer `run_worker` over one-shot execution.
- Keep generated files inside the active workspace, and verify saved files before completion.

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


## System & OS Info
- Ubuntu 22.04
- Python 3
- Node.js (npm,npx,etc)
- Go

## Validate Information, Check 
