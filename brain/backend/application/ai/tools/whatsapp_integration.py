from pprint import pprint
import random
import time

from backend.application.ai.tools.models import SendFileInput
import logging
import os
from pathlib import Path
import re
from urllib.parse import quote, urlparse

import requests
from backend.application.ai.tools.models import (
    BroadcastToAdminsInput,
    GetUserInformationResponse,
    SendMessageInput,
    SendMessageResponse
)
from backend.application.ai.tools.parse import ParseTool
from backend.application.ai.tools.dbaccess import DBAcess
from config.config import Config, BASE_DIR

from langchain_core.tools import StructuredTool

from backend.infrastructure.database.database import MongoDB



logger = logging.getLogger("uvicorn.error")

MARKDOWN_LINK_RE = re.compile(r"^\s*\[([^\]]+)\]\(([^)]+)\)\s*$")
TRAILING_URL_PUNCTUATION = ".,;:"


def _clean_file_path_or_url(file_path: str) -> str:
    clean_path = (file_path or "").strip()
    markdown_match = MARKDOWN_LINK_RE.match(clean_path)
    if markdown_match:
        clean_path = markdown_match.group(2).strip()

    parsed = urlparse(clean_path)
    if parsed.scheme in {"http", "https"} and parsed.netloc:
        clean_path = clean_path.rstrip(TRAILING_URL_PUNCTUATION)

    return clean_path


