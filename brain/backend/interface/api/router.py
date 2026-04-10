from datetime import datetime
import logging

from fastapi import APIRouter, BackgroundTasks, HTTPException
from pydantic import BaseModel

from backend.application.ai.agents import ChatAgentUseCase
from backend.application.health import CheckHealthUseCase
from backend.application.report import ReportUseCase
from backend.domain.report import LangSmithLogReport, LangSmithRunReport
from backend.application.ai.tools.models import LevelSystem

logger = logging.getLogger('uvicorn.error')


class HealthResponse(BaseModel):
    status: str
    service: str
    timestamp: datetime

class ChatAgentRequest(BaseModel):
    number: str
    message: str
    is_system: bool
    level: str = "INFO"

class ChatAgentResponse(BaseModel):
    status: str
    response: str


class ChatAgentProcessDiagramResponse(BaseModel):
    status: str
    mermaid: str


def create_health_router(usecase: CheckHealthUseCase) -> APIRouter:
    router = APIRouter(prefix="/checkhealth", tags=["health"])

    @router.get("", response_model=HealthResponse)
    def check_health() -> HealthResponse:
        result = usecase.execute()
        return HealthResponse(
            status=result.status,
            service=result.service,
            timestamp=result.timestamp,
        )

    return router

def create_langsmith_getlist_router(usecase: ReportUseCase) -> APIRouter:
    router = APIRouter(prefix="/langsmith", tags=["langsmith"])

    @router.get("/runs", response_model=list[LangSmithRunReport])
    def get_runs():
        return usecase.get_list()

    return router

def create_langsmith_select_run_router(usecase: ReportUseCase) -> APIRouter:
    router = APIRouter(prefix="/langsmith", tags=["langsmith"])

    @router.get("/runs/{run_id}", response_model=LangSmithLogReport)
    def select_run(run_id: str):
        return usecase.select_run(run_id)

    return router


def create_chat_agent_router(usecase: ChatAgentUseCase) -> APIRouter:
    router = APIRouter(prefix="/chat-agent", tags=["chat-agent"])

    @router.get("/process-diagram", response_model=ChatAgentProcessDiagramResponse)
    def get_process_diagram() -> ChatAgentProcessDiagramResponse:
        mermaid_source = usecase.get_process_diagram_mermaid().strip()
        if not mermaid_source:
            raise HTTPException(status_code=503, detail="process diagram is not available")

        return ChatAgentProcessDiagramResponse(
            status="success",
            mermaid=mermaid_source,
        )

    @router.post("", response_model=ChatAgentResponse)
    async def chat_agent(request: ChatAgentRequest, background_tasks: BackgroundTasks):
        try:
            level = LevelSystem[request.level.upper()]
        except KeyError:
            level = LevelSystem.INFO
            logger.info(f"Invalid log level '{request.level}' received. Defaulting to INFO.")
        background_tasks.add_task(
            usecase.ChatAgent,
            messages=request.message,
            number=request.number,
            is_system=request.is_system,
            level=level,
        )
        return ChatAgentResponse(
            status="success",
            response="success to call brain"
        )

    return router
