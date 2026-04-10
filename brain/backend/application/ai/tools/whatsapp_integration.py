import logging
from urllib.parse import quote

import requests
from backend.application.ai.tools.models import (
    GetUserInformationResponse,
    SendMessageInput,
    SendMessageResponse
)
from backend.application.ai.tools.parse import ParseTool
from config.config import Config

from langchain_core.tools import StructuredTool

from backend.infrastructure.database.database import MongoDB



logger = logging.getLogger("uvicorn.error")


class WhatsAppIntegration:
    def __init__(self):
        self.ver = "v1"
        self.url = f"http://{Config.GIN_SERVER_HOST}:{Config.GIN_SERVER_PORT}/{self.ver}/"
        self.parse_tool = ParseTool()
        self.db = MongoDB()
        self.tmp_msg_collection = self.db.MONGO_TEMPLATE_MSG_COLLECTION

    # Send Message Tools
    def _safe_normalize_identifier(self, identifier: str) -> str:
        normalized = (identifier or "").strip()
        if not normalized:
            return ""

        try:
            return self.parse_tool.parse_phone_number(normalized)
        except ValueError:
            return normalized

    def _identity_key(self, identifier: str) -> str:
        normalized = self._safe_normalize_identifier(identifier)
        if "@" in normalized:
            return normalized.split("@", 1)[0]
        return normalized

    def request_send_message(self, number: str, message: str, sender: str | None = None) -> SendMessageResponse:
        number = (number or "").strip()
        message = (message or "").strip()
        sender = (sender or "").strip()

        if not message:
            return SendMessageResponse(
                success=False,
                message="message is a required field, ask the user to provide it",
                data=None,
                status="failed",
                on_hold=False,
            )
        
        if not number:
            return SendMessageResponse(
                success=False,
                message="number is a required field, ask the user to provide it",
                data=None,
                status="failed",
                on_hold=False,
            )

        try:
            number = self.parse_tool.parse_phone_number(number)
        except ValueError as e:
            return SendMessageResponse(
                success=False,
                message=str(e),
                data=None,
                status="failed",
                on_hold=False,
            )
            
        sender_identifier = self._safe_normalize_identifier(sender) if sender else number
        sender_key = self._identity_key(sender_identifier)
        target_key = self._identity_key(number)
        is_different_recipient = bool(sender_key and target_key and target_key != sender_key)
        
        try:
            if is_different_recipient:
                collection = self.db.get_collection(self.tmp_msg_collection)
                result = collection.find_one({"user": sender})
                if result is None:
                    collection.insert_one({"user": sender, "pending_number": number, "pending_message": message})
                else:
                    collection.update_one({"user": sender}, {"$set": {"pending_number": number, "pending_message": message}})

                # Notify the original sender that one outbound message is pending confirmation.
                if sender:
                    notification_result = self.request_send_message(
                        sender,
                        "💬 [1] one message on-hold, waiting for confirmation",
                        sender,
                    )
                    if not notification_result.success:
                        logger.warning(
                            "Failed to send on-hold notification to '%s': %s",
                            sender,
                            notification_result.message,
                        )
                return SendMessageResponse(
                    success=False,
                    message=(
                        "message is on-hold and has NOT been sent yet. "
                        f"Pending confirmation for recipient: {number}."
                    ),
                    data=None,
                    status="on_hold",
                    on_hold=True,
                )
            
            response = requests.post(
                f"{self.url}message/send",
                json={"number": number, "message": message},
                timeout=120,
            )
            response.raise_for_status()
            payload = response.json()
            if not isinstance(payload, dict):
                raise ValueError("invalid response format")

            payload.setdefault("data", None)
            payload.setdefault("status", "sent" if payload.get("success") else "failed")
            payload.setdefault("on_hold", False)
            return SendMessageResponse(**payload)
        except (requests.exceptions.RequestException, ValueError) as e:
            logger.error("Error sending message to '%s': %s", number, e)
            return SendMessageResponse(
                success=False,
                message=f"failed to send message: {e}",
                data=None,
                status="failed",
                on_hold=False,
            )
    
    def send_message(self, sender:str):
        """Send a message to a WhatsApp user and return a normalized success/failure response."""
        def _send(number: str, message: str) -> SendMessageResponse:
            return self.request_send_message(number, message, sender)
        
        return StructuredTool.from_function(
            func=_send,
            name="send_message",
            description=(
                "Send a WhatsApp message to a recipient identifier. Recipient can be a clean "
                "numeric user ID (for default server) or a full JID with server suffix such as "
                "@s.whatsapp.net or @lid. Only call this tool when both recipient and message are "
                "available. If one field is missing, ask for it first and do not execute this tool. "
                "Do not pass name-only aliases (example: 'kereyhan') as recipient identifier. "
                "If recipient is not numeric/JID and no confirmed number exists in context, ask for phone number/JID first. "
                "For multi-turn chat, you may combine values from previous turns before calling. "
                "Interpret tool response status carefully: status='on_hold' means message has NOT "
                "been sent and still needs explicit user confirmation."
            ),
            args_schema=SendMessageInput
        )
        
    def broadcast_message(self, message: str):
        """Broadcast a message to contacts.
        and return a normalized success/failure response. This is for one-way announcements.
        """
        message = (message or "").strip()

        if not message:
            return SendMessageResponse(
                success=False,
                message="message is a required field, ask the user to provide it",
                data=None,
                status="failed",
                on_hold=False,
            )        
        
        try:
            response = requests.post(
                f"{self.url}message/broadcast",
                json={"message": message},
                timeout=120,
            )
            response.raise_for_status()
            payload = response.json()
            if not isinstance(payload, dict):
                raise ValueError("invalid response format")

            payload.setdefault("data", None)
            payload.setdefault("status", "sent" if payload.get("success") else "failed")
            payload.setdefault("on_hold", False)
            return SendMessageResponse(**payload)
        except requests.exceptions.HTTPError as e:
            response_body = ""
            if e.response is not None:
                response_body = (e.response.text or "").strip()

            detail = str(e)
            if response_body:
                detail = f"{detail}; response_body={response_body}"

            logger.error("Error sending broadcast message: %s", detail)
            return SendMessageResponse(
                success=False,
                message=f"failed to send broadcast message: {detail}",
                data=None,
                status="failed",
                on_hold=False,
            )
        except (requests.exceptions.RequestException, ValueError) as e:
            logger.error("Error sending broadcast message: %s", e)
            return SendMessageResponse(
                success=False,
                message=f"failed to send broadcast message: {e}",
                data=None,
                status="failed",
                on_hold=False,
            )

    # Get User Information Tool
    def get_user_information(self, number: str) -> GetUserInformationResponse:
        """Fetch WhatsApp contact information for a provided target number.

        This tool accepts a clean number or a full WhatsApp JID (e.g. @lid).
        It is only for lookup by explicit target input.
        Do not use this tool to get information about the current client/human
        who is currently chatting unless their number is explicitly provided as
        the lookup target.

        Returns a normalized response with explicit success/failure status.
        """
        
        try:
            number = self.parse_tool.parse_phone_number(number)
        except ValueError as e:
            return GetUserInformationResponse(
                success=False,
                message=str(e),
                data=None,
            )

        try:
            response = requests.get(f"{self.url}contact/info/{quote(number, safe='')}", timeout=120)
            response.raise_for_status()
            payload = response.json()
            if not isinstance(payload, dict):
                raise ValueError("invalid response format")

            payload.setdefault("data", None)
            return GetUserInformationResponse(**payload)
        except (requests.exceptions.RequestException, ValueError) as e:
            logger.error("Error fetching user information for '%s': %s", number, e)
            return GetUserInformationResponse(
                success=False,
                message=f"failed to get user information: {e}",
                data=None,
            )
            