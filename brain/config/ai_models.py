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
                model='minimax-m2.7:cloud'
            ).model_dump()
            self.collection.insert_one(config)
        return OllamaConfig(
            config_name=config.get("config_name", "ollama"),
            base_url=config.get("base_url", "http://localhost:11434"),
            model=config.get("model", "minimax-m2.7:cloud")
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