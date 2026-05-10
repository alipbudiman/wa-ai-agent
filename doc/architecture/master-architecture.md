---
title: "Architecture Master"
category: "architecture"
last_analyzed: "2026-05-03"
---

# Architecture Master

## Overview
WaNexus is built on a **Dual-Core Microservice Architecture**, separating high-concurrency communication handling (Go) from complex cognitive reasoning (Python). This separation allows for maximum throughput in message processing while enabling heavy-duty LLM orchestration and tool execution in an isolated reasoning environment.

---

## 🏗️ System Overview

### High-Level Architecture
<!-- Full component map of the WaNexus system -->
```mermaid
graph TD
    User([WhatsApp User]) <--> WA[WhatsApp Cloud/Web]
    WA <--> GoCore[Go Runtime - WhatsMeow]
    
    subgraph "Go Runtime (Service A)"
        GoCore --> Dispatcher[Event Dispatcher]
        Dispatcher --> Router[HTTP/Internal Router]
        Router --> Manager[UI/Manager]
    end
    
    Router <--> BrainAPI[Python Brain - FastAPI]
    
    subgraph "Python Brain (Service B)"
        BrainAPI --> LangGraph[LangGraph Orchestrator]
        LangGraph --> RouterAgent[Router Agent]
        RouterAgent --> CodeAgent[Code Agent]
        RouterAgent --> ResearchAgent[Research Agent]
        RouterAgent --> ReplyAgent[Reply Agent]
    end
    
    subgraph "Infrastructure"
        LangGraph <--> MongoDB[(MongoDB Atlas)]
        LangGraph <--> Redis[(Redis Cache)]
        LangGraph <--> VectorDB[(Vector Store)]
        LangGraph <--> Ollama[Ollama / External LLM]
    end
    
    LangGraph --> Tools[Toolbox: Bash, FileX, WebSearch, ImageGen]
```

### Core Components
1. **Go Runtime (The Communicator)**: Handles persistent connection to WhatsApp, session management, and message ingestion.
2. **Python Brain (The Reasoning Engine)**: Handles intent routing, tool orchestration, and autonomous execution via LangGraph.
3. **Data & Infrastructure**: MongoDB for state, Redis for cache, Vector Store for RAG, and Ollama for local LLMs.

---

## 🔄 Data Flow

### Message Lifecycle
<!-- Message lifecycle sequence diagram -->
```mermaid
sequenceDiagram
    participant User as WhatsApp User
    participant Go as Go Runtime (WhatsMeow)
    participant Brain as Python Brain (FastAPI)
    participant LLM as LLM (Ollama/Anthropic)
    participant DB as MongoDB / VectorDB
    participant Tools as Tool System

    User->>Go: Sends Message
    Go->>Go: Parse & Validate
    Go->>Brain: POST /api/v1/chat-agent
    
    rect rgb(240, 240, 240)
        Note over Brain, Tools: Brain Orchestration
        Brain->>DB: Load Checkpoint (History)
        Brain->>LLM: Identify Intent (Router)
        
        alt Tool Needed
            LLM-->>Brain: Request Tool Call
            Brain->>Tools: Execute (Bash/Search/etc.)
            Tools-->>Brain: Tool Result
            Brain->>LLM: Refine Answer
        end
        
        alt RAG Needed
            Brain->>DB: Similarity Search
            DB-->>Brain: Relevant Context
            Brain->>LLM: Augment Context
        end
        
        LLM-->>Brain: Final Response
        Brain->>DB: Save Checkpoint
    end

    Brain-->>Go: JSON Response
    Go-->>User: Send WhatsApp Message
```

### Step-by-Step Lifecycle
1. **Ingestion**: WhatsApp message captured by Go client.
2. **Forwarding**: Metadata and content forwarded to Brain API.
3. **Processing**: LangGraph orchestrates history loading, intent routing, and tool usage.
4. **Completion**: Response generated and state persisted in MongoDB.
5. **Delivery**: Response returned to Go and sent via WhatsApp.

---
## Related Documents
- [Brain Master](../brain/master-brain.md)
- [Agents Master](../agents/master-agents.md)
