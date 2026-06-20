from PIL import ImageFile
from typing import Optional
import logging
from datetime import datetime
from typing import Any, List

from backend.infrastructure.database.database import MongoDB, RedisDB
from langchain_core.tools import StructuredTool

from backend.application.ai.tools.models import AdminData, AdminDataOutput, InputHistory, LogSystemEntry
from backend.application.ai.tools.time import TimeTool


logger = logging.getLogger("uvicorn.error")


class DBAcess:
    def __init__(self):
        self.db = MongoDB()
        self.tmp_msg_collection = MongoDB.MONGO_TEMPLATE_MSG_COLLECTION
        self.log_system_collection = MongoDB.MONGO_LOG_SYSTEM_COLLECTION
        self.input_history_collection = MongoDB.MONGO_USER_INPUT_HISTORY_COLLECTION
        self.manager_bot_config = MongoDB.MONGO_MANAGER_BOT_CONFIG_COLLECTION
        self.escalation_log_collection = MongoDB.ESCALATION_LOG_COLLECTION
        self.time_tool = TimeTool()
        self.redis_db = RedisDB()
        self.redis_client = self.redis_db.get_client()
        self._session_ttl = int(getattr(self.redis_db, "REDIS_SESSION_EXPIRE_SECONDS", self.redis_db.REDIS_TTL))
        self.db.escalation_ttl_index()

    @staticmethod
    def _decode_redis_value(value: Any) -> str:
        if isinstance(value, bytes):
            return value.decode("utf-8", errors="ignore")
        return str(value)

    def _decode_redis_hash(self, data: dict[Any, Any]) -> dict[str, str]:
        return {
            self._decode_redis_value(key): self._decode_redis_value(value)
            for key, value in data.items()
        }

        
    def db_get_current_pending_message(self, sender: str) -> str:
        """
        Read the current on-hold message for one sender from temporary storage.

        Returns:
        - Non-empty string: there is an on-hold message that has NOT been sent yet.
        - Empty string: no current on-hold message exists for this sender.

        This function is read-only and does not send messages.
        """
        collection = self.db.get_collection(self.tmp_msg_collection)
        results = collection.find_one({
            "user": sender,
        })
        
        if results:
            return str(results)
        else: return ""
    
    def get_current_pending_message(self, sender: str) -> StructuredTool:
        """
        Build an AI-facing tool to check whether an on-hold message still exists.

        Agent usage:
        - Call this tool when the user asks delivery status (sent vs still pending).
        - Call this tool before claiming a pending message has been sent.

        Output contract:
        - The tool ALWAYS returns a non-empty status text.
        - `ON_HOLD_EXISTS` means a pending message is still in database.
        - `NO_ON_HOLD_MESSAGE` means there is no active pending message in database.
        """
        def _get() -> str:
            pending_message = self.db_get_current_pending_message(sender)
            if pending_message:
                return (
                    "PENDING_STATUS: ON_HOLD_EXISTS\n"
                    "has_on_hold_message: true\n"
                    f"pending_message: {pending_message}"
                )

            return (
                "PENDING_STATUS: NO_ON_HOLD_MESSAGE\n"
                "has_on_hold_message: false\n"
                "pending_message:"
            )
        
        return StructuredTool.from_function(
            func=_get,
            name="get_current_pending_message",
            description=(
                "Check whether the current sender still has an on-hold message in database. "
                "Use this for pending/sent status verification. "
                "Interpret output strictly: PENDING_STATUS=ON_HOLD_EXISTS means not sent yet; "
                "PENDING_STATUS=NO_ON_HOLD_MESSAGE means no active on-hold message exists."
            ) 
        )
        
    # TODO: GUI - implement in GUI for see historical log system
    def db_set_log_system(self, log_entry: LogSystemEntry) -> bool:
        """Insert a log entry into the log system collection. This is currently used for recording pending messages and their status changes."""
        collection = self.db.get_collection(self.log_system_collection)
        result = collection.insert_one(log_entry.model_dump())
        return bool(result.inserted_id)
    
    def get_agent_input_log(self, sender: str, func_name: str) -> list[InputHistory]:
        """
Session Resume (IMPORTANT):
- When the user's message implies continuing or resuming previous work (e.g., "lanjutkan", "continue",
    "fix the code from before", "tambahkan fitur di project tadi", "perbaiki yang tadi", "update the previous project"),
    you MUST first call `agent_input_log` with `func_name='run_worker'` to check for existing sessions.
- If `agent_input_log` returns session(s), pick the most relevant `session_id` based on the task context
    and timestamp, then pass it to `run_code_agent` via the `session_id` parameter.
- This allows the worker to resume from the previous conversation context (files, tool calls, decisions)
    instead of starting from scratch, which is significantly more efficient and accurate.
- If `agent_input_log` returns NO_SESSIONS_FOUND, proceed normally with a new worker session (no session_id).
- Do NOT ask the user for a session_id; always look it up automatically via `agent_input_log`.
- When multiple sessions exist, prefer the most recent one unless the user's message clearly references an older task.
        """
        key_pattern = f"session:{sender}:{func_name}:*"
        parsed_results: list[InputHistory] = []

        try:
            for redis_key in self.redis_client.scan_iter(match=key_pattern):
                data = self.redis_client.hgetall(redis_key)
                if not data:
                    continue

                decoded = self._decode_redis_hash(data)
                decoded.setdefault("sender", sender)
                decoded.setdefault("func_name", func_name)
                decoded.setdefault("message", "")
                decoded.setdefault("timestamp", datetime.now().strftime("%Y-%m-%d %H:%M:%S"))

                if not decoded.get("session_id"):
                    key_text = self._decode_redis_value(redis_key)
                    parts = key_text.rsplit(":", 1)
                    if len(parts) == 2:
                        decoded["session_id"] = parts[1]

                try:
                    parsed_results.append(InputHistory(**decoded))
                except Exception as exc:
                    logger.warning(
                        "Skipping invalid input history record for sender=%s func=%s: %s",
                        sender,
                        func_name,
                        exc,
                    )
        except Exception as exc:
            logger.warning("Failed to read Redis session logs for sender=%s func=%s: %s", sender, func_name, exc)
            return []

        parsed_results.sort(key=lambda item: item.timestamp, reverse=True)
        return parsed_results

    def get_latest_agent_input_log(self, sender: str, func_name: str) -> InputHistory | None:
        sessions = self.get_agent_input_log(sender=sender, func_name=func_name)
        return sessions[0] if sessions else None
        

    def find_session_agent_input_log(self,sender: str, func_name: str, session_id: str) -> dict | None:
        """Get an input log entry based on task_id. This is currently used for retrieving input history after task completion."""
        key = f"session:{sender}:{func_name}:{session_id}"
        try:
            data = self.redis_client.hgetall(key)
            if not data:
                return None

            self.redis_client.expire(key, self._session_ttl)  # Refresh expiration on access
            return self._decode_redis_hash(data)
        except Exception as exc:
            logger.warning("Failed to read Redis session key=%s: %s", key, exc)
            return None
        
    
    def add_agent_input_log(self, input_history: InputHistory) -> bool:
        """Insert log for every agent action base on sender and message content. This is currently used for recording pending messages and their status changes."""
        key = f"session:{input_history.sender}:{input_history.func_name}:{input_history.session_id}"
        record_map = input_history.model_dump()
        if not record_map.get("timestamp"):
            record_map["timestamp"] = datetime.now().strftime("%Y-%m-%d %H:%M:%S")

        try:
            self.redis_client.hset(key, mapping=record_map)
            self.redis_client.expire(key, self._session_ttl)
            data = self.redis_client.hgetall(key)
            if not data:
                return False
            return True
        except Exception as exc:
            logger.warning("Failed to write Redis input history key=%s: %s", key, exc)
            return False
    
    def update_agent_status_input_log(self, sender: str, session_id: str, func_name: str, message: str) -> bool:
        """Update the status of an existing input log entry based on task_id. This is currently used for updating the status of pending messages (e.g., from 'processing' to 'completed')."""
        if not session_id:
            return False

        key = f"session:{sender}:{func_name}:{session_id}"
        try:
            self.redis_client.hset(
                key,
                mapping={
                    "sender": sender,
                    "func_name": func_name,
                    "session_id": session_id,
                    "message": message,
                    "timestamp": datetime.now().strftime("%Y-%m-%d %H:%M:%S"),
                },
            )
            self.redis_client.expire(key, self._session_ttl)
            return bool(self.redis_client.hgetall(key))
        except Exception as exc:
            logger.warning("Failed to update Redis session key=%s: %s", key, exc)
            return False
    
    def clear_agent_input_log(self):
        """Clear all input logs. This is currently used for cleaning up all session logs, e.g., during testing or when resetting the system."""
        key_pattern = f"session:*"
        try:
            keys_deleted = 0
            for redis_key in self.redis_client.scan_iter(match=key_pattern):
                self.redis_client.delete(redis_key)
                keys_deleted += 1
            logger.info("Cleared %d Redis session logs with pattern=%s", keys_deleted, key_pattern)
            return True
        except Exception as exc:
            logger.warning("Failed to clear Redis session logs with pattern=%s: %s", key_pattern, exc)
            return False

    def delete_agent_input_log(self, session_id: str) -> bool:
        """Delete log for every agent action base on sender. This is currently used for cleaning up input history after task completion."""
        collection = self.db.get_collection(self.input_history_collection)
        result = collection.delete_many({"session_id": session_id})
        return result.deleted_count > 0

    def db_get_historical_pending_messages(self, sender: str) -> list:
        """Reserved for future historical pending-message lookup."""
        collection = self.db.get_collection(self.log_system_collection)
        results = collection.find(
            {
                "$or": [
                    {"sender": sender},
                    {"metadata.sender": sender},
                ]
            }
        ).sort("timestamp", -1)
        return list(results)
    
    def get_historical_pending_messages(self, sender: str) -> StructuredTool:
        """Reserved for future historical pending-message lookup."""
        def _get() -> list:
            historical_messages = self.db_get_historical_pending_messages(sender)
            return historical_messages
        
        return StructuredTool.from_function(
            func=_get,
            name="get_historical_pending_messages",
            description=(
                "Check the historical pending messages for the sender. "
                "This is reserved for future use and currently returns all historical pending messages as a string."
            ) 
        )
    
    @staticmethod
    def _normalize_admin_data(admin: Any) -> AdminData | None:
        """Normalize admin records from API input or legacy database shape."""
        try:
            if isinstance(admin, AdminData):
                admin_data = admin
            elif isinstance(admin, dict):
                number = str(admin.get("number") or "").strip()
                name = str(admin.get("name") or number).strip()
                admin_data = AdminData(name=name, number=number)
            elif isinstance(admin, str):
                number = admin.strip()
                if not number:
                    return None
                admin_data = AdminData(name=number, number=number)
            else:
                return None
        except Exception as exc:
            logger.warning("Skipping invalid admin record=%r: %s", admin, exc)
            return None

        name = admin_data.name.strip() or admin_data.number.strip()
        number = admin_data.number.strip()
        if not number:
            return None

        return AdminData(name=name, number=number)

    @classmethod
    def _normalize_admins_data(cls, admins: list[Any]) -> list[AdminData]:
        normalized_admins: list[AdminData] = []
        seen_numbers: set[str] = set()

        for admin in admins:
            admin_data = cls._normalize_admin_data(admin)
            if admin_data is None or admin_data.number in seen_numbers:
                continue

            normalized_admins.append(admin_data)
            seen_numbers.add(admin_data.number)

        return normalized_admins

    @staticmethod
    def _dump_admins_data(admins: list[AdminData]) -> list[dict[str, str]]:
        return [admin.model_dump() for admin in admins]

    def get_admins_data(self) -> AdminDataOutput:
        """Get admin configuration using the same shape as the database document."""
        collection = self.db.get_collection(self.manager_bot_config)
        results = collection.find_one({
            "parameter": "admins"
        })

        # Fetch waiting list from escalation log collection instead of admins document
        escalation_collection = self.db.get_collection(self.escalation_log_collection)
        waiting_for_admins = [
            doc["identifier"]
            for doc in escalation_collection.find({}, {"identifier": 1})
            if "identifier" in doc
        ]
        waiting_for_admins = sorted(list(set(waiting_for_admins)))

        if results is None:
            output = AdminDataOutput(waiting_for_admins=waiting_for_admins)
            collection.insert_one({
                "parameter": "admins",
                "numbers": self._dump_admins_data(output.numbers),
            })
            return output
        
        raw_admins = results.get("numbers", [])
        should_update_admins = False
        if not isinstance(raw_admins, list):
            raw_admins = []
            should_update_admins = True

        admins = self._normalize_admins_data(raw_admins)
        normalized_admins = self._dump_admins_data(admins)
        
        output = AdminDataOutput(
            parameter="admins",
            numbers=admins,
            waiting_for_admins=waiting_for_admins,
        )

        if should_update_admins or raw_admins != normalized_admins:
            collection.update_one(
                {"parameter": "admins"},
                {
                    "$set": {
                        "numbers": normalized_admins,
                    }
                },
            )

        return output

    
    def set_admins_data(self, numbers: List[AdminData]) -> bool:
        """Add or update admin users in the database, deduplicated by number."""
        new_admins = self._normalize_admins_data(numbers)
        if not new_admins:
            return False

        collection = self.db.get_collection(self.manager_bot_config)
        existing_admins = self.get_admins_data().numbers
        admins_by_number = {admin.number: admin for admin in existing_admins}

        for admin in new_admins:
            admins_by_number[admin.number] = admin

        merged_admins = list(admins_by_number.values())
        result = collection.update_one(
            {"parameter": "admins"},
            {"$set": {"numbers": self._dump_admins_data(merged_admins)}},
            upsert=True
        )
        return result.modified_count > 0 or result.upserted_id is not None
    
    def remove_admins_data(self, numbers: List[AdminData]) -> bool:
        """Remove admin users from the database by number."""
        admins_to_remove = self._normalize_admins_data(numbers)
        if not admins_to_remove:
            return False

        collection = self.db.get_collection(self.manager_bot_config)
        remove_numbers = {admin.number for admin in admins_to_remove}
        remaining_admins = [
            admin
            for admin in self.get_admins_data().numbers
            if admin.number not in remove_numbers
        ]

        result = collection.update_one(
            {"parameter": "admins"},
            {"$set": {"numbers": self._dump_admins_data(remaining_admins)}}
        )
        return result.modified_count > 0
    
    def get_waiting_for_admin_list(self) -> List[str]:
        """Get the list of identifiers that are currently waiting for admin approval."""
        collection = self.db.get_collection(self.escalation_log_collection)
        results = collection.find()
        waiting_list = []
        for result in results:
            if "identifier" in result:
                waiting_list.append(result["identifier"])
        return sorted(waiting_list)

    def get_waiting_for_admin(self, identifier: str) -> Optional[str]:
        """Get the identifier that are currently waiting for admin approval."""
        collection = self.db.get_collection(self.escalation_log_collection)
        results = collection.find_one({"identifier": identifier})
        if results is None:
            return None
        return results.get("identifier", None)

    def disable_ai_reply(self, identifier: str) -> bool:
        """Add a phone number to the admin waiting list."""
        identifier = (identifier or "").strip()
        if not identifier:
            return False

        collection = self.db.get_collection(self.escalation_log_collection)
        find = collection.find_one({"identifier": identifier})
        if find:
            return True
        collection.insert_one({
            "identifier": identifier,
            "created_at": datetime.utcnow(),
        })
        return True
    
    def enable_ai_reply(self, identifier: str) -> bool:
        """Remove a phone number from the admin waiting list."""
        identifier = (identifier or "").strip()
        if not identifier:
            return False

        collection = self.db.get_collection(self.escalation_log_collection)
        result = collection.delete_one({"identifier": identifier})
        return result.deleted_count > 0