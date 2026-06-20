from langgraph.checkpoint.mongodb import MongoDBSaver
import os
import redis
from collections.abc import Sequence
from datetime import datetime, timezone
from typing import Any

from langchain_core.runnables import RunnableConfig
from langgraph.checkpoint.base import ChannelVersions, Checkpoint, CheckpointMetadata
from pymongo import MongoClient
from pymongo.errors import OperationFailure


class ThreadTTLCheckpointSaver(MongoDBSaver):
    """MongoDBSaver that expires a whole thread after its latest checkpoint/write."""

    def _latest_thread_activity(self, thread_id: str, checkpoint_ns: str) -> datetime | None:
        query = {
            "thread_id": thread_id,
            "checkpoint_ns": checkpoint_ns,
            "created_at": {"$exists": True},
        }
        projection = {"created_at": 1}
        sort = [("created_at", -1)]
        candidates = [
            self.checkpoint_collection.find_one(query, projection, sort=sort),
            self.writes_collection.find_one(query, projection, sort=sort),
        ]
        activity_values = [
            item["created_at"]
            for item in candidates
            if item and isinstance(item.get("created_at"), datetime)
        ]
        if activity_values:
            return max(activity_values)
        return None

    def _delete_thread_namespace(self, thread_id: str, checkpoint_ns: str) -> None:
        query = {"thread_id": thread_id, "checkpoint_ns": checkpoint_ns}
        self.checkpoint_collection.delete_many(query)
        self.writes_collection.delete_many(query)

    def _delete_if_thread_expired(self, thread_id: str, checkpoint_ns: str) -> bool:
        if not self.ttl:
            return False

        latest_activity = self._latest_thread_activity(thread_id, checkpoint_ns)
        if latest_activity is None:
            return False

        now = datetime.now(tz=timezone.utc)
        if latest_activity.tzinfo is None:
            latest_activity = latest_activity.replace(tzinfo=timezone.utc)
        if (now - latest_activity).total_seconds() <= self.ttl:
            return False

        self._delete_thread_namespace(thread_id, checkpoint_ns)
        return True

    def _touch_thread(self, thread_id: str, checkpoint_ns: str) -> None:
        if not self.ttl:
            return

        query = {"thread_id": thread_id, "checkpoint_ns": checkpoint_ns}
        update = {"$set": {"created_at": datetime.now(tz=timezone.utc)}}
        self.checkpoint_collection.update_many(query, update)
        self.writes_collection.update_many(query, update)

    def get_tuple(self, config: RunnableConfig):
        thread_id = config["configurable"]["thread_id"]
        checkpoint_ns = config["configurable"].get("checkpoint_ns", "")
        if self._delete_if_thread_expired(thread_id, checkpoint_ns):
            return None
        return super().get_tuple(config)

    def put(
        self,
        config: RunnableConfig,
        checkpoint: Checkpoint,
        metadata: CheckpointMetadata,
        new_versions: ChannelVersions,
    ) -> RunnableConfig:
        result = super().put(config, checkpoint, metadata, new_versions)
        self._touch_thread(
            config["configurable"]["thread_id"],
            config["configurable"]["checkpoint_ns"],
        )
        return result

    def put_writes(
        self,
        config: RunnableConfig,
        writes: Sequence[tuple[str, Any]],
        task_id: str,
        task_path: str = "",
    ) -> None:
        super().put_writes(config, writes, task_id, task_path)
        self._touch_thread(
            config["configurable"]["thread_id"],
            config["configurable"]["checkpoint_ns"],
        )


