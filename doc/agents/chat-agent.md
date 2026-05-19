---
title: "Chat Agent (Router)"
category: "agent"
last_analyzed: "2026-05-03"
---

# Chat Agent (Router)

## Overview
The **Chat Agent** (implemented as `ChatAgentUseCase`) is the primary interface between the Go runtime and the specialized reasoning sub-agents. It acts as both a **Router** and a **Supervisor**, managing the main LangGraph workflow and orchestrating tool usage.

## Workflow

<!-- Chat Agent execution sequence -->
```mermaid
sequenceDiagram
    participant START
    participant Router as RouteQuery
    participant Assistant
    participant Tools
    participant END

    START->>Router: Receive User Message
    Router->>Assistant: Delegate Complex Task
    
    loop Reasoning Loop
        Assistant->>Tools: Request Tool (Search/File/etc)
        Tools-->>Assistant: Return Result
        Assistant->>Assistant: Analyze & Plan Next Step
    end
    
    Assistant->>END: Final Response to User
```

## Step-by-Step Workflow
1. **Trigger**: Triggered by an HTTP POST request from the Go runtime to `/api/v1/chat-agent`.
2. **Intent Analysis**: `route_query` determines if the message is `direct_chat` (no tools) or requires the `assistant` mode (tools enabled).
3. **Context Loading**: Retrieves the latest summary, recent messages from MongoDB, and RAG context (using `MAIN_AGENT` collection vector search).
4. **Reasoning**:
    - If a task is engineering-focused, it calls the `run_worker` tool (dispatching to **Code Agent**).
    - If it's a deep research request, it calls the `research_agent_tool`.
5. **Tool Execution**: Executes system-level tools (Bash, File management, etc.) as requested by the model.
6. **Safety Check**: If a tool is marked as sensitive (e.g. `delete_file_dir`), the flow enters the `human_feedback` state.
7. **Synthesis**: Combines tool results into a coherent final answer.

## Tool Access
The Chat Agent has access to the most extensive toolset in the system:
- **`TimeTool`**: Current date/time awareness.
- **`WebSearchTool`**: Wikipedia and web searching.
- **`FileX` & `FileManagementToolkit`**: Compression, deletion, and file management sandboxed in `playground/`.
- **`TerminalX`**: Command execution (simple and screened).
- **`WhatsAppIntegration`**: Sending messages/files, parsing identifiers, and broadcast management.
- **`AgentCode`**: Specialized engineering sub-agent (`run_worker`).
- **`ResearchAgent`**: Specialized deep reasoning sub-agent (`research_agent_tool`).

## Technical Advantages
- **Tool Coercion**: Features a custom parser (`__extract_textual_tool_calls`) that allows the agent to work with models that don't support native tool-calling (via XML or custom markup).
- **Graceful Summarization**: Automatically manages context window overflow by summarizing conversation history once a token threshold is reached.
- **Safety**: Uses a `_sensitive_tools` list and `_need_decide` rules to detect potentially dangerous shell commands or file operations.

## Output Contract
- **Type**: `AIMessage` or string.
- **Content**: Markdown-formatted text, often containing emojis for a "premium" feel.
- **Metadata**: Includes session/thread IDs for stateful continuation.

## Related Documents
- [Orchestration](../brain/orchestration.md) — The graph structure.
- [Code Agent](code-agent.md) — The engineering sub-agent.
- [System Overview](../architecture/system-overview.md) — Component map.
