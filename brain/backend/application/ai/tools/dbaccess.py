

from backend.infrastructure.database.database import MongoDB
from langchain_core.tools import StructuredTool

from backend.application.ai.tools.models import LogSystemEntry
from backend.application.ai.tools.time import TimeTool


class DBAcess:
    def __init__(self):
        self.db = MongoDB()
        self.tmp_msg_collection = MongoDB.MONGO_TEMPLATE_MSG_COLLECTION
        self.log_system_collection = MongoDB.MONGO_LOG_SYSTEM_COLLECTION
        self.time_tool = TimeTool()

        
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
    
    # def db_set_agent_code_workdir(self, sender: str, work_dir: str) -> bool:
    #     """
    #     Set the current working directory for code agent execution for a sender.

    #     This is used to maintain state across multiple code agent calls within the same conversation.
    #     For example, if the agent needs to change directory to run certain commands, we can store that state here.

    #     Returns:
    #     - True if the operation was successful (upserted a document).
    #     - False if there was an error during the database operation.
    #     """
    #     collection = self.db.get_collection(self.tmp_msg_collection)
    #     result = collection.update_one(
    #         {"user": sender},
    #         {
    #             "$set": {
    #                 "agent_code_workdir": work_dir,
    #                 "timestamp": self.time_tool.current_time(),
    #                 }
    #          },
    #         upsert=True
    #     )
    #     return result.acknowledged
        
    def db_set_log_system(self, log_entry: LogSystemEntry) -> bool:
        collection = self.db.get_collection(self.log_system_collection)
        result = collection.insert_one(log_entry.model_dump())
        return bool(result.inserted_id)

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