class MongoDB:
    _instance: MongoClient = None
    _checkpointers: dict[tuple[str, str, str], ThreadTTLCheckpointSaver] = {}
    _checkpointer_ttls: dict[tuple[str, str, str], int] = {}
    # database configuration
    MONGO_URI = os.getenv('MONGODB_URI', 'mongodb://localhost:27017/')
    MONGO_DB_NAME = os.getenv('MONGODB_DB_NAME', 'wa_ai_agent_db')
    MONGO_CHECKPOINT_TTL = int(os.getenv('MONGODB_CHECKPOINT_TTL', (3600 * 24) * 7))
    MONGO_ESCALATION_LOG_TTL = int(os.getenv('MONGO_ESCALATION_LOG_TTL', 60 * 30))
    MONGO_CHECKPOINT_COLLECTION = os.getenv('MONGODB_CHECKPOINT_COLLECTION', 'checkpoints')
    MONGO_CHECKPOINT_WRITES_COLLECTION = os.getenv('MONGODB_CHECKPOINT_WRITES_COLLECTION', 'checkpoint_writes')
    MONGO_MANAGER_BOT_CONFIG_COLLECTION = 'manager_bot_config'
    MONGO_TEMPLATE_MSG_COLLECTION = 'tmp_messages'
    MONGO_LOG_SYSTEM_COLLECTION = 'log_system'
    MONGO_USER_INPUT_HISTORY_COLLECTION = 'input_history'
    TOOLS_BUILDER_COLLECTION = "tools_builder"
    CATALOG_COLLECTION = "catalog"
    ESCALATION_LOG_COLLECTION = "escalation_log"

    @classmethod
    def escalation_ttl_index(cls):
        collection = cls.get_collection(cls.ESCALATION_LOG_COLLECTION)
        collection.create_index("created_at", expireAfterSeconds=cls.MONGO_ESCALATION_LOG_TTL)
        
    
    @classmethod
    def get_client(cls) -> MongoClient:
        if cls._instance is None:
            cls._instance = MongoClient(cls.MONGO_URI)
        return cls._instance
    
    @classmethod
    def get_checkpointer(
        cls,
        ttl: int | None = None,
        checkpoint_collection_name: str | None = None,
        writes_collection_name: str | None = None,
    ) -> ThreadTTLCheckpointSaver:
        effective_ttl = cls.MONGO_CHECKPOINT_TTL if ttl is None else ttl
        if effective_ttl <= 0:
            raise ValueError("MongoDB checkpointer ttl must be a positive number of seconds.")

        checkpoint_collection_name = checkpoint_collection_name or cls.MONGO_CHECKPOINT_COLLECTION
        writes_collection_name = writes_collection_name or cls.MONGO_CHECKPOINT_WRITES_COLLECTION
        cache_key = (cls.MONGO_DB_NAME, checkpoint_collection_name, writes_collection_name)

        if cache_key not in cls._checkpointers:
            cls._sync_ttl_index(checkpoint_collection_name, effective_ttl)
            cls._sync_ttl_index(writes_collection_name, effective_ttl)
            cls._checkpointers[cache_key] = ThreadTTLCheckpointSaver(
                cls.get_client(),
                db_name=cls.MONGO_DB_NAME,
                checkpoint_collection_name=checkpoint_collection_name,
                writes_collection_name=writes_collection_name,
                ttl=effective_ttl,
            )
            cls._checkpointer_ttls[cache_key] = effective_ttl
        elif cls._checkpointer_ttls[cache_key] != effective_ttl:
            raise ValueError(
                "MongoDB checkpointer ttl is collection-level. Use separate "
                "checkpoint and writes collections for different TTL values."
            )
        return cls._checkpointers[cache_key]

    @classmethod
    def _sync_ttl_index(cls, collection_name: str, ttl: int) -> None:
        collection = cls.get_collection(collection_name)
        try:
            indexes = collection.list_indexes()
            for index in indexes:
                if dict(index["key"]) != {"created_at": 1}:
                    continue

                if index.get("expireAfterSeconds") == ttl:
                    return

                cls.get_db(cls.MONGO_DB_NAME).command(
                    {
                        "collMod": collection_name,
                        "index": {
                            "keyPattern": {"created_at": 1},
                            "expireAfterSeconds": ttl,
                        },
                    }
                )
                return
        except OperationFailure as exc:
            code_name = getattr(exc, "codeName", None)
            if code_name is None and exc.details:
                code_name = exc.details.get("codeName")
            if code_name == "NamespaceNotFound" or exc.code == 26:
                return
            raise
    
    @classmethod
    def close(cls):
        if cls._instance is not None:
            for checkpointer in cls._checkpointers.values():
                checkpointer.close()
            cls._checkpointers = {}
            cls._checkpointer_ttls = {}
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
