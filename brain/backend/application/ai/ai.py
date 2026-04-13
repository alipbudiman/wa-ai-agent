from typing import Optional

from langchain_nvidia_ai_endpoints import ChatNVIDIA
from langchain_openrouter import ChatOpenRouter
from langchain_ollama import ChatOllama
from langchain_anthropic import ChatAnthropic

from config.config import Config

class AI:
    def __init__(self, model: Optional[str] = None):
        self.config = Config()
        self.model = model

    def LLM(self, temperature: Optional[float] = None, reasoning: bool = True):
        providers = self.config.get_provider_config().providers
        if not providers:
            raise ValueError("No providers configured. Please set the 'providers' field in the database. if you in bot, type `/select-providers` to set it.")
        
        match providers:
        
            case "openrouter":
                openrouter_config = self.config.get_openrouter_config()
                if not openrouter_config.api_key:
                    raise ValueError("OpenRouter API key is required. Please set it in the configuration. if you in bot, type `/openrouter-config` to set it.")
                
                llm = ChatOpenRouter(
                    api_key=openrouter_config.api_key,
                    model=self.model or openrouter_config.model,
                    temperature=temperature,
                    reasoning={"effort": "medium", "summary": "auto"} if reasoning else None
                )
                return llm
            
            case "nvidia":
                nvidia_config = self.config.get_nvidia_config()
                if not nvidia_config.api_key:
                    raise ValueError("NVIDIA API key is required. Please set it in the configuration. if you in bot, type `/nvidia-config` to set it.")
                
                llm = ChatNVIDIA(
                    api_key=nvidia_config.api_key,
                    model=self.model or nvidia_config.model,
                    temperature=temperature,
                    model_kwargs={"thinking_mode": True} if reasoning else {},
                )
                return llm
            
            case "ollama":
                ollama_config = self.config.get_ollama_config()
                llm = ChatOllama(
                    model=self.model or ollama_config.model,
                    base_url=ollama_config.base_url,
                    temperature=temperature,
                    reasoning=reasoning,
                )
                return llm
            
            case "anthropic":
                anthropic_config = self.config.get_anthropic_config()
                if not anthropic_config.api_key:
                    raise ValueError("Anthropic API key is required. Please set it in the configuration. if you in bot, type `/anthropic-config` to set it.")
                                
                llm = ChatAnthropic(
                    api_key=anthropic_config.api_key,
                    model=self.model or anthropic_config.model or "claude-sonnet-4-5-20250929",
                    base_url=anthropic_config.base_url,
                    temperature=1 if reasoning else (temperature or 1),
                    effort="medium" if reasoning else None,     # xhigh, high, medium, low, minimal, none
                )
                return llm
            case _:
                raise ValueError(f"Unsupported provider configuration: {providers}. Please check the 'providers' field in the database. if you in bot, type `/select-providers` to set it.")