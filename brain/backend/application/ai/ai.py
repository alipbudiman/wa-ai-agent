from typing import Optional, Any
import logging

from langchain.messages import AIMessage
from langchain_nvidia_ai_endpoints import ChatNVIDIA
from langchain_google_genai import ChatGoogleGenerativeAI
from langchain_openai import ChatOpenAI
from langchain_openrouter import ChatOpenRouter
from langchain_ollama import ChatOllama
from langchain_anthropic import ChatAnthropic
from langgraph.graph import END, START, StateGraph, MessagesState

from backend.infrastructure.database.database import MongoDB
from config.ai_models import AgentProvidersAndModel
from config.config import Config



logger = logging.getLogger('uvicorn.error')


class AI:
    def __init__(self, model: Optional[str] = None):
        self.config = Config()
        self.model = model
        self.checkpointer = MongoDB.get_checkpointer()
        self.list_providers = self.config.list_providers

    def LLM(self, temperature: Optional[float] = None, reasoning: bool = True, providers: Optional[AgentProvidersAndModel] = None, structured_output: Optional[Any] = None) -> Any:
        if providers != None:
            model = providers.agent_model if providers.agent_model != "default" or providers.agent_model is None else self.model
            set_providers = providers.agent_provider
        else:
            model = self.model
            set_providers = providers if providers else self.config.get_provider_config().providers
            if not set_providers:
                raise ValueError("No providers configured. Please set the 'providers' field in the database. if you in bot, type `/select-providers` to set it.")
        
        if set_providers not in self.list_providers:
            raise ValueError(f"Unsupported provider configuration: {set_providers}. Please check the 'providers' field in the database. if you in bot, type `/select-providers` to set it.")
        
        match set_providers:
            
            case "openrouter":
                openrouter_config = self.config.get_openrouter_config()
                if not openrouter_config.api_key:
                    raise ValueError("OpenRouter API key is required. Please set it in the configuration. if you in bot, type `/openrouter-config` to set it.")
                
                
                
                llm = ChatOpenRouter(
                    api_key=openrouter_config.api_key,
                    model=model if model else openrouter_config.model,
                    temperature=temperature,
                    reasoning={"effort": "medium", "summary": "auto"} if reasoning else None
                )
                if structured_output:
                    llm = llm.with_structured_output(structured_output)
                return llm
            
            case "nvidia":
                nvidia_config = self.config.get_nvidia_config()
                if not nvidia_config.api_key:
                    raise ValueError("NVIDIA API key is required. Please set it in the configuration. if you in bot, type `/nvidia-config` to set it.")
                
                llm = ChatNVIDIA(
                    api_key=nvidia_config.api_key,
                    model=model if model else nvidia_config.model,
                    temperature=temperature,
                    model_kwargs={"thinking_mode": True} if reasoning else {},
                )
                if structured_output:
                    llm = llm.with_structured_output(structured_output)
                return llm
            
            case "ollama":
                ollama_config = self.config.get_ollama_config()
                llm = ChatOllama(
                    model=model if model else ollama_config.model,
                    base_url=ollama_config.base_url,
                    temperature=temperature,
                    reasoning=reasoning,
                )
                if structured_output:
                    llm = llm.with_structured_output(structured_output)
                return llm
            
            case "anthropic":
                anthropic_config = self.config.get_anthropic_config()
                if not anthropic_config.api_key:
                    raise ValueError("Anthropic API key is required. Please set it in the configuration. if you in bot, type `/anthropic-config` to set it.")
                                
                llm = ChatAnthropic(
                    api_key=anthropic_config.api_key,
                    model=model if model else anthropic_config.model or "claude-sonnet-4-5-20250929",
                    base_url=anthropic_config.base_url,
                    temperature=1 if reasoning else (temperature or 1),
                    effort="medium" if reasoning else None,     # xhigh, high, medium, low, minimal, none
                )
                if structured_output:
                    llm = llm.with_structured_output(structured_output)
                return llm
            
            case "google":
                google_config = self.config.get_google_config()
                if not google_config.api_key:
                    raise ValueError("Google API key is required. Please set it in the configuration. if you in bot, type `/google-config` to set it.")
                llm = ChatGoogleGenerativeAI(
                    api_key=google_config.api_key,
                    model=model if model else google_config.model,
                    temperature=temperature,
                    reasoning=reasoning,
                )
                if structured_output:
                    llm = llm.with_structured_output(structured_output)
                return llm
            
            case "minimax":
                minimax_config = self.config.get_minimax_config()
                if not minimax_config.api_key:
                    raise ValueError("Minimax API key is required. Please set it in the configuration. if you in bot, type `/minimax-config` to set it.")
                if not minimax_config.group_id:
                    raise ValueError("Minimax Group ID is required. Please set it in the configuration. if you in bot, type `/minimax-config group-id [group_id]` to set it.")

                minimax_kwargs = {
                    "api_key": minimax_config.api_key,
                    "model": model if model else minimax_config.model,
                    "base_url": "https://api.minimax.io/v1",
                    "extra_body": {"reasoning_split": True} if reasoning else None,
                }
                if temperature is not None and 0 < temperature <= 1:
                    minimax_kwargs["temperature"] = temperature
                elif temperature is not None:
                    logger.warning("Ignoring invalid MiniMax temperature=%s; valid range is 0 < temperature <= 1.", temperature)

                llm = ChatOpenAI(**minimax_kwargs)
                if structured_output:
                    llm = llm.with_structured_output(structured_output)
                return llm
            
            case "openai":
                openai_config = self.config.get_openai_config()
                if not openai_config.api_key:
                    raise ValueError("OpenAI API key is required. Please set it in the configuration. if you in bot, type `/openai-config` to set it.")
                llm = ChatOpenAI(
                    api_key=openai_config.api_key,
                    model=model if model else openai_config.model,
                    temperature=temperature,
                    reasoning={"effort": "medium", "summary": "auto"} if reasoning else None
                )
                if structured_output:
                    llm = llm.with_structured_output(structured_output)
                return llm
            case _:
                raise ValueError(f"Unsupported provider configuration: {providers}. Please check the 'providers' field in the database. if you in bot, type `/select-providers` to set it.")
