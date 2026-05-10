# PATH_READ — Understood Source Files

| # | File Path | What I Understand From It |
|---|-----------|--------------------------|
| 1 | `README.md` | Core project overview, microservice architecture (Go/Python), features, and setup instructions. |
| 2 | `brain/README.md` | Brain module overview, Clean Architecture structure, API endpoints, and development setup. |
| 3 | `main.go` | Main entry point for the Go service, handles WhatsApp connection, HTTP server, and orchestration. |
| 4 | `brain/app.py` | Entry point for the Python Brain API, initializes FastAPI and tools. |
| 5 | `brain/backend/bootstrap/app_factory.py` | FastAPI application factory, defines routers for health, agents, and reporting. |
| 6 | `brain/backend/application/ai/agents.py` | Main ChatAgent (Router) logic using LangGraph, handles tool orchestration and human-in-the-loop. |
| 7 | `brain/backend/application/ai/code.py` | AgentCode logic using Claude Agent SDK for complex engineering tasks and multi-step reasoning. |
| 8 | `brain/backend/application/ai/reaserch.py` | ResearchAgent logic using parallel web/Wikipedia searches and data synthesis. |
| 9 | `brain/backend/application/ai/reply.py` | ReplyAgent (Sofia) logic using RAG (Vector DB) for context-aware responses. |
