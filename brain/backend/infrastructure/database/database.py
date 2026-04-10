import os

from pymongo import MongoClient

class MongoDB:
    _instance: MongoClient = None
    # database configuration
    MONGO_URI = os.getenv('MONGODB_URI', 'mongodb://localhost:27017/')
    MONGO_DB_NAME = os.getenv('MONGODB_DB_NAME', 'wa_ai_agent_db')
    MONGO_TEMPLATE_MSG_COLLECTION = 'tmp_messages'
    MONGO_LOG_SYSTEM_COLLECTION = 'log_system'
    
    @classmethod
    def get_client(cls) -> MongoClient:
        if cls._instance is None:
            cls._instance = MongoClient(cls.MONGO_URI)
        return cls._instance
    
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