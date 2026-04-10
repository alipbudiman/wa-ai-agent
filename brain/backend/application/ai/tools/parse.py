
import re
from langchain_core.tools import StructuredTool


KNOWN_WHATSAPP_SERVERS = {
    "s.whatsapp.net",
    "g.us",
    "c.us",
    "broadcast",
    "lid",
    "msgr",
    "interop",
    "newsletter",
    "hosted",
    "hosted.lid",
    "bot",
}

NUMERIC_USER_SERVERS = {
    "s.whatsapp.net",
    "c.us",
    "lid",
    "hosted",
    "hosted.lid",
    "msgr",
    "interop",
    "bot",
}


class ParseTool:
    def __init__(self):
        pass

    def _normalize_numeric_user(self, value: str, original: str, apply_local_prefix: bool = True) -> str:
        user_part = value.strip()
        if not user_part:
            raise ValueError(f"Unable to parse phone number: {original}")

        if ":" in user_part:
            user_part = user_part.split(":", 1)[0]
        if "." in user_part:
            possible_user = user_part.split(".", 1)[0]
            if possible_user.isdigit():
                user_part = possible_user

        if user_part.startswith("+"):
            user_part = user_part[1:]
        if user_part.startswith("00"):
            user_part = user_part[2:]

        digits = re.sub(r"\D", "", user_part)
        if not digits:
            raise ValueError(f"Unable to parse phone number: {original}")

        if apply_local_prefix:
            if digits.startswith("0"):
                digits = f"62{digits[1:]}"
            elif digits.startswith("8"):
                digits = f"62{digits}"

        if len(digits) < 8 or len(digits) > 20:
            raise ValueError(f"Unable to parse phone number: {original}")

        return digits

    def parse_phone_number(self, number: str) -> str:
        """Normalize a raw phone/JID input into a WhatsApp identifier.

        This helper is intended to prepare user-provided phone text before it is
        passed to WhatsApp tools such as send_message, get_user_information, or
        help_menu.

        Accepted input examples:
        - 08211222
        - +62 821-1222
        - 0812-3456-7890@s.whatsapp.net
        - 867051314767696@lid
        - 6281234567890

        Normalization behavior:
        - if input is a full JID, preserves the server suffix (e.g. @lid)
        - normalizes numeric user IDs for user-based servers
        - supports plain numbers by converting them into clean numeric user IDs
        - converts Indonesian local prefixes:
          - 0xxxxxxxxxx -> 62xxxxxxxxxx
          - 8xxxxxxxxxx -> 628xxxxxxxxx

        Returns:
            A clean WhatsApp identifier:
            - digits-only user ID for plain number input (example: 6281234567890)
            - full JID when server is provided (example: 867051314767696@lid)

        Raises:
            ValueError: if the input cannot be parsed into a valid identifier.
        """
        if not isinstance(number, str):
            raise ValueError("Phone number must be a string")

        raw = number.strip()
        if not raw:
            raise ValueError("Phone number is empty")

        if "@" in raw:
            user_part, server = raw.split("@", 1)
            server = server.strip().lower()
            if not server:
                raise ValueError(f"Unable to parse phone number: {number}")
            if server not in KNOWN_WHATSAPP_SERVERS:
                raise ValueError(f"Unsupported WhatsApp server in identifier: {server}")

            if server in NUMERIC_USER_SERVERS:
                apply_local_prefix = server in {"s.whatsapp.net", "c.us"}
                normalized_user = self._normalize_numeric_user(
                    user_part,
                    number,
                    apply_local_prefix=apply_local_prefix,
                )
            else:
                normalized_user = re.sub(r"\s+", "", user_part)
                if not normalized_user:
                    raise ValueError(f"Unable to parse phone number: {number}")

            return f"{normalized_user}@{server}"

        return self._normalize_numeric_user(raw, number)

    def parse_phone_number_tool(self) -> StructuredTool:
        """Build a non-throwing parser tool for LLM use.

        This wrapper prevents parser exceptions from aborting the LangGraph tool node.
        """

        def _parse(number: str) -> str:
            raw = (number or "").strip()
            if not raw:
                return (
                    "PARSE_STATUS: MISSING_INPUT\n"
                    "normalized_identifier:\n"
                    "reason: recipient is empty. Ask user for phone number or full WhatsApp JID."
                )

            # Name-only aliases (without digits/JID) must be clarified first.
            if "@" not in raw and not re.search(r"\d", raw):
                return (
                    "PARSE_STATUS: NEED_EXPLICIT_NUMBER\n"
                    "normalized_identifier:\n"
                    "reason: recipient looks like a name/alias without numeric or JID information. "
                    "Ask user for phone number or full JID before sending."
                )

            try:
                normalized = self.parse_phone_number(raw)
            except ValueError as err:
                return (
                    "PARSE_STATUS: INVALID\n"
                    "normalized_identifier:\n"
                    f"reason: {err}"
                )

            return (
                "PARSE_STATUS: OK\n"
                f"normalized_identifier: {normalized}"
            )

        return StructuredTool.from_function(
            func=_parse,
            name="parse_phone_number",
            description=(
                "Safely validate and normalize recipient phone/JID. "
                "Never throws tool exceptions. "
                "Use before send_message when recipient candidate includes digits or JID. "
                "If PARSE_STATUS=NEED_EXPLICIT_NUMBER, ask user for number/JID and do not call send_message yet."
            ),
        )
