

from backend.infrastructure.database.database import MongoDB
from langchain_core.tools import StructuredTool

from backend.application.ai.tools.models import InputHistory, LogSystemEntry
from backend.application.ai.tools.time import TimeTool
from backend.application.ai.tools.whatsapp_integration import WhatsAppIntegration


class DBAcess:
    def __init__(self):
        self.db = MongoDB()
        self.tmp_msg_collection = MongoDB.MONGO_TEMPLATE_MSG_COLLECTION
        self.log_system_collection = MongoDB.MONGO_LOG_SYSTEM_COLLECTION
        self.input_history_collection = MongoDB.MONGO_USER_INPUT_HISTORY_COLLECTION
        self.time_tool = TimeTool()
        self.wa_bot = WhatsAppIntegration()

        
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
        
        if results: return results["pending_message"]
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
        
    # TODO: implement in GUI for see historical log system
    def db_set_log_system(self, log_entry: LogSystemEntry) -> bool:
        """Insert a log entry into the log system collection. This is currently used for recording pending messages and their status changes."""
        collection = self.db.get_collection(self.log_system_collection)
        result = collection.insert_one(log_entry.model_dump())
        return bool(result.inserted_id)
    
    def add_agent_input_log(self, input_history: InputHistory) -> bool:
        """Insert log for every agent action base on sender and message content. This is currently used for recording pending messages and their status changes."""
        collection = self.db.get_collection(self.input_history_collection)
        results = collection.insert_one(input_history.model_dump())
        return bool(results.inserted_id)
    
    def update_agent_status_input_log(self, task_id: str, new_status: str) -> bool:
        """Update the status of an existing input log entry based on task_id. This is currently used for updating the status of pending messages (e.g., from 'processing' to 'completed')."""
        collection = self.db.get_collection(self.input_history_collection)
        results = collection.find_one({"id": task_id})
        if not results:
            return False
        
        result = collection.update_one(
            {"id": task_id},
            {"$set": {"status": new_status}}
        )
        return result.modified_count > 0
    
    def delete_agent_input_log(self, task_id: str) -> bool:
        """Delete log for every agent action base on sender. This is currently used for cleaning up input history after task completion."""
        collection = self.db.get_collection(self.input_history_collection)
        result = collection.delete_many({"id": task_id})
        return result.deleted_count > 0


    def db_get_historical_pending_messages(self, sender: str) -> list:
        """Reserved for future historical pending-message lookup."""
        collection = self.db.get_collection(self.log_system_collection)
        results = collection.find(LogSystemEntry(
                sender=sender,
                level=LogSystemEntry.Level.PENDING
            ).model_dump()
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