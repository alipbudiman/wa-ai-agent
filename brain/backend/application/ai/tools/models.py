from pydantic import BaseModel, Field
from enum import Enum

class LevelSystem(str, Enum):
    DEBUG = "DEBUG"
    INFO = "INFO"
    WARNING = "WARNING"
    ERROR = "ERROR"
    HISTORY = "HISTORY"

class GetUserInformationData(BaseModel):
    Found: bool
    FirstName: str | None = None
    FullName: str | None = None
    PushName: str | None = None
    BusinessName: str | None = None
    RedactedPhone: str | None = None

class GetUserInformationResponse(BaseModel):
    success: bool
    message: str
    data: GetUserInformationData | None = None

class SendMessageResponse(BaseModel):
    success: bool
    message: str
    data: None
    status: str = "sent"
    on_hold: bool = False
    
class LogSystemEntry(BaseModel):
    timestamp: str = Field(description="ISO 8601 format timestamp of the log entry.")
    level: LevelSystem = Field(description="Log level indicating the severity or type of the log entry.")
    message: str = Field(description="Main log message content describing the event or action.")
    metadata: dict | None = None

class InputHistory(BaseModel):
    session_id: str = Field(description="Unique identifier for the input history entry.")
    timestamp: str = Field(description="format YYYY-MM-DD HH:MM:SS (human-readable time). e.g., 2026-04-11 14:35:22")
    sender: str = Field(description="Identifier of the user who sent the input.")
    message: str = Field(description="Content of the function that was called.")
    func_name: str = Field(description="Function name of the tool that was called.")

class SendMessageInput(BaseModel):
    number: str = Field(description="Recipient identifier. Required. Use a clean numeric user ID (e.g., 6281234567890) or a full WhatsApp JID when server must be preserved (e.g., 867051314767696@lid). Never guess or fabricate this value.")
    message: str = Field(description="Message content to send. Required.")

class SendFileInput(BaseModel):
    number: str = Field(
        description=(
            "Recipient identifier. Required. Use a clean numeric user ID "
            "(e.g., 6281234567890) or a full WhatsApp JID when server must "
            "be preserved (e.g., 867051314767696@lid). Never guess or fabricate this value."
        )
    )
    file_path: str = Field(
        description=(
            "Absolute or relative path to the file to send. Required. "
            "The file MUST be located inside the 'playground/' directory. "
            "Always compress the file/folder first using the 'compress' tool before sending. "
            "Pass the resulting compressed file path here."
        )
    )
    caption: str | None = Field(
        default=None,
        description="Optional caption or description for the file being sent."
    )

class CommandInput(BaseModel):
    command: str = Field(description="Command to execute in the terminal. Required.")

class WorkerTaskInput(BaseModel):
    task: str = Field(
        description=(
            "Canonical worker brief from the latest user request. Required. "
            "Start with the user's actual intent (same language, key entities, and constraints), "
            "then add steps/success criteria if needed. Do not replace user intent with internal "
            "diagnostic or test goals unless the user explicitly asks for testing."
        )
    )
    session_id: str | None = Field(
        default=None,
        description=(
            "Session ID from a previous worker execution. Optional. Defaults to None (new session). "
            "When the user wants to continue or resume a previous task, first call `agent_input_log` "
            "with `func_name='run_worker'` to retrieve existing sessions, then pass the matching "
            "`session_id` here so the worker resumes from prior conversation context. "
            "If no previous session exists or the task is entirely new, leave as None."
        )
    )
    work_dir: str | None = Field(
        default="playground/",
        description=(
            "Working directory for the agent's terminal commands. Optional. Defaults to 'playground/'. "
            "The agent should check existence before creating, and never create outside 'playground/' unless explicitly requested."
        ),
    )
    wait_for_completion: bool = Field(
        default=True,
        description=(
            "When true (default), wait for worker completion and return worker output in the same turn so "
            "the assistant can continue reasoning with that context. Set false for fire-and-forget background mode."
        ),
    )

class WebSearchInput(BaseModel):
    query: str = Field(
        description="Web search question or keyword from the current chat context. Required."
    )
    topic: str = Field(
        default="general",
        description=(
            "Tavily search topic. Optional. Use 'general' by default; use 'news' when the "
            "request is explicitly about recent/current events."
        ),
    )