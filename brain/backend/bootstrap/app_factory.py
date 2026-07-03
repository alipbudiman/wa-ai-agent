from backend.interface.api.router import create_escalation_router, create_scoring_agent
from fastapi import FastAPI

from backend.application.health import CheckHealthUseCase
from backend.application.ai.agents import ChatAgentUseCase
from backend.application.report import ReportUseCase
from backend.application.aidata import CheckAIData
from backend.application.ai.code import AgentCodeUsecase
from backend.application.ai.reaserch import ResearchAgentUsecase
from backend.application.ai.reply import ReplyAgentUsecase
from backend.application.ai.tools.catalog import Catalog
from backend.application.ai.tools.dbaccess import DBAcess
from backend.scoring.scoring import ScoringAgent
from backend.interface.api.router import create_admin_number_store_router, create_ai_data_router, create_catalog_store_router, create_health_router
from backend.interface.api.router import create_langsmith_getlist_router
from backend.interface.api.router import create_langsmith_select_run_router
from backend.interface.api.router import create_chat_agent_router
from backend.interface.api.router import create_chat_agent_code_router
from backend.interface.api.router import create_chat_agent_research_router
from backend.interface.api.router import create_chat_agent_reply_router
from backend.interface.api.router import create_vector_store_router
from backend.interface.api.router import create_escalation_router


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
    app.include_router(create_chat_agent_reply_router(ReplyAgentUsecase()), prefix="/api/v1")
    app.include_router(create_langsmith_getlist_router(ReportUseCase()), prefix="/api/v1")
    app.include_router(create_langsmith_select_run_router(ReportUseCase()), prefix="/api/v1")
    app.include_router(create_vector_store_router(), prefix="/api/v1")
    app.include_router(create_catalog_store_router(Catalog()), prefix="/api/v1")
    app.include_router(create_admin_number_store_router(DBAcess()), prefix="/api/v1")
    app.include_router(create_escalation_router(DBAcess()), prefix="/api/v1")
    app.include_router(create_scoring_agent(ScoringAgent()), prefix="/api/v1")
    


    return app
