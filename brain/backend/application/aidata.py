

from backend.domain.aidata import AIDataInjectionResponse, AIDataStatus
from backend.application.ai.ai import AI
from config.config import Config


class CheckAIData:
    def __init__(self):
        self.data = Config()
        self.llm = AI()

    def execute(self) -> AIDataStatus:
        return AIDataStatus(
            list_ai_providers=self.data.list_providers
        )
    
    def injectAIConversation(self, number: str, messages: str) -> AIDataInjectionResponse:
        success = self.llm.InjectAIConversation(number=number, messages=messages)
        return AIDataInjectionResponse(success=success)