class WhatsAppIntegration:
    def __init__(self):
        self.config = Config()
        self.ver = "v1"
        self.parse_tool = ParseTool()
        self.db = MongoDB()
        self.tmp_msg_collection = self.db.MONGO_TEMPLATE_MSG_COLLECTION
        self.access_db = DBAcess()
        
    def __get_self_url(self) -> str:
        """Get the base URL for the Gin server, which serves as the integration point for WhatsApp interactions."""
        gin_config = self.config.get_gin_server_config()
        # Runtime env should override persisted DB config, especially in Docker.
        host = (os.getenv("GIN_SERVER_HOST") or "").strip() or gin_config.host
        port = (os.getenv("GIN_SERVER_PORT") or "").strip() or gin_config.port
        return f"http://{host}:{port}/{self.ver}/"

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

    def _is_group_jid(self, identifier: str) -> bool:
        return (identifier or "").strip().lower().endswith("@g.us")

    def request_send_message(self, number: str, message: str, sender: str | None = None) -> SendMessageResponse:
        """send message to a whatsapp user"""
        
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

        # Determine if confirmation is needed.
        # If the target is the same as the requester (sender), skip on-hold.
        is_different_recipient = False
        if sender_key and target_key:
            is_different_recipient = (target_key != sender_key)

        logger.debug(
            "request_send_message sender=%s number=%s sender_key=%s target_key=%s is_different_recipient=%s",
            sender,
            number,
            sender_key,
            target_key,
            is_different_recipient,
        )
        
        try:
            if is_different_recipient:
                collection = self.db.get_collection(self.tmp_msg_collection)
                result = collection.find_one({"user": sender, "parameter": "send_message"})
                if result is None:
                    collection.insert_one({"user": sender, "parameter": "send_message", "pending_number": number, "pending_message": message})
                else:
                    collection.update_one({"user": sender, "parameter": "send_message"}, {"$set": {"pending_number": number, "pending_message": message}})

                # Notify the original sender that one outbound message is pending confirmation.
                if sender:
                    build_msg = "💬 [1] one message on-hold, waiting for confirmation"
                    build_msg += f"\nsend to {number}"
                    build_msg += "\ntype `/send` to confirm sending or `/unsend` to cancel"
                    build_msg += "\n*Message Preview:*```\n" + message + "\n```"
                    notification_result = self.request_send_message(
                        sender,
                        build_msg,
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
                f"{self.__get_self_url()}message/send",
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
        except Exception as e:
            logger.exception("Unexpected error sending message to '%s'", number)
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
                "Do not pass name-only aliases (example: 'ke reyhan' or 'to reyhan' without a phone number) as recipient identifier. "
                "If recipient is not numeric/JID and no confirmed number exists in context, use get_all_contacts first, "
                "ask the user to confirm the matched contact, then pass the confirmed JID/ID/Number here. "
                "For multi-turn chat, you may combine values from previous turns before calling. "
                "Interpret tool response status carefully: status='on_hold' means message has NOT "
                "been sent and still needs explicit user confirmation."
            ),
            args_schema=SendMessageInput
        )
        
    def request_send_file(
        self,
        number: str,
        file_path: str,
        caption: str | None = None,
        sender: str | None = None,
        allow_url: bool = False,
    ) -> SendMessageResponse:
        """send file to a whatsapp user"""
        number = (number or "").strip()
        file_path = _clean_file_path_or_url(file_path)
        caption = (caption or "").strip()
        sender = (sender or "").strip()

        if not file_path:
            return SendMessageResponse(
                success=False,
                message="file_path is a required field, ask the user to provide it",
                data=None,
                status="failed",
                on_hold=False,
            )
        
        # Remote URLs are restricted to trusted internal flows; local files must stay in playground/.
        parsed_file_url = urlparse(file_path)
        is_remote_file_url = parsed_file_url.scheme in {"http", "https"} and bool(parsed_file_url.netloc)
        if is_remote_file_url:
            if not allow_url:
                return SendMessageResponse(
                    success=False,
                    message="URL file sending is only allowed for trusted internal flows.",
                    data=None,
                    status="failed",
                    on_hold=False,
                )
        else:
            playground_root = str(Path(BASE_DIR, "playground").resolve())
            resolved_path = str(Path(file_path).resolve())
            if not resolved_path.startswith(playground_root):
                return SendMessageResponse(
                    success=False,
                    message="File must be inside the playground/ directory. Move or compress the file to playground/ first.",
                    data=None,
                    status="failed",
                    on_hold=False,
                )
            if not Path(resolved_path).is_file():
                return SendMessageResponse(
                    success=False,
                    message=f"File not found: {file_path}",
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
        
        # Determine if confirmation is needed.
        # If the target is the same as the requester (sender), skip on-hold.
        is_different_recipient = False
        if sender_key and target_key:
            is_different_recipient = (target_key != sender_key)
        
        try:
            if is_different_recipient:
                collection = self.db.get_collection(self.tmp_msg_collection)
                result = collection.find_one({"user": sender, "parameter": "send_file"})
                if result is None:
                    collection.insert_one({"user": sender, "parameter": "send_file", "pending_number": number, "pending_file": file_path, "pending_caption": caption})
                else:
                    collection.update_one({"user": sender, "parameter": "send_file"}, {"$set": {"pending_number": number, "pending_file": file_path, "pending_caption": caption}})

                # Notify the original sender that one outbound message is pending confirmation.
                if sender:
                    build_msg = "💬 [1] one file on-hold, waiting for confirmation"
                    build_msg += f"\nsend to {number}"
                    build_msg += "\ntype `/file-send` to confirm sending or `/file-unsend` to cancel"
                    build_msg += "\n*File Path:*```\n" + file_path + "\n```"
                    notification_result = self.request_send_message(
                        sender,
                        build_msg,
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
                f"{self.__get_self_url()}message/send-file",
                json={"number": number, "file_path": file_path, "caption": caption},
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
            logger.error("Error sending file to '%s': %s", number, e)
            return SendMessageResponse(
                success=False,
                message=f"failed to send file: {e}",
                data=None,
                status="failed",
                on_hold=False,
            )
    
    def send_file(self, sender:str):
        """Send a file to a WhatsApp user and return a normalized success/failure response."""
        def _send(number: str, file_path: str, caption: str | None = None) -> SendMessageResponse:
            return self.request_send_file(number, file_path, caption, sender)
        
        return StructuredTool.from_function(
            func=_send,
            name="send_file",
            description=(
                "Send a file to a WhatsApp recipient. The file_path MUST point to a file "
                "inside the 'playground/' directory — files outside playground are blocked. "
                "Before calling this tool, ALWAYS compress (except for images, videos or audio files) the target file/folder first using the "
                "'compress' tool (default format: zip). Pass the compressed file path as file_path. "
                "Workflow: compress → send_file. "
                "Only call this tool when recipient, file_path, and optional caption are available. "
                "If the recipient is missing, ask for it first. Do not pass name-only aliases as recipient. "
                "If recipient is not numeric/JID and no confirmed number exists in context, use get_all_contacts first, "
                "ask the user to confirm the matched contact, then pass the confirmed JID/ID/Number here. "
                "Interpret tool response status: status='on_hold' means file on hold status and waiting for user confirmation to send."
                "On-hold status only applies when the recipient is different from the sender,"
                "and needs explicit user confirmation via /file-send or /file-unsend."
            ),
            args_schema=SendFileInput
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
                f"{self.__get_self_url()}message/broadcast",
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

        This tool accepts a clean number or a full WhatsApp JID (e.g. @lid) for information about user in WhatsApp.
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
            response = requests.get(f"{self.__get_self_url()}contact/info/{quote(number, safe='')}", timeout=120)
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
    
    def get_all_commands(self):
        """Get all available commands and their descriptions."""
        try:
            response = requests.get(f"{self.__get_self_url()}commands", timeout=120)
            response.raise_for_status()
            payload = response.json()
            if not isinstance(payload, dict):
                raise ValueError("invalid response format")
            return payload.get("data", [])
        except (requests.exceptions.RequestException, ValueError) as e:
            logger.error("Error fetching commands: %s", e)
            return {"success": False, "message": f"failed to get commands: {e}", "data": None}
    
    def request_get_all_contacts(self):
        """Get all contacts from the WhatsApp server endpoint."""
        try:
            response = requests.get(f"{self.__get_self_url()}show-contacts", timeout=120)
            response.raise_for_status()
            payload = response.json()
            if not isinstance(payload, dict):
                raise ValueError("invalid response format")
            return payload.get("data", [])
        except (requests.exceptions.RequestException, ValueError) as e:
            logger.error("Error fetching contacts: %s", e)
            return {"success": False, "message": f"failed to get contacts: {e}", "data": None}

    def get_all_contacts(self, sender: str | None = None):
        """Build a tool for listing/searching WhatsApp contacts."""
        def _get_all_contacts():
            return self.request_get_all_contacts()

        return StructuredTool.from_function(
            func=_get_all_contacts,
            name="get_all_contacts",
            description=(
                "Get the user's saved WhatsApp contacts, including names and JID/ID/Number when available. "
                "Use this when the user asks to search, list, or find a contact by name, and before "
                "send_message/send_file when the recipient is name-only. Match names case-insensitively "
                "against First Name, Full Name, and Push Name. Partial names can be likely matches, but "
                "you must ask the user to confirm a likely match before using its JID/ID/Number for a send action."
            ),
        )

    def store_ai_message_to_inbox(self, message: str, number: str):
        """Store a message from AI to the inbox collection in MongoDB."""
        try:
            number = self.parse_tool.parse_phone_number(number)
        except Exception as e:
            logger.error("Error parsing phone number: %s", e)
            return {"success": False, "message": f"invalid phone number: {e}", "data": None}

        sender_identifier = self._safe_normalize_identifier(number)
        try:
            response = requests.post(
                f"{self.__get_self_url()}store-ai-message", 
                json={"message": message, "number": sender_identifier}, 
                timeout=120
            )
            response.raise_for_status()
            payload = response.json()
            if not isinstance(payload, dict):
                raise ValueError("invalid response format")
            return payload.get("data", [])
        except Exception as e:
            logger.error(f"Error storing AI message to inbox for number '{sender_identifier}': {e}")
            return {"success": False, "message": f"failed to store AI message to inbox: {e}", "data": None}

    def requests_broadcast_to_admins(self, client_number: str, message: str):
        """Broadcast a message to all admin contacts."""
        client_number = (client_number or "").strip()
        message = (message or "").strip()
        if not message:
            time.sleep(random.uniform(0.5, 2))  # Small delay to avoid rapid-fire logging
            return SendMessageResponse(
                success=False,
                message="message is a required field, ask the user to provide it",
                data=None,
                status="failed",
                on_hold=False,
            )
        
        admin_config = self.access_db.get_admins_data()
        admins = admin_config.numbers
        if not admins:
            logger.warning("No admin contacts found for broadcasting.")
            return SendMessageResponse(
                success=False,
                message="No admin contacts found for broadcasting.",
                data=None,
                status="failed",
                on_hold=False,
            )
        
        self.access_db.disable_ai_reply(client_number)

        
        sent_count = 0
        failed_admins: list[str] = []
        for admin in admins:
            admin_number = admin.number.strip()
            if not admin_number:
                continue

            notification_result = self.request_send_message(
                admin_number,
                message=f"Escalation request from {client_number}:\n\n{message}",
                sender=admin_number,
            )
            if not notification_result.success:
                failed_admins.append(admin_number)
                logger.warning(
                    "Failed to send on-hold notification to '%s': %s",
                    admin_number,
                    notification_result.message,
                )
            else:
                sent_count += 1
        
        return SendMessageResponse(
            success=sent_count > 0,
            message=(
                "Escalation request sent to admins."
                if sent_count > 0
                else "Failed to send escalation request to admins."
            ),
            data={
                "sent_count": sent_count,
                "failed_admins": failed_admins,
            },
            status="sent" if sent_count > 0 else "failed",
            on_hold=False,
        )
    
    def broadcast_to_admins(self, client_number: str):
        """Broadcast a message to all admin contacts."""
        def _broadcast(message: str) -> SendMessageResponse:
            return self.requests_broadcast_to_admins(client_number, message)
        
        return StructuredTool.from_function(
            func=_broadcast,
            name="broadcast_to_admins",
            description=(
                "Escalate the current user conversation to configured admins/CS contacts. "
                "Use this only when the user explicitly asks to talk to a human, admin, staff, CS, customer service, or operator, "
                "or when the user reports a problem that needs manual handling. "
                "Pass a concise message that includes the user's request and the reason admin/CS handling is needed. "
                "The tool disables future AI replies for this client so an admin can handle the conversation."
            ),
            args_schema=BroadcastToAdminsInput
        )
    
