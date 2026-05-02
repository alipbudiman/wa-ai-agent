from typing import Optional, Any
import logging

from langchain.messages import AIMessage
from langchain_nvidia_ai_endpoints import ChatNVIDIA
from langchain_google_genai import ChatGoogleGenerativeAI
from langchain_openrouter import ChatOpenRouter
from langchain_ollama import ChatOllama
from langchain_anthropic import ChatAnthropic
from langgraph.graph import END, START, StateGraph, MessagesState

from backend.infrastructure.database.database import MongoDB
from config.ai_models import AgentProvidersAndModel
from config.config import Config

from langgraph.checkpoint.mongodb import MongoDBSaver


logger = logging.getLogger('uvicorn.error')


class AI:
    def __init__(self, model: Optional[str] = None):
        self.config = Config()
        self.model = model
        self.checkpointer = MongoDB.get_checkpointer()

    def LLM(self, temperature: Optional[float] = None, reasoning: bool = True, providers: Optional[AgentProvidersAndModel] = None, structured_output: Optional[Any] = None) -> Any:
        if providers != None:
            model = providers.agent_model if providers.agent_model != "default" or providers.agent_model is None else self.model
            set_providers = providers.agent_provider
        else:
            model = self.model
            set_providers = providers if providers else self.config.get_provider_config().providers
            if not set_providers:
                raise ValueError("No providers configured. Please set the 'providers' field in the database. if you in bot, type `/select-providers` to set it.")
        
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
                llm = ChatGoogleGenerativeAI(
                    model=model if model else google_config.model,
                    temperature=temperature,
                    reasoning=reasoning,
                )
                if structured_output:
                    llm = llm.with_structured_output(structured_output)
                return llm
            case _:
                raise ValueError(f"Unsupported provider configuration: {providers}. Please check the 'providers' field in the database. if you in bot, type `/select-providers` to set it.")
    
    def InjectAIConversation(self, number: str, messages: str) -> bool:
        try:
            normalized_thread_id = str(number or "").strip()
            if not normalized_thread_id:
                return False

            if "@" not in normalized_thread_id:
                normalized_thread_id = f"{normalized_thread_id}@s.whatsapp.net"

            normalized_messages = str(messages or "").strip()
            if not normalized_messages:
                return False

            config = {"configurable": {"thread_id": normalized_thread_id}}

            builder = StateGraph(MessagesState)
            builder.add_node("inject_conversation", lambda _: {})
            builder.add_edge(START, "inject_conversation")
            builder.add_edge("inject_conversation", END)
            graph = builder.compile(checkpointer=self.checkpointer)

            graph.invoke(
                {
                    "messages": [
                        AIMessage(content=normalized_messages)
                    ]
                },
                config=config,
            )
            return True
        except Exception as e:
            logger.error(f"Error injecting AI conversation: {e}")
            return False