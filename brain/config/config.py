import os
from pathlib import Path
from dotenv import load_dotenv
from pydantic import BaseModel

from backend.infrastructure.database.database import MongoDB
from config.ai_models import AIModelConfigManager

BASE_DIR = Path(__file__).resolve().parents[2]
ENV_FILE = BASE_DIR / '.env'

# Load env from repository root file, regardless of the current working directory.
if ENV_FILE.exists():
    load_dotenv(dotenv_path=ENV_FILE)
else:
    load_dotenv()

class ProviderConfig(BaseModel):
    config_name: str = "providers"  # MongoDB document identifier
    providers: str  # Comma-separated string of enabled providers, e.g. "ollama,openrouter"    

class TavilyConfig(BaseModel):
    config_name: str = "tavily"  # MongoDB document identifier
    api_key: str

class LangSmithConfig(BaseModel):
    config_name: str = "langsmith"  # MongoDB document identifier
    api_key: str
    project_name: str

class GinServerConfig(BaseModel):
    config_name: str = "gin_server"  # MongoDB document identifier
    port: str = '8001'
    host: str = 'localhost'

class Config(AIModelConfigManager):
    def __init__(self):
        super().__init__()
        self.list_providers = ["ollama", "openrouter", "nvidia" ,"anthropic"]  # Supported AI model providers
        self.__init_all()
        
    def __init_all(self):
        """Eagerly initialize all configurations to ensure they exist in the database with default values if not already set. This simplifies access patterns for the rest of the application, as it can assume these configurations are always present."""
        self.get_provider_config()
        self.get_tavily_config()
        self.get_langsmith_config()
        self.get_gin_server_config()
        self.get_openrouter_config()
        self.get_ollama_config()
        self.get_nvidia_config()
        self.get_anthropic_config()
    
    def get_provider_config(self):
        """Fetch provider configuration from MongoDB, with fallback to default if not present. The providers field is a comma-separated string of enabled providers."""
        config = self.collection.find_one({"config_name": "providers"})
        if config is None:
            config = ProviderConfig(
                config_name="providers",
                providers="ollama,openrouter"  # default enabled providers
            ).model_dump()
            self.collection.insert_one(config)
        return ProviderConfig(
            config_name=config.get("config_name", "providers"),
            providers=config.get("providers", "ollama,openrouter")  # default to ollama and openrouter if not set
        )

    def get_tavily_config(self) -> TavilyConfig:
        """Fetch Tavily configuration from MongoDB, with fallback to default if not present."""
        config = self.collection.find_one({"config_name": "tavily"})
        if config is None:
            config = TavilyConfig(
                config_name="tavily",
                api_key=''
            ).model_dump()
            self.collection.insert_one(config)
        return TavilyConfig(
            config_name=config.get("config_name", "tavily"),
            api_key=config.get("api_key", "")
        )
    
    def get_langsmith_config(self) -> LangSmithConfig:
        """Fetch LangSmith configuration from MongoDB, with fallback to default if not present."""
        config = self.collection.find_one({"config_name": "langsmith"})
        if config is None:
            config = LangSmithConfig(
                config_name="langsmith",
                api_key='',
                project_name='wa_bot_ai_trace'
            ).model_dump()
            self.collection.insert_one(config)
        return LangSmithConfig(
            config_name=config.get("config_name", "langsmith"),
            api_key=config.get("api_key", ""),
            project_name=config.get("project_name", "wa_bot_ai_trace")
        )
    
    def get_gin_server_config(self):
        """Fetch Gin server configuration from MongoDB, with fallback to default if not present."""
        env_host = (os.getenv("GIN_SERVER_HOST") or "").strip()
        env_port = (os.getenv("GIN_SERVER_PORT") or "").strip()
        default_host = env_host or "localhost"
        default_port = env_port or "8001"

        config = self.collection.find_one({"config_name": "gin_server"})
        if config is None:
            config = GinServerConfig(
                config_name="gin_server",
                port=default_port,
                host=default_host
            ).model_dump()
            self.collection.insert_one(config)

        return GinServerConfig(
            config_name=config.get("config_name", "gin_server"),
            port=(str(config.get("port", "")).strip() or default_port),
            host=(str(config.get("host", "")).strip() or default_host)
        )
    

    # Other configurations can be added here as needed