from fastapi import FastAPI

from backend.application.health import CheckHealthUseCase
from backend.application.ai.agents import ChatAgentUseCase
from backend.interface.api.router import create_health_router
from backend.interface.api.router import create_chat_agent_router


def create_app() -> FastAPI:
    app = FastAPI(
        title="Brain AI Agent API",
        description="FastAPI backend sebagai otak AI Agent."
                    " WA bot bertindak sebagai kurir pesan.",
        version="0.1.0",
    )

    health_usecase = CheckHealthUseCase()
    chat_agent_usecase = ChatAgentUseCase()
    app.include_router(create_health_router(health_usecase), prefix="/api/v1")
    app.include_router(create_chat_agent_router(chat_agent_usecase), prefix="/api/v1")

    return app
