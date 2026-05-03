## Completeness Check
Before finishing, verify:
- [ ] Every agent in the codebase has its own page in `agents/`
- [ ] `system-overview.md` has at least one Mermaid diagram
- [ ] `data-flow.md` shows the full message lifecycle
- [ ] `README.md` has a working table of contents linking all docs
- [ ] No module or directory was left without at least a mention
- [ ] Every source file that was read appears in `PATH_READ.md` with a real description
- [ ] Every doc file that was written appears in `PATH_WORK.md` with a real description
- [ ] No row in `PATH_WORK.md` points to a doc that does not exist on disk
- [ ] No source file was documented without first appearing in `PATH_READ.md`
- [ ] Every agent doc includes a step-by-step workflow and at least one Mermaid diagram
- [ ] `tech-stack.md` covers every dependency found in the manifest files
- [ ] Every tool or function registered to an agent is documented
- [ ] Every environment variable found in the codebase is listed and categorized
- [ ] `analysis.md` contains at least one section on architectural advantages
- [ ] All cross-references between docs use relative links and are valid
- [ ] No document contains assumptions — every claim traces back to a real file
- [ ] `PATH_READ.md` and `PATH_WORK.md` are themselves listed in `README.md`

### Per-Agent Writing Requirements
Every agent document in `agents/` MUST contain:

- [ ] A **step-by-step workflow** section — numbered list tracing every action
      the agent takes from trigger to completion, based on actual code found
- [ ] A **Mermaid sequence diagram** showing the full interaction flow between
      the agent, the LLM, any tools it calls, memory, and the user
- [ ] A **Mermaid flowchart or stateDiagram** if the agent has branching logic,
      decision points, or state transitions

No agent document is considered complete without all three.
If a diagram cannot be produced because the logic is unclear,
note it explicitly in `notes/analysis.md` as a gap.