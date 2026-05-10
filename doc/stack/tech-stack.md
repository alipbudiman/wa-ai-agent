---
title: "Tech Stack"
category: "stack"
last_analyzed: "2026-05-03"
---

# Tech Stack

## Overview
WaNexus leverages a modern, high-performance tech stack designed for concurrency, intelligence, and reliability. The system is split into two primary environments: a **Go Runtime** for connectivity and a **Python Brain** for AI reasoning.

## Core Languages & Runtimes
| Name | Version | Role | Why This Choice |
|------|---------|------|-----------------|
| **Go** | 1.25.0 | Connectivity & Proxy | Superior concurrency model (goroutines) and low memory footprint for persistent socket connections. |
| **Python** | 3.12+ | AI & Reasoning | Mature ecosystem for AI/ML (LangChain, LangGraph) and rapid development of complex logic. |
| **Node.js** | 20+ | Tooling (Skills) | Required for the `skills` CLI and certain sub-agent capabilities. |

## Backend Frameworks
| Name | Role | Why This Choice |
|------|------|-----------------|
| **Gin (Go)** | HTTP Router | High-performance, lightweight web framework for the management API. |
| **FastAPI (Python)** | Brain API | Modern, high-performance Python framework with native async support and Pydantic validation. |
| **LangGraph** | Orchestration | Enables cyclic, stateful multi-agent workflows with built-in checkpointing. |
| **LangChain** | LLM Abstraction | Unified interface for interacting with multiple LLM providers and tool management. |

## AI & LLM Infrastructure
| Name | Role | Why This Choice |
|------|------|-----------------|
| **Claude Agent SDK** | Engineering Worker | Powering the Code Agent with iterative planning and execution capabilities. |
| **Ollama** | Local LLM Hosting | Provides a private, local alternative to cloud-based LLMs, ensuring data security. |
| **LangSmith** | Observability | Used for tracing, debugging, and evaluating agent performance. |
| **Tavily** | Search Engine | AI-optimized search engine for the Research Agent. |

## Data & Persistence
| Name | Role | Why This Choice |
|------|------|-----------------|
| **MongoDB** | Primary Database | Flexible NoSQL schema for storing complex conversation states and configurations. |
| **Atlas Vector Search**| Vector Store | High-performance vector search integrated directly into the database for RAG. |
| **Redis** | Caching | Low-latency storage for transient session data and state. |
| **SQLite** | WA Session Storage | Lightweight, file-based database used by WhatsMeow for WhatsApp session state. |

## Infrastructure & DevOps
| Name | Role | Why This Choice |
|------|------|-----------------|
| **Docker** | Containerization | Ensures consistent environments across development and production. |
| **Ubuntu 22.04** | Base OS | Stable, widely supported Linux distribution for container images. |
| **WhatsMeow** | WhatsApp Client | The leading Go library for interacting with the WhatsApp multidevice API. |

## Key Libraries (Python)
- **`pydantic`**: Data validation and settings management.
- **`httpx`**: Asynchronous HTTP client for API communication.
- **`uvicorn`**: High-performance ASGI server.
- **`wikipedia`**: Used by the Research Agent for broad context.

## Key Libraries (Go)
- **`swaggo`**: Automatic Swagger documentation generation.
- **`godotenv`**: Environment variable management.
- **`qrterminal`**: Generating QR codes for WhatsApp login in the terminal.

## Related Documents
- [System Overview](../architecture/system-overview.md) — How these technologies connect.
- [Brain Overview](../brain/overview.md) — Python module specifics.
- [Code Agent](../agents/code-agent.md) — SDK implementation.
