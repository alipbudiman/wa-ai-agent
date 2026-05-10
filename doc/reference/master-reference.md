---
title: "Reference Master"
category: "reference"
last_analyzed: "2026-05-03"
---

# Reference Master

## Overview
This document serves as a technical reference for the WaNexus system, covering the underlying technology stack and a deep architectural analysis of the system's design and implementation.

---

## 🛠️ Tech Stack

### Languages & Runtimes
- **Go (1.25.0)**: Connectivity, message ingestion, and socket management.
- **Python (3.12+)**: Cognitive reasoning, agent orchestration, and LLM interfacing.
- **Node.js (20+)**: Supporting tools and CLI utilities (Skills).

### Frameworks & Libraries
- **Gin (Go)**: Management API and router.
- **FastAPI (Python)**: High-performance Brain API.
- **LangGraph / LangChain**: Stateful agentic workflows and LLM abstractions.
- **Claude Agent SDK**: Powering the specialized Code Agent.

### Data & Infrastructure
- **MongoDB & Atlas Vector Search**: Main persistence and RAG capabilities.
- **Redis**: Low-latency caching.
- **Docker**: Containerization and isolation.
- **WhatsMeow**: Core WhatsApp client library.

---

## 🔍 Architectural Analysis

### Design Insights
1. **Dual-Core Efficiency**: Splitting high-concurrency Go logic from reasoning-heavy Python logic ensures both responsiveness and depth.
2. **Stateful Loops**: LangGraph enables cyclic reasoning, allowing agents to recover from errors and refine plans iteratively.
3. **Human-in-the-Loop**: Integrated safety gates for sensitive tool execution balance autonomy with security.

### Technical Advantages
- **Tool Coercion**: Robust parsing of textual tool calls makes the system model-agnostic and resilient to API variations.
- **Dynamic Workspace Resolution**: Intelligent path mapping across Windows and Linux environments ensures Docker portability.

### Identified Gaps
- **Secret Management**: Opportunity to implement centralized secret masking at the SDK level.
- **Communication Latency**: Potential to move to message queues for high-volume background tasks.
- **Vector Tuning**: RAG thresholds are currently fixed and could be made dynamic based on embedding models.

---
## Related Documents
- [Architecture Master](../architecture/master-architecture.md)
- [Brain Master](../brain/master-brain.md)
- [Agents Master](../agents/master-agents.md)
