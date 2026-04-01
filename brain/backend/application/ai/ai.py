from langchain_openrouter import ChatOpenRouter

from config.config import Config

class AI:
    def __init__(self, api_key: str):
        self.api_key = api_key

    def LLM(self, model: str = Config.OPENROUTER_MODEL, temperature: float = 0.7):
        llm = ChatOpenRouter(
            api_key=self.api_key,
            model=model,
            temperature=temperature
        )
        return llm
