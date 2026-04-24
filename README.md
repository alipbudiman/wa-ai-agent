# WaNexus AI Agent

> **An intelligent, containerized WhatsApp AI agent bridging a Go-based WhatsApp runtime with a powerful Python-based reasoning engine.**

WaNexus is a state-of-the-art AI-powered WhatsApp bot system. It leverages a modern microservice architecture, separating the high-concurrency WhatsApp connection handling (written in Go) from the heavy, stateful LLM orchestration (written in Python using FastAPI and LangGraph/LangChain).

## 🚀 Key Features

* **Advanced AI Reasoning:** Integrated Large Language Models (LLMs) capabilities using LangChain and LangGraph for routing, tool usage, and memory-aware conversation flows.
* **Autonomous Agent Capabilities:** The agent is equipped with a versatile toolset allowing it to:
  * **Execute Actions Natively:** Run terminal commands, deploy background workers, and perform complex iterative scripting directly on the host system.
  * **Manage Files:** Read, manipulate, and compress files into archives seamlessly.
  * **Search the Web:** Perform real-time internet searches for factual lookups.
  * **Interact with Databases:** Safely execute localized queries.
  * **WhatsApp Automation:** Send rich texts, files, broadcast messages, and manage contacts autonomously via WhatsApp.
* **Dual-Core Architecture:** A robust Go-based WhatsApp client (using WhatsMeow) securely handles message streams and interfaces with a Python (FastAPI) Brain backend for cognitive tasks.
* **Automated Task Pipelines:** Consistent and repeatable workflows capable of handling complex multi-step chains (e.g., lookup → enrich → summarize → send).
* **Cron-Ready Scheduler:** Built-in support for reminders, periodic checks, and automated notifications.
* **Container-Native Integration:** Built seamlessly on `ubuntu:22.04` docker images for maximum system flexibility, isolation, and robustness.
* **Observability & Tracing:** Out-of-the-box support for deep debugging, tracing, and performance optimization.
* **Extensive API Support:** Multiple API handlers for WhatsApp operations including message sending, contact lookups, dynamic help menus, and targeted broadcasts.

## 🔗 Quick Links

