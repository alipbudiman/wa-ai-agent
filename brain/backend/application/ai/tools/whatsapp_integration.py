import logging

import requests
from backend.application.ai.tools.models import (
    GetUserInformationResponse,
    SendMessageResponse
)
from config.config import Config


logger = logging.getLogger("uvicorn.error")


class WhatsAppIntegration:
    def __init__(self):
        self.ver = "v1"
        self.url = f"http://{Config.GIN_SERVER_HOST}:{Config.GIN_SERVER_PORT}/{self.ver}/"

    def send_message(self, number: str, message: str) -> SendMessageResponse:
        """Send a message to a WhatsApp user and return a normalized success/failure response."""
        
        try:
            response = requests.post(
                f"{self.url}message/send",
                json={"number": number, "message": message},
                timeout=30,
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

    def get_user_information(self, number: str) -> GetUserInformationResponse:
        """Fetch user information from the WhatsApp integration endpoint.
        Returns a normalized response with explicit success/failure status.
        """
        
        try:
            response = requests.get(f"{self.url}contact/info/{number}", timeout=30)
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
        
        