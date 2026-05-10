---
title: "Research Agent"
category: "agent"
last_analyzed: "2026-05-03"
---

# Research Agent

## Overview
The **Research Agent** (implemented as `ResearchAgentUsecase`) is a specialized analyst designed to answer complex factual questions by gathering and synthesizing information from multiple web sources. It uses a **Parallel Fan-out/Fan-in** strategy to maximize speed and coverage.

## Workflow

<!-- Research Agent execution sequence -->
```mermaid
graph TD
    START --> Wiki[Search Wikipedia]
    START --> GenQuery[Generate Search Queries]
    
    GenQuery --> Web1[Web Search Q1]
    GenQuery --> Web2[Web Search Q2]
    GenQuery --> Web3[Web Search Q3]
    
    Web1 --> Barrier[Collect Results]
    Web2 --> Barrier
    Web3 --> Barrier
    
    Barrier --> Summarize[Summarize Web Data]
    Wiki --> Answer[Generate Final Answer]
    Summarize --> Answer
    
    Answer --> END
```

## Step-by-Step Workflow
1. **Trigger**: Handled via the `/api/v1/chat-agent/research` endpoint.
2. **Decomposition**: `generate_query_search_prompt` breaks the user's question into 3-5 distinct search queries.
3. **Parallel Execution**:
    - **Wikipedia**: Searches for broad contextual background.
    - **Web Search**: Dispatches multiple parallel `Send` requests to fetch real-time data for each query.
4. **Synthesis (Fan-in)**:
    - `collect_search_results` act as a barrier node, waiting for all parallel web searches to finish.
    - `summarize_search_results` condenses the raw web data into a structured research briefing.
5. **Final Generation**: `generate_answer` combines the Wikipedia context and the Web research summary to produce a data-driven, comprehensive response.

## Tool Access
- **`WebSearchTool`**: Performs the actual web and Wikipedia searches.
- **`TimeTool`**: Ensures search queries and answers are contextually relevant to the current time.
- **`WhatsAppIntegration`**: Final delivery of the research report to the user.

## Technical Advantages
- **High Throughput**: By using parallel `Send` nodes in LangGraph, the agent can perform 5+ web searches in the time it would take to do one.
- **Data-Driven**: The system is instructed to prioritize "Numbers, value ranges, percentages, and volumes" in its summaries.
- **Structured Synthesis**: The `SummarizeSearch` node uses a specific 5-point executive summary format to ensure consistency and professional-grade reporting.

## Output Contract
- **Type**: `AIMessage`.
- **Content**: A detailed report including an Executive Summary, Key Facts per Topic, Cross-source comparison, and Actionable conclusions.

## Related Documents
- [Chat Agent](chat-agent.md) — The main router.
- [Reply Agent](reply-agent.md) — For comparison with RAG-based research.
- [Orchestration](../brain/orchestration.md) — LangGraph parallel patterns.
