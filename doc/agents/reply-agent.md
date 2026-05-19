---
title: "Reply Agent (Sofia)"
category: "agent"
last_analyzed: "2026-05-03"
---

# Reply Agent (Sofia)

## Overview
The **Reply Agent** (implemented as `ReplyAgentUsecase`), also known as **Sofia**, is a specialized conversationalist that uses **Retrieval-Augmented Generation (RAG)** to answer questions based on a local knowledge base.

## Workflow

<!-- Reply Agent execution sequence -->
```mermaid
sequenceDiagram
    participant START
    participant Retrieve
    participant Conversation
    participant Tools
    participant Summarize
    participant END

    START->>Retrieve: Similarity Search (Query)
    Retrieve->>Conversation: Inject Relevant Context
    Conversation->>Tools: Execute Custom DB Tools (if pending)
    Tools->>Conversation: Internal Tool Results
    Conversation->>Summarize: Should Summarize?
    Summarize-->>Conversation: Compact History
    Conversation->>END: Final Reply
```

## Step-by-Step Workflow
1. **Trigger**: Handled via the `/api/v1/chat-agent/reply` endpoint.
2. **Retrieval**: The `retrieve` node performs a similarity search on the MongoDB vector store using the user's latest message. (Targeting `REPLY_AGENT` collection).
3. **Context Injection**: Relevant document chunks (with sources and relevance scores) are injected into the system prompt.
4. **Tool Execution**: Allows accessing Custom Database Tools via `find_relevant_custom_tools` and `execute_custom_tool`. Raw output is stripped before sending to the user.
5. **Generation**: The `conversation` node generates a response using the augmented context and current conversation history.
6. **Memory Management**: If the session is long, the `should_continue` node triggers `summarize_conversation` to keep the context size manageable.

## Tool Access
- **`DocumentLoader`**: Interface for the MongoDB Atlas Vector Search.
- **`ToolsBuilder`**: Provides structured access to execute Custom Database Tools (`find_relevant_custom_tools`, `execute_custom_tool`).
- **`WhatsAppIntegration`**: Final delivery of the response.

## Technical Advantages
- **Grounded Responses**: Sofia is instructed to use the provided document context primarily, reducing hallucinations for domain-specific queries.
- **Score Thresholding**: Uses a `RAG_SCORE_THRESHOLD` (default 0.60) to ensure that only truly relevant information is used. If no documents pass the threshold, it gracefully falls back to general knowledge.
- **Traceability**: The agent includes source and chunk metadata in its internal context, allowing for future "citing sources" features.

## Output Contract
- **Type**: `AIMessage`.
- **Content**: Concise, clear, and context-aware text responses.

## Related Documents
- [Research Agent](research-agent.md) — Comparison with web-based research.
- [Chat Agent](chat-agent.md) — The main router.
- [Tech Stack](../stack/tech-stack.md) — Vector DB details.