Once the system is running, you can access the following services locally:
- **Manager UI:** [http://localhost:8001](http://localhost:8001/access)
- **WhatsApp Bot Server Docs (Go):** [http://localhost:8001/swagger/index.html](http://localhost:8001/swagger/index.html)
- **Brain API Docs (Python):** [http://localhost:8000/docs](http://localhost:8000/docs)
- **Ollama API:** [http://localhost:11434](http://localhost:11434)

---

## 🛠️ Docker Quick Start (Go + Brain + MongoDB + Redis + Ollama)

### Prerequisites
- [Docker Desktop](https://www.docker.com/products/docker-desktop/) installed and running.
- Execute all commands from the repository root (where `docker-compose.yml` resides).

Verify available services:
```bash
docker compose config --services
```
*Expected output: `mongo`, `redis`, `ollama`, `brain`, `wa-agent`*

### 1. Initial Setup Sequence
Before starting the full stack, you must prepare your LLM backend (Ollama, Anthropic, or OpenRouter). The recommended path is to begin with Ollama.

**Environment Configurations:**
Ensure your `.env` variables align with your deployment. Example configuration:
```env
MONGODB_URI=mongodb://mongo:27017
MONGODB_DB_ALIAS=wa_ai_agent
MONGODB_DB_NAME=wa_ai_agent
REDIS_HOST=redis
REDIS_PORT=6379
REDIS_TTL=2592000

GIN_SERVER_PORT=8001
GIN_SERVER_HOST=localhost
APP_MODE=release
```

### 2. Start Core Data Services (MongoDB + Redis)
```bash
docker compose up -d mongo redis
```

Verify database services are running:
```bash
docker compose ps mongo redis
docker compose logs --tail=50 redis
```

### 3. Start the Ollama Service
```bash
docker compose up -d ollama
```

Verify that Ollama is up and running:
```bash
docker compose ps ollama
docker compose logs --tail=50 ollama
```

### 4. Ollama Authentication (Required for Cloud Models)
Sign in to Ollama Cloud to access powerful external models. Open the linked provided in the terminal to complete your login:
```bash
docker compose exec ollama ollama signin
```
> **Why Ollama Cloud?** It integrates automatically with our AI agent SDKs without requiring independent Anthropic or OpenAI API keys out of the box, offering highly capable models for intensive coding and debugging tasks.

### 5. Provision Primary AI Models
Pull your desired model into the container. For first-time setups, we recommend the `minimax-m2.7:cloud` model:
```bash
docker compose exec ollama ollama pull minimax-m2.7:cloud
```
*Note: You can configure other providers like OpenRouter, Anthropic, or Nvidia API via the Manager UI, or by utilizing the `/select-providers` command within WhatsApp once the bot is active.*

Verify model installation:
```bash
docker compose exec ollama ollama list
```

### 6. Launch the Full Stack
Start the remaining application stack:
```bash
docker compose up -d --build
```
Ensure all services are running smoothly:
```bash
docker compose ps
```

### 7. Verify `npx skills` in Containers
Both `wa-agent` and `brain` images now install Node.js, the `skills` CLI, and run this build-time setup:
```bash
npx skills add https://github.com/vercel-labs/skills --skill find-skills -g -a universal -y
```

Use these commands to verify `npx` and the installed skill at runtime:
```bash
docker compose exec wa-agent sh -lc "node -v && npm -v && npx --yes skills --version && npx --yes skills list -g --json"
docker compose exec brain sh -lc "node -v && npm -v && npx --yes skills --version && npx --yes skills list -g --json"
```
Expected: CLI version is printed and `find-skills` appears in the global skills list.

To stop the entire stack cleanly:
```bash
docker compose down
```

---

## 🛟 Troubleshooting & Notes

- **"401 Unauthorized" from Brain Logs:** Re-run the `ollama signin` sequence inside the `ollama` container.
- **Service "ollama" is not running:** Run the following recovery sequence:
  ```bash
  docker compose up -d ollama
  docker compose ps ollama
  docker compose exec ollama ollama pull minimax-m2.7:cloud
  ```
- **Data Persistence:**
  - Authentication data is persisted within the `ollama_data` Docker volume.
  - MongoDB, Redis data, and WA Sessions are similarly volume-backed.
  - Shared playground files are kept in `playground_data` (mapped to `/app/playground`).
- **Resetting the Environment:**
  Running `docker compose down` will stop containers but retain data. If you intend to completely wipe the system state (requiring a re-login and model re-download), use `docker compose down -v`.

---

## 🎖️ Acknowledgments

Special thanks to the amazing technologies powering this project:
- **[LangChain](https://www.langchain.com/)** - Orchestrating LLMs and creating agentic workflows.
- **[Tavily](https://tavily.com/)** - AI research and search tooling.
- **[WhatsMeow](https://whatsmeow.dev/)** - High-performance Go library for the WhatsApp API.
- **[Ollama](https://ollama.com/)** - Seamless local and cloud LLM hosting with powerful models.
- **[MongoDB](https://www.mongodb.com/)** - Flexible NoSQL database for configuration and state management.
- **[FastAPI](https://fastapi.tiangolo.com/)** - High-performance Python web framework for the Brain API.
- **[Docker](https://www.docker.com/)** - Containerization platform enabling seamless deployment and isolation.
- **[Ubuntu](https://ubuntu.com/) 22.04** - Stable and versatile base image for all services.
- **[Go](https://golang.org/)** - Efficient and concurrent runtime for the WhatsApp client.
- **[Python](https://www.python.org/)** - Powerful language for AI orchestration and reasoning.
- **[Pydantic](https://pydantic.dev/)** - Data validation and settings management for Python models.
- **[OpenRouter](https://openrouter.ai/)** - Flexible API gateway for accessing multiple LLM providers.
- **[Anthropic](https://www.anthropic.com/)** - Cutting-edge AI research lab providing powerful models.
- **[NVIDIA AI](https://www.nvidia.com/en-us/ai/)** - Industry-leading AI models and infrastructure.
- **[GitHub Copilot](https://copilot.github.com/)** - AI pair programming assistant that inspired many of the agent's capabilities.
- **[Visual Studio Code](https://code.visualstudio.com/)** - The editor where most of the development magic happens.
- **The Open Source Community** - For all the libraries, tools, and resources that made this project possible.
- LangChain Academy and the broader AI development community for their invaluable resources and support.
- for my parent, Lecturer, family and friends who have been supportive of my learning and growth in AI and software development.
- who invented ai and programming languages in the first place, enabling all of us to build amazing things on their shoulders.
