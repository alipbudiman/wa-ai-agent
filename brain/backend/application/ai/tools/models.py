from pydantic import BaseModel, Field


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

class SendMessageInput(BaseModel):
    number: str = Field(description="Recipient identifier. Required. Use a clean numeric user ID (e.g., 6281234567890) or a full WhatsApp JID when server must be preserved (e.g., 867051314767696@lid). Never guess or fabricate this value.")
    message: str = Field(description="Message content to send. Required.")

class HelpMenuInput(BaseModel):
    number: str = Field(description="Identifier of the current human/client actively chatting with the AI. Required. Use a clean numeric user ID (e.g., 6281234567890) or full JID if available (e.g., 867051314767696@lid). Never guess or fabricate this value.")