# Project Guidelines

## Code Style
- Keep changes minimal and scoped to the requested task.
- Follow existing style in each language area instead of reformatting unrelated code.
- Prefer explicit error handling in Go (`if err != nil`) and clear return paths.
- For Python service logs, use logging (Uvicorn/FastAPI logger) instead of `print`.

## Architecture
- This repo contains two runtimes:
  - Go WhatsApp bot at repo root (`main.go`, `internal/**`).
  - Python Brain API under `brain/`.
- Preserve boundaries:
  - Go side: transport/event handling and orchestration.
  - Python side: AI/tool workflow and HTTP API (`/api/v1/chat-agent`).
- Python structure follows clean architecture in `brain/backend/`:
  - `domain/`, `application/`, `interface/api/`, `bootstrap/`.
- Go side uses ports/adapters pattern:
  - contracts in `internal/domain/ports/`, implementations in `internal/infrastructure/`.

## Build and Test
- Go compile/test check:
  - `go test ./...`
- Run Go bot:
  - `go run main.go`
- Run Python Brain API:
  - `cd brain && pip install -r requirements.txt`
  - `cd brain && uvicorn app:app --reload --host 0.0.0.0 --port 8000`
- Brain OpenAPI is available at:
  - `http://127.0.0.1:8000/openapi.json`

## Conventions
- Keep Go bot and Python brain loosely coupled via HTTP JSON contract:
  - Request: `{ "message": "..." }`
  - Response: `{ "status": "success", "response": "..." }`
- For Python files under `brain/` (FastAPI runtime), prioritize `async def` for endpoints and I/O-bound workflows so request handling stays non-blocking.
- For Go workflows outside `brain/`, prefer goroutine-based concurrency for long-running/background tasks, coordinated with `context.Context` and clear error handling.
- Apply async/concurrency intentionally; avoid unnecessary parallelism that can introduce race conditions or make debugging harder.
- If editing `ChatCompletion` on Go side, keep user-facing fallback responses for timeout/error paths.
- If editing LangGraph flow in Python:
  - Tool node should route back to LLM node for final answer generation.
  - Extract final non-empty AI content from messages state.
- Tavily output can vary (dict/list/JSON string). Normalize before iterating fields.

## Environment Notes
- Python config loads `.env` from repository root.
- Go runtime reads env via `config.Load()` and currently expects local brain endpoint (`BRAIN_API_BASE_URL`, default `http://127.0.0.1:8000`).
- Ensure Brain API is running before testing AI replies from the Go bot.

## Reference Docs
- Brain service overview and local run notes:
  - `brain/README.md`
