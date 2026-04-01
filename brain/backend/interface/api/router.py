from datetime import datetime

from fastapi import APIRouter
from pydantic import BaseModel

from backend.application.agents import ChatAgentUseCase
from backend.application.health import CheckHealthUseCase


class HealthResponse(BaseModel):
    status: str
    service: str
    timestamp: datetime

class ChatAgentRequest(BaseModel):
    message: str

class ChatAgentResponse(BaseModel):
    status: str
    response: str


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

def create_chat_agent_router(usecase: ChatAgentUseCase) -> APIRouter:
    router = APIRouter(prefix="/chat-agent", tags=["chat-agent"])

    @router.post("", response_model=ChatAgentResponse)
    def chat_agent(request: ChatAgentRequest):
        results = usecase.ChatAgent(messages=request.message)
        return ChatAgentResponse(
            status="success",
            response=results
        )

    return router
