from fastapi import FastAPI

from backend.application.health import CheckHealthUseCase
from backend.application.ai.agents import ChatAgentUseCase
from backend.application.report import ReportUseCase
from backend.application.aidata import CheckAIData
from backend.application.ai.code import AgentCodeUsecase
from backend.interface.api.router import create_ai_data_router, create_health_router
from backend.interface.api.router import create_langsmith_getlist_router
from backend.interface.api.router import create_langsmith_select_run_router
from backend.interface.api.router import create_chat_agent_router
from backend.interface.api.router import create_chat_agent_code_router
from backend.interface.api.router import create_chat_agent_research_router
from backend.application.ai.reaserch import ResearchAgentUsecase


def create_app() -> FastAPI:

    app = FastAPI(
        title="Brain AI Agent API",
        description="FastAPI backend sebagai otak AI Agent."
                    " WA bot bertindak sebagai kurir pesan.",
        version="0.1.0",
    )

    app.include_router(create_health_router(CheckHealthUseCase()), prefix="/api/v1")
    app.include_router(create_ai_data_router(CheckAIData()), prefix="/api/v1")
    app.include_router(create_chat_agent_router(ChatAgentUseCase()), prefix="/api/v1")
    app.include_router(create_chat_agent_code_router(AgentCodeUsecase()), prefix="/api/v1")
    app.include_router(create_chat_agent_research_router(ResearchAgentUsecase()), prefix="/api/v1")
    app.include_router(create_langsmith_getlist_router(ReportUseCase()), prefix="/api/v1")
    app.include_router(create_langsmith_select_run_router(ReportUseCase()), prefix="/api/v1")


    return app
