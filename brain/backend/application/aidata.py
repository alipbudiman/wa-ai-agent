

import logging
from backend.domain.aidata import AIDataInjectionResponse, AIDataStatus
from backend.application.ai.reply import ReplyAgentUsecase
from config.config import Config

logger = logging.getLogger('uvicorn.error')

class CheckAIData:
    def __init__(self):
        self.data = Config()
        self.reply_agent = ReplyAgentUsecase()

    def execute(self) -> AIDataStatus:
        return AIDataStatus(
            list_ai_providers=self.data.list_providers
        )
    
    async def injectAIConversation(self, number: str, messages: str) -> AIDataInjectionResponse:
        try:
            await self.reply_agent.ReplyAgent(
                messages=messages,
                number=number,
                is_system=True
            )
            return AIDataInjectionResponse(success=True)
        except Exception as e:
            logger.error(f"Error injecting AI conversation: {e}")
            return AIDataInjectionResponse(success=False)