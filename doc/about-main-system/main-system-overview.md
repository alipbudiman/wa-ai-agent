---
title: "Main System Overview"
category: "architecture"
last_analyzed: "2026-05-03"
---

# WaNexus: The Intelligence Cortex

## 🌟 System Essence
**WaNexus** is not just a chatbot; it is a **fully autonomous AI agentic system** designed to bridge the gap between instant messaging (WhatsApp) and complex system-level engineering and research. It functions as a "Digital Cortex" that can perceive intent, plan multi-step operations, and execute them directly on a host system with high precision.

## 🏛️ The Dual-Core Pillar
The system's robustness stems from its unique split-architecture:

### 1. The Gateway Core (Go)
Built for **high-throughput and persistence**.
- **Role**: Handles the socket connection to WhatsApp, manages authentication, and serves as the HTTP gateway.
- **Strength**: Go's concurrency model ensures that the bot stays "alive" and responsive to thousands of messages while the AI is busy "thinking."

### 2. The Reasoning Core (Python)
Built for **intelligence and cognitive depth**.
- **Role**: Orchestrates Large Language Models (LLMs) via LangGraph.
- **Strength**: Leverages the Python AI ecosystem to perform RAG (Retrieval-Augmented Generation), iterative coding loops, and parallel research.

## 🧠 The Intelligence Engine (Agents)
WaNexus operates through a specialized division of labor:

- **The Router (ChatAgent)**: The primary supervisor. It uses intent-classification to decide if a request is a simple chat, a technical task for the Code Agent, or a research query for the Research Agent.
- **The Engineer (Code Agent)**: Powered by the Claude Agent SDK. It can write code, debug scripts, manage terminal sessions, and explore repositories within a sandboxed playground.
- **The Analyst (Research Agent)**: A parallel-processing machine. It searches multiple queries across the web and Wikipedia simultaneously to synthesize a data-driven report.
- **The Specialist (Sofia/Reply Agent)**: A RAG-based agent that focuses on your local uploaded documents, providing grounded and accurate answers from your private knowledge base.

## 🛠️ System Capabilities & Autonomy
The system is equipped with a versatile "hands-on" toolbox:
- **Terminal Execution**: Safely runs shell commands and manages persistent terminal sessions (tmux).
- **File Management**: Full CRUD operations on the filesystem, including compression and directory manipulation.
- **Image Generation**: Dynamic generation of visual assets for web or app development tasks.
- **Web Research**: Real-time factual lookups via Tavily and Wikipedia.
- **Database Access**: Direct integration with MongoDB for state persistence and vector search.

## 🛡️ Reliability & Safety
- **State Persistence**: Every thought, tool result, and conversation turn is checkpointed in MongoDB. Even if the system crashes, it can resume exactly where it left off.
- **Human-in-the-Loop**: Potentially destructive actions are automatically detected and paused for user approval.
- **Self-Correction**: The agents are designed to see their own errors. If a terminal command fails, the agent reads the stderr and attempts a fix autonomously.

## 🚀 The Vision
WaNexus is designed to be the ultimate **AI Pair Programmer and Research Partner**. By combining the reliability of Go with the intelligence of Python and LangGraph, it creates a system that doesn't just talk, but **works**.

---

## Related Documents
- [System Architecture](../architecture/system-overview.md)
- [Agent Workflows](../agents/chat-agent.md)
- [Tech Stack](../stack/tech-stack.md)
