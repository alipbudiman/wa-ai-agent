from langgraph.checkpoint.mongodb import MongoDBSaver
import os
import redis
from pymongo import MongoClient
from datetime import datetime


class MongoDB:
    _instance: MongoClient = None
    _checkpointer: MongoDBSaver = None
    # database configuration
    MONGO_URI = os.getenv('MONGODB_URI', 'mongodb://localhost:27017/')
    MONGO_DB_NAME = os.getenv('MONGODB_DB_NAME', 'wa_ai_agent_db')
    MONGO_MANAGER_BOT_CONFIG_COLLECTION = 'manager_bot_config'
    MONGO_TEMPLATE_MSG_COLLECTION = 'tmp_messages'
    MONGO_LOG_SYSTEM_COLLECTION = 'log_system'
    MONGO_USER_INPUT_HISTORY_COLLECTION = 'input_history'
    TOOLS_BUILDER_COLLECTION = "tools_builder"
    
    @classmethod
    def get_client(cls) -> MongoClient:
        if cls._instance is None:
            cls._instance = MongoClient(cls.MONGO_URI)
        return cls._instance
    
    @classmethod
    def get_checkpointer(cls) -> MongoDBSaver:
        if cls._checkpointer is None:
            cls._checkpointer = MongoDBSaver(cls.get_client(), db_name=cls.MONGO_DB_NAME, collection_name="agent_checkpointer")
        return cls._checkpointer
    
    @classmethod
    def close(cls):
        if cls._instance is not None:
            cls._instance.close()
            cls._instance = None
    
    @classmethod
    def get_db(cls, db_name: str):
        return cls.get_client()[db_name]
    
    @classmethod
    def get_collection(cls, collection_name: str):
        return cls.get_db(cls.MONGO_DB_NAME)[collection_name]

class RedisDB:
    _instance: redis.Redis = None
    REDIS_HOST = os.getenv('REDIS_HOST', 'localhost')
    REDIS_PORT = int(os.getenv('REDIS_PORT', 6379))
    REDIS_TTL = int(os.getenv('REDIS_TTL', (3600 * 24) * 30))  # Default 30 hari
    
    @classmethod
    def get_client(cls) -> redis.Redis:
        if cls._instance is None:
            cls._instance = redis.Redis(host=cls.REDIS_HOST, port=cls.REDIS_PORT, db=0)
        return cls._instance