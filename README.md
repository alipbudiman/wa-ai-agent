# WaNexus V2

Special Thanks to Amazing Technologies that made this project possible.
- LangChain: https://www.langchain.com/
- Tavily: https://tavily.com/
- WhatsMeow: https://whatsmeow.dev/


WhatsApp Bot server docs: http://localhost:8001/swagger/index.html
Brain API docs: http://localhost:8000/docs
Manager UI: http://localhost:8001/access

**System Highlights:**

* Integrated large language model (LLM) for reasoning and response generation.
* LangChain/LangGraph agent flow for routing, tool usage, and memory-aware execution.
* Multi-API handlers for WhatsApp operations (message sending, contact lookup, help menu, broadcast).
* Interconnected architecture between Go WhatsApp runtime and Python Brain API.
* Automated task handler pipelines for consistent and repeatable workflows.
* Cron-ready scheduler for reminders, periodic checks, and notifications.
* Multi-step workflow chaining (lookup → enrich → summarize → send).
* Containerized environment built natively on `ubuntu:22.04` for extended system flexibility.
* Observability and tracing for debugging and performance optimization.
## Docker (Go + Brain + MongoDB + Ollama)

Prerequisites:

- Docker Desktop is running.
- You run all commands from repository root (same folder as `docker-compose.yml`).

Check available services first:

```bash
docker compose config --services
```

Expected services:

- `mongo`
- `ollama`
- `brain`
- `wa-agent`

### First-time setup (recommended sequence)

1. Setup environment variables by copying the example template:

```bash
cp .env.example .env
```
*(Open `.env` and fill in your missing API Keys like Tavily & LangSmith)*

2. Start Ollama service first:

```bash
docker compose up -d ollama
```

2. Verify Ollama is up:

```bash
docker compose ps ollama
docker compose logs --tail=50 ollama
```

3. Sign in to Ollama Cloud (required for `*:cloud` models):

```bash
docker compose exec ollama ollama signin
```

Then open the `ollama.com/connect?...` URL shown in terminal and complete login in your browser.

4. Pull Ollama model inside container (for first-time setup, default model `minimax-m2.7:cloud`):

```bash
docker compose exec ollama ollama pull minimax-m2.7:cloud
```

5. Verify model exists:

```bash
docker compose exec ollama ollama list
```

6. Start full application stack:

```bash
docker compose up -d --build
```

7. Check all services:

```bash
docker compose ps
```

### Ollama credential notes

- If you see `401 unauthorized` from Brain logs, run `ollama signin` again in the `ollama` container.
- Authentication data is persisted in `ollama_data` volume (`/root/.ollama` in container).
- `docker compose down` keeps credentials.
- `docker compose down -v` removes credentials and requires sign in again.

### If you get: "service \"ollama\" is not running"

Run this recovery sequence:

```bash
docker compose up -d ollama
docker compose ps ollama
docker compose logs --tail=100 ollama
docker compose exec ollama ollama pull minimax-m2.7:cloud
```

If `docker compose ps ollama` still does not show `Up`, inspect full logs:

```bash
docker compose logs ollama
```

Note:

- First pull can be slow because Ollama image and model are large.
- Models are persisted in Docker volume `ollama_data`.
- Playground files are persisted in Docker volume `playground_data` and shared between `brain` and `wa-agent` at `/app/playground`.
- `docker compose down` stops containers but keeps volumes.
- Use `docker compose down -v` only if you intentionally want to remove all persisted data (Mongo, WA session, Ollama models).

Useful endpoints after startup:

- Brain API: http://localhost:8000/docs
- Manager UI: http://localhost:8001/access
- Ollama API: http://localhost:11434

Stop stack:

```bash
docker compose down
```
