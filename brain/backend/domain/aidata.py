

from openai import BaseModel


class AIDataStatus(BaseModel):
    list_ai_providers: list[str]