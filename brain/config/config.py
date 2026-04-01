import os
from pathlib import Path
from dotenv import load_dotenv

BASE_DIR = Path(__file__).resolve().parents[2]
ENV_FILE = BASE_DIR / '.env'

# Load env from repository root file, regardless of the current working directory.
if ENV_FILE.exists():
    load_dotenv(dotenv_path=ENV_FILE)
else:
    load_dotenv()

class Config:
    # OpenRouter API Key
    OPENROUTER_API_KEY = os.getenv('OPENROUTER_API_KEY')
    TAVILY_API_KEY = os.getenv('TAVILY_API_KEY')
    OPENROUTER_MODEL = os.getenv('OPENROUTER_MODEL', 'qwen/qwen3.6-plus-preview:free')

    # Other configurations can be added here as needed