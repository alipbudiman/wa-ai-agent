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

class SendMessageInput(BaseModel):
    number: str = Field(description="Recipient identifier. Required. Use a clean numeric user ID (e.g., 6281234567890) or a full WhatsApp JID when server must be preserved (e.g., 867051314767696@lid). Never guess or fabricate this value.")
    message: str = Field(description="Message content to send. Required.")

class CommandInput(BaseModel):
    command: str = Field(description="Command to execute in the terminal. Required.")

class WorkerTaskInput(BaseModel):
    task: str = Field(description="Task description for the agent to execute. Required.")
    work_dir: str = Field(
        default="playground/",
        description=(
            "Working directory for the agent's terminal commands. Optional. Defaults to 'playground/'. "
            "The agent should check existence before creating, and never create outside 'playground/' unless explicitly requested."
        ),
    )

class WebSearchInput(BaseModel):
    query: str = Field(
        description="Web search question or keyword from the current chat context. Required."
    )
    number: str = Field(
        description=(
            "Identifier of the current human/user actively chatting with the agent. "
            "Required. This is the active chat user ID/JID used for progress notifications, "
            "not a target contact lookup field."
        )
    )
    topic: str = Field(
        default="general",
        description=(
            "Tavily search topic. Optional. Use 'general' by default; use 'news' when the "
            "request is explicitly about recent/current events."
        ),
    )