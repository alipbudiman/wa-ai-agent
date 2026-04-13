# WaNexus: Brain Intelligence API

> **The cognitive engine and reasoning backend for the WaNexus AI Agent.**

This FastAPI-based Python backend acts as the core "brain" of the system. While the Go-based WhatsApp bot functions as the gateway and message dispatcher, this service handles all the heavy lifting: interpreting intents, orchestrating multi-step reasoning, executing specialized tools, and structuring intelligent responses.

## 🏗️ Clean Architecture Structure

The codebase is organized adhering to the principles of Clean Architecture, ensuring separation of concerns, high testability, and a robust microservice design.

- **`backend/domain/`**
  - Contains pure domain entities, models, and interfaces. This layer is independent of any external frameworks or libraries.
- **`backend/application/`**
  - Houses the core business logic and Use Cases (e.g., `CheckHealthUseCase`, agent routing handlers). It dictates the application's unique workflows.
- **`backend/interface/api/`**
  - The HTTP adapter layer. Contains FastAPI routers, controllers, and Pydantic response schemas to interpret and serialize external network requests.
- **`backend/bootstrap/`**
  - Responsible for Dependency Injection (DI) construction and core FastAPI app initialization, effectively bridging the application layers.
- **`app.py`**
  - The primary entry point for kicking off the FastAPI server.

## 🔌 API Reference & Usage

Once deployed, the Brain API exposes a comprehensive auto-generated Swagger UI.

- **Interactive API Documentation:** [http://localhost:8000/docs](http://localhost:8000/docs)

### Example Endpoint Usage

**Health Check endpoint:**
`GET /api/v1/checkhealth`

**Expected Response Payload:**
```json
{
  "status": "ok",
  "service": "brain-agent-api",
  "timestamp": "2026-04-01T12:34:56.789012+00:00"
}
```

## 💻 Local Development Setup

If you wish to run the Brain API independently of the Docker ecosystem (e.g., for local debugging or feature development), follow these instructions:

1. Create and activate a virtual environment (recommended).
2. Install the necessary Python dependencies:
   ```bash
   pip install -r requirements.txt
   ```
3. Start the FastAPI development server using Uvicorn:
   ```bash
   uvicorn app:app --reload --host 0.0.0.0 --port 8000
   ```

*Note: The Brain API requires a connection to a MongoDB instance as defined in your `.env` to operate fully.*
