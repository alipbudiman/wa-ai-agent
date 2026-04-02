import logging
from urllib.parse import quote

import requests
from backend.application.ai.tools.models import (
    GetUserInformationResponse,
    HelpMenuInput,
    SendMessageInput,
    SendMessageResponse
)
from backend.application.ai.tools.parse import ParseTool
from config.config import Config

from langchain_core.tools import StructuredTool



logger = logging.getLogger("uvicorn.error")


class WhatsAppIntegration:
    def __init__(self):
        self.ver = "v1"
        self.url = f"http://{Config.GIN_SERVER_HOST}:{Config.GIN_SERVER_PORT}/{self.ver}/"
        self.parse_tool = ParseTool()

    # Send Message Tools
    def __request_send_message(self, number: str, message: str) -> SendMessageResponse:
        number = (number or "").strip()
        message = (message or "").strip()

        if not message:
            return SendMessageResponse(
                success=False,
                message="message is a required field, ask the user to provide it",
                data=None,
            )
        
        if not number:
            return SendMessageResponse(
                success=False,
                message="number is a required field, ask the user to provide it",
                data=None,
            )

        try:
            number = self.parse_tool.parse_phone_number(number)
        except ValueError as e:
            return SendMessageResponse(
                success=False,
                message=str(e),
                data=None,
            )
            
        
        try:
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
            return SendMessageResponse(**payload)
        except (requests.exceptions.RequestException, ValueError) as e:
            logger.error("Error sending message to '%s': %s", number, e)
            return SendMessageResponse(
                success=False,
                message=f"failed to send message: {e}",
                data=None,
            )
    
    def send_message(self):
        """Send a message to a WhatsApp user and return a normalized success/failure response."""
        return StructuredTool.from_function(
            func=self.__request_send_message,
            name="send_message",
            description=(
                "Send a WhatsApp message to a recipient identifier. Recipient can be a clean "
                "numeric user ID (for default server) or a full JID with server suffix such as "
                "@s.whatsapp.net or @lid. Only call this tool when both recipient and message are "
                "available. If one field is missing, ask for it first and do not execute this tool. "
                "For multi-turn chat, you may combine values from previous turns before calling."
            ),
            args_schema=SendMessageInput
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
        
    # Open Help Menu Tool
    def __request_help_menu(self, number: str) -> SendMessageResponse:
        try:
            number = self.parse_tool.parse_phone_number(number)
        except ValueError as e:
            return SendMessageResponse(
                success=False,
                message=str(e),
                data=None,
            )

        try:
            response = requests.get(f"{self.url}help/{quote(number, safe='')}", timeout=120)
            response.raise_for_status()
            payload = response.json()
            if not isinstance(payload, dict):
                raise ValueError("invalid response format")

            payload.setdefault("data", None)
            if payload.get("success") is True:
                payload["message"] = "help menu sent successfully"

            return SendMessageResponse(**payload)
        except (requests.exceptions.RequestException, ValueError) as e:
            logger.error("Error opening help menu for '%s': %s", number, e)
            return SendMessageResponse(
                success=False,
                message=f"failed to open help menu: {e}",
                data=None,
            )
    
    def help_menu(self):
        """Return a help menu (information) with available commands list,
        tools, ai ability and usage instructions.
        
        Returns a normalized response with explicit success/failure status."""
        return StructuredTool.from_function(
            func=self.__request_help_menu,
            name="help_menu",
            description=(
                "Open the help menu and send it directly to the user's WhatsApp chat. "
                "After calling this tool successfully, do not repeat the full help content. "
                "Reply with a short confirmation only."
            ),
            args_schema=HelpMenuInput
        )
            