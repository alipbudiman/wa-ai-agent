

from backend.domain.aidata import AIDataStatus
from config.config import Config


class CheckAIData:
    def __init__(self):
        self.data = Config()

    def execute(self) -> AIDataStatus:
        return AIDataStatus(
            list_ai_providers=self.data.list_providers
        )