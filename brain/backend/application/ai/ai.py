from langchain_openrouter import ChatOpenRouter
from langchain_ollama import ChatOllama

from config.config import Config

class AI:
    def __init__(self):
        self.openrouter_api_key=Config.OPENROUTER_API_KEY

    def LLM(self, temperature: float = 0.7, reasoning: bool = True):
        if self.openrouter_api_key:
            # OpenRouter: reasoning menerima dict untuk konfigurasi effort & summary
            reasoning_config = {
                "effort": "high",     # xhigh, high, medium, low, minimal, none
                "summary": "auto"     # auto, concise, detailed
            } if reasoning else None

            llm = ChatOpenRouter(
                api_key=self.openrouter_api_key,
                model=Config.OPENROUTER_MODEL,
                temperature=temperature,
                reasoning=reasoning_config
            )
            return llm
        else:
            # Ollama: reasoning menerima bool (True/False/None)
            # True  = thinking content disimpan di additional_kwargs["reasoning_content"]
            # False = reasoning dimatikan
            # None  = default behavior, <think> tags muncul di content
            llm = ChatOllama(
                model=Config.OLLAMA_MODEL,
                temperature=temperature,
                reasoning=reasoning,
            )
            return llm
