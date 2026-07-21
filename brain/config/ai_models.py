import os

from pydantic import BaseModel
from backend.infrastructure.database.database import MongoDB


class OpenRouterConfig(BaseModel):
    config_name: str = "openrouter"  # MongoDB document identifier
    api_key: str
    model: str
    
class OllamaConfig(BaseModel):
    config_name: str = "ollama"  # MongoDB document identifier
    base_url: str
    model: str

class NvidiaConfig(BaseModel):
    config_name: str = "nvidia"  # MongoDB document identifier
    api_key: str
    model: str

class AnthropicConfig(BaseModel):
    config_name: str = "anthropic"  # MongoDB document identifier
    base_url: str
    api_key: str
    model: str
    auth_token: str = ""

class GoogleConfig(BaseModel):
    config_name: str = "google"  # MongoDB document identifier
    api_key: str
    img_model: str
    model: str

class MinimaxConfig(BaseModel):
    config_name: str = "minimax"  # MongoDB document identifier
    api_key: str
    group_id: str
    img_model: str
    model: str

class OpenAIConfig(BaseModel):
    config_name: str = "openai"  # MongoDB document identifier
    api_key: str
    img_model: str
    img_quality: str = "medium"
    model: str

class AgentProvidersAndModel(BaseModel):
    agent_provider: str
    agent_model: str

class AgentConfigModels(BaseModel):
    config_name: str = "agent_config"
    agent_model: AgentProvidersAndModel = AgentProvidersAndModel(agent_provider="default", agent_model="default")
    agent_code_model: AgentProvidersAndModel = AgentProvidersAndModel(agent_provider="default", agent_model="default")
    research_model: AgentProvidersAndModel = AgentProvidersAndModel(agent_provider="default", agent_model="default")
    reply_model: AgentProvidersAndModel = AgentProvidersAndModel(agent_provider="default", agent_model="default")

class AIModelConfigManager:
    def __init__(self):
        self.db = MongoDB()
        self.collection = self.db.get_collection(MongoDB.MONGO_MANAGER_BOT_CONFIG_COLLECTION)
    
    def get_openrouter_config(self) -> OpenRouterConfig:
        config = self.collection.find_one({"config_name": "openrouter"})
        if config is None:
            config = OpenRouterConfig(
                config_name="openrouter",
                api_key='',
                model='openrouter/free'
            ).model_dump()
            self.collection.insert_one(config)
        return OpenRouterConfig(
            config_name=config.get("config_name", "openrouter"),
            api_key=config.get("api_key", ""),
            model=config.get("model", "openrouter/free")
        )
    
    def get_nvidia_config(self) -> NvidiaConfig:
        config = self.collection.find_one({"config_name": "nvidia"})
        if config is None:
            config = NvidiaConfig(
                config_name="nvidia",
                api_key='',
                model='nvidia/llama3-70b'
            ).model_dump()
            self.collection.insert_one(config)
        return NvidiaConfig(
            config_name=config.get("config_name", "nvidia"),
            api_key=config.get("api_key", ""),
            model=config.get("model", "nvidia/llama3-70b")
        )

    def get_ollama_config(self) -> OllamaConfig:
        config = self.collection.find_one({"config_name": "ollama"})
        if config is None:
            config = OllamaConfig(
                config_name="ollama",
                base_url='http://localhost:11434',
                model='gemma4:31b-cloud'
            ).model_dump()
            self.collection.insert_one(config)
        # OLLAMA_BASE_URL env var (set by docker-compose) takes priority over the
        # persisted DB value, so the deployment owns the service address, not the DB.
        # Falls back to the stored/default value for local dev (Ollama on localhost).
        env_base_url = (os.getenv("OLLAMA_BASE_URL") or "").strip()

        return OllamaConfig(
            config_name=config.get("config_name", "ollama"),
            base_url=env_base_url or config.get("base_url", "http://localhost:11434"),
            model=config.get("model", "gemma4:31b-cloud")
        )
    
    def get_anthropic_config(self) -> AnthropicConfig:
        config = self.collection.find_one({"config_name": "anthropic"})
        if config is None:
            config = AnthropicConfig(
                config_name="anthropic",
                base_url='',
                api_key='',
                model='',
                auth_token='',
            ).model_dump()
            self.collection.insert_one(config)
            
        return AnthropicConfig(
            config_name=config.get("config_name", "anthropic"),
            base_url=config.get("base_url", ""),
            api_key=config.get("api_key", ""),
            model=config.get("model", ""),
            auth_token=config.get("auth_token", ""),
        )
    
    def get_google_config(self) -> GoogleConfig:
        config = self.collection.find_one({"config_name": "google"})
        if config is None:
            config = GoogleConfig(
                config_name="google",
                api_key='',
                img_model='gemini-2.5-flash',
                model='gemini-2.5-flash'
            ).model_dump()
            self.collection.insert_one(config)
        return GoogleConfig(
            config_name=config.get("config_name", "google"),
            api_key=config.get("api_key", ""),
            img_model=config.get("img_model", "gemini-2.5-flash"),
            model=config.get("model", "gemini-2.5-flash"),
        )
    
    def get_minimax_config(self) -> MinimaxConfig:
        config = self.collection.find_one({"config_name": "minimax"})
        if config is None:
            config = MinimaxConfig(
                config_name="minimax",
                api_key='',
                group_id='',
                img_model='image-01',
                model='MiniMax-M2.7'
            ).model_dump()
            self.collection.insert_one(config)
        return MinimaxConfig(
            config_name=config.get("config_name", "minimax"),
            api_key=config.get("api_key", ""),
            group_id=config.get("group_id", ""),
            img_model=config.get("img_model", "image-01"),
            model=config.get("model", "MiniMax-M2.7"),
        )
    
    def get_openai_config(self) -> OpenAIConfig:
        config = self.collection.find_one({"config_name": "openai"})
        if config is None:
            config = OpenAIConfig(
                config_name="openai",
                api_key='',
                model='gpt-5',
                img_model='gpt-image-2-2026-04-21',
                img_quality='medium'
            ).model_dump()
            self.collection.insert_one(config)
        return OpenAIConfig(
            config_name=config.get("config_name", "openai"),
            api_key=config.get("api_key", ""),
            model=config.get("model", "gpt-5"),
            img_model=config.get("img_model", "gpt-image-2-2026-04-21"),
            img_quality=config.get("img_quality", "medium")
        )

    def get_agent_model_config(self) -> AgentConfigModels:
        config = self.collection.find_one({"config_name": "agent_config"})
        if config is None:
            config = AgentConfigModels(
                config_name="agent_config",
                agent_model=AgentProvidersAndModel(agent_provider="default", agent_model="default"),
                reply_model=AgentProvidersAndModel(agent_provider="default", agent_model="default"),
                agent_code_model=AgentProvidersAndModel(agent_provider="default", agent_model="default"),
                research_model=AgentProvidersAndModel(agent_provider="default", agent_model="default")
            ).model_dump()
            self.collection.insert_one(config)
        return AgentConfigModels(
            config_name=config.get("config_name", "agent_config"),
            agent_model=AgentProvidersAndModel(**config.get("agent_model", {"agent_provider": "default", "agent_model": "default"})),
            reply_model=AgentProvidersAndModel(**config.get("reply_model", {"agent_provider": "default", "agent_model": "default"})),
            agent_code_model=AgentProvidersAndModel(**config.get("agent_code_model", {"agent_provider": "default", "agent_model": "default"})),
            research_model=AgentProvidersAndModel(**config.get("research_model", {"agent_provider": "default", "agent_model": "default"}))
        )
