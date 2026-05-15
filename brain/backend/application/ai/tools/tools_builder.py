import hashlib
import json
import logging
import os
import re
import subprocess
import sys
from importlib import metadata
from typing import Any

from langchain_core.tools import StructuredTool
from pydantic import BaseModel, Field

from backend.application.ai.tools.models import (
    ToolAccess,
    ToolData,
    ToolDataRequest,
    ToolDataReturn,
)
from backend.infrastructure.database.database import MongoDB
from backend.application.ai.tools.whatsapp_integration import WhatsAppIntegration
from backend.application.ai.models import REPLY_AGENT


logger = logging.getLogger("uvicorn.error")


class CustomToolInput(BaseModel):
    tool_input: str | None = Field(
        default=None,
        description=(
            "Optional input for this custom database tool. Use plain text or a JSON string "
            "when the user request provides data the custom tool should process. "
            "Leave empty for information-only tools."
        ),
    )


class ToolFindRequest(BaseModel):
    query: str = Field(
        description=(
            "User request or search phrase used to find relevant custom database tools. "
            "Include key nouns, desired action, and any target domain."
        )
    )
    limit: int = Field(
        default=5,
        description="Maximum number of matching tools to return. Defaults to 5.",
    )


class ToolsBuilder:
    CODE_EXEC_TIMEOUT_SECONDS = 300
    PIP_INSTALL_TIMEOUT_SECONDS = 3600
    MAX_DESCRIPTION_CHARS = 1200
    MAX_EXEC_OUTPUT_CHARS = 12000

    def __init__(self):
        self.db = MongoDB()
        self.wa_bot = WhatsAppIntegration()
        self.tools_builder_collection = MongoDB.TOOLS_BUILDER_COLLECTION

    @staticmethod
    def _compact_text(text: str | None, max_chars: int = MAX_DESCRIPTION_CHARS) -> str:
        compact = " ".join(str(text or "").split())
        if len(compact) <= max_chars:
            return compact
        return compact[: max_chars - 3].rstrip() + "..."

    @classmethod
    def _clean_exec_output(cls, output: str) -> str:
        cleaned = str(output or "")
        cleaned = re.sub(r"\x1b\[[0-?]*[ -/]*[@-~]", "", cleaned)
        cleaned = re.sub(r"\x1b\][^\x07]*(?:\x07|\x1b\\)", "", cleaned)
        cleaned = cleaned.replace("\x0c", "\n")
        cleaned = cleaned.strip()
        if len(cleaned) <= cls.MAX_EXEC_OUTPUT_CHARS:
            return cleaned
        return (
            cleaned[: cls.MAX_EXEC_OUTPUT_CHARS].rstrip()
            + "\n... [custom tool output truncated] ..."
        )

    @staticmethod
    def _format_execution_output(tool_name: str, output: str, success: bool, exit_code: int = 0) -> str:
        status = "success" if success else "failed"
        return (
            f"CUSTOM_TOOL_EXECUTION_STATUS: {status}\n"
            f"CUSTOM_TOOL_NAME: {tool_name}\n"
            f"CUSTOM_TOOL_EXIT_CODE: {exit_code}\n\n"
            "CUSTOM_TOOL_OUTPUT_BEGIN\n"
            f"{output or '[NO OUTPUT]'}\n"
            "CUSTOM_TOOL_OUTPUT_END\n\n"
            "ASSISTANT_FINAL_RESPONSE_INSTRUCTION:\n"
            "- In your final reply, always show the captured custom tool output under an `output:` section.\n"
            "- Put the output between `---` lines so the user can see exactly what the tool produced.\n"
            "- After the output block, add a short description of what the output represents.\n"
            "- Do not say the output is meaningless just because it is decorative, ASCII art, animation frames, or repeated lines; describe it plainly."
        )

    @staticmethod
    def safe_tool_name(name: str) -> str:
        """
        Convert a database tool name into a provider-safe LangChain tool name.

        Most tool-call providers accept only letters, numbers, underscores, and
        hyphens, with practical length limits. The original database name is
        still accepted by the gateway tool.
        """
        raw_name = str(name or "").strip()
        slug = re.sub(r"[^a-zA-Z0-9_-]+", "_", raw_name).strip("_")
        slug = re.sub(r"_+", "_", slug)
        if not slug:
            slug = "custom_tool"
        if not re.match(r"^[a-zA-Z_]", slug):
            slug = f"custom_{slug}"

        if slug == raw_name and len(slug) <= 64:
            return slug

        digest = hashlib.sha1(raw_name.encode("utf-8")).hexdigest()[:8]
        base = slug[: 64 - len(digest) - 1].rstrip("_-") or "custom_tool"
        return f"{base}_{digest}"

    @staticmethod
    def _normalize_tool_record(record: dict[str, Any]) -> dict[str, Any]:
        normalized = dict(record or {})

        # Support older documents that used `tool_name` instead of `name`.
        if not normalized.get("name") and normalized.get("tool_name"):
            normalized["name"] = normalized.get("tool_name")

        code_value = normalized.get("code")
        if isinstance(code_value, str):
            normalized["code"] = {"code": code_value, "package": []}
        elif isinstance(code_value, dict):
            packages = code_value.get("package", code_value.get("packages", []))
            if packages is None:
                packages = []
            elif isinstance(packages, str):
                packages = [packages]
            elif not isinstance(packages, list):
                packages = list(packages) if isinstance(packages, (tuple, set)) else []

            code_value["package"] = [str(package).strip() for package in packages if str(package).strip()]
            normalized["code"] = code_value
        elif code_value in ("", {}, []):
            normalized["code"] = None

        return normalized

    def _collection(self):
        return self.db.get_collection(self.tools_builder_collection)

    def _enabled_filter(self) -> dict[str, Any]:
        return {
            "$or": [
                {"enabled": {"$exists": False}},
                {"enabled": True},
            ]
        }

    def _load_tools(self, agent_type: str) -> list[ToolData]:
        tools: list[ToolData] = []
        try:
            collection = self._collection()
            query = {
                "$and": [
                    ToolDataRequest(agent_type=agent_type).model_dump(),
                    self._enabled_filter(),
                ]
            }
            raw_results = collection.find(query).sort("name", 1)

            for raw_result in raw_results:
                try:
                    normalized = self._normalize_tool_record(raw_result)
                    tools.append(ToolData(**normalized))
                except Exception as parse_exc:
                    logger.warning("Failed to parse custom tool database record: %s", parse_exc)
        except Exception as exc:
            logger.warning("Failed to load custom tools for agent_type=%s: %s", agent_type, exc)

        return tools

    def get_tools_builder_list(self, agent_type: str) -> list[ToolDataReturn]:
        data: list[ToolDataReturn] = []
        for tool in self._load_tools(agent_type):
            data.append(ToolDataReturn(name=tool.name, description=tool.description))
        return data

    def get_toools_builder_list(self, agent_type: str) -> list[ToolDataReturn]:
        """Backward-compatible alias for the previous misspelled method name."""
        return self.get_tools_builder_list(agent_type=agent_type)

    def catalog_signature(self, agent_type: str) -> str:
        """
        Stable signature used to refresh LLM/tool-node caches when DB tools change.
        """
        payload = [
            tool.model_dump(mode="json", exclude_none=True)
            for tool in self._load_tools(agent_type)
        ]
        encoded = json.dumps(payload, sort_keys=True, ensure_ascii=True)
        return hashlib.sha256(encoded.encode("utf-8")).hexdigest()[:16]

    def custom_tools_catalog(self, agent_type: str) -> str:
        tools = self._load_tools(agent_type)
        if not tools:
            return ""

        lines = [
            "CUSTOM DATABASE TOOLS:",
            "Use these tools when the user's request matches a custom tool description.",
            "Prefer calling the native custom tool name when available; otherwise call `access_tools_builder` with the original tool name.",
        ]
        for tool in tools:
            safe_name = self.safe_tool_name(tool.name)
            description = self._compact_text(tool.description)
            lines.append(
                f"- original_name: {tool.name}\n"
                f"  callable_tool_name: {safe_name}\n"
                f"  description: {description}"
            )

        return "\n".join(lines)

    @staticmethod
    def _tool_search_tokens(text: str) -> set[str]:
        tokens = re.findall(r"[a-zA-Z0-9_+-]{2,}", str(text or "").lower())
        stopwords = {
            "the", "and", "for", "with", "from", "that", "this", "tool",
            "tools", "user", "request", "data", "info", "show", "get",
            "use", "using", "yang", "dan", "untuk", "dari", "dengan",
            "buat", "cari", "ambil",
        }
        return {token for token in tokens if token not in stopwords}

    def find_relevant_tools(
        self,
        *,
        agent_type: str,
        query: str,
        limit: int = 5,
    ) -> list[ToolData]:
        tools = self._load_tools(agent_type)
        if not tools:
            return []

        clean_query = str(query or "").strip()
        if not clean_query:
            return tools[: max(1, int(limit or 5))]

        query_tokens = self._tool_search_tokens(clean_query)
        scored_tools: list[tuple[int, ToolData]] = []
        lowered_query = clean_query.lower()

        for tool in tools:
            searchable_text = " ".join(
                [
                    tool.name,
                    self.safe_tool_name(tool.name),
                    tool.description,
                    tool.information or "",
                ]
            )
            lowered_text = searchable_text.lower()
            text_tokens = self._tool_search_tokens(searchable_text)
            overlap = len(query_tokens & text_tokens)
            score = overlap

            if tool.name.lower() in lowered_query or lowered_query in tool.name.lower():
                score += 8
            if self.safe_tool_name(tool.name).lower() in lowered_query:
                score += 6
            if lowered_query and lowered_query in lowered_text:
                score += 4

            if score > 0:
                scored_tools.append((score, tool))

        if not scored_tools:
            return []

        scored_tools.sort(key=lambda item: (-item[0], item[1].name.lower()))
        safe_limit = max(1, min(20, int(limit or 5)))
        return [tool for _, tool in scored_tools[:safe_limit]]

    def relevant_tools_catalog(self, *, agent_type: str, query: str, limit: int = 5) -> str:
        all_tools = self._load_tools(agent_type)
        if not all_tools:
            return "No custom database tools are configured for this agent."

        relevant_tools = self.find_relevant_tools(agent_type=agent_type, query=query, limit=limit)
        if not relevant_tools:
            available = ", ".join(tool.name for tool in all_tools)
            return (
                "No clearly relevant custom database tools were found for the query.\n"
                f"Available custom tools: {available}"
            )

        lines = [
            "RELEVANT CUSTOM DATABASE TOOLS:",
            "Use `execute_custom_tool` with original_name or callable_tool_name when one of these tools matches the task.",
        ]
        for tool in relevant_tools:
            lines.append(
                f"- original_name: {tool.name}\n"
                f"  callable_tool_name: {self.safe_tool_name(tool.name)}\n"
                f"  description: {self._compact_text(tool.description)}"
            )
        return "\n".join(lines)

    def _find_tool(self, agent_type: str, tool_name: str) -> ToolData | None:
        requested = str(tool_name or "").strip()
        if not requested:
            return None

        tools = self._load_tools(agent_type)
        for tool in tools:
            if tool.name == requested:
                return tool

        for tool in tools:
            if self.safe_tool_name(tool.name) == requested:
                return tool

        lowered = requested.lower()
        for tool in tools:
            if tool.name.lower() == lowered or self.safe_tool_name(tool.name).lower() == lowered:
                return tool

        return None

    def __install_if_missing(self, package: str) -> str:
        package_spec = str(package or "").strip()
        if not package_spec:
            return ""

        distribution_name = re.split(r"[<>=!~;\[]", package_spec, maxsplit=1)[0].strip()
        try:
            metadata.version(distribution_name)
            return f"{package_spec}: already installed"
        except metadata.PackageNotFoundError:
            pass

        logger.info("Installing custom tool package: %s", package_spec)
        process = subprocess.run(
            [sys.executable, "-m", "pip", "install", package_spec],
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
            timeout=self.PIP_INSTALL_TIMEOUT_SECONDS,
            stdin=subprocess.DEVNULL,
        )
        output = (process.stdout or "").strip()
        if process.returncode != 0:
            raise RuntimeError(
                f"Failed to install package '{package_spec}' (exit_code={process.returncode}):\n{output}"
            )

        return f"{package_spec}: installed"

    def __exec_code_from_external_tools_with_package(
        self,
        *,
        tool_name: str,
        code: str,
        package: list[str] | None,
        tool_input: str | None = None,
    ) -> str:
        install_messages: list[str] = []
        for pkg in package or []:
            install_result = self.__install_if_missing(pkg)
            if install_result:
                install_messages.append(install_result)

        custom_tool_input = "" if tool_input is None else str(tool_input)
        bootstrap = (
            "import json\n"
            "import os\n"
            "CUSTOM_TOOL_NAME = os.environ.get('CUSTOM_TOOL_NAME', '')\n"
            "CUSTOM_TOOL_INPUT = os.environ.get('CUSTOM_TOOL_INPUT', '')\n"
            "tool_input = CUSTOM_TOOL_INPUT\n"
            "try:\n"
            "    input_data = json.loads(CUSTOM_TOOL_INPUT) if CUSTOM_TOOL_INPUT else None\n"
            "except Exception:\n"
            "    input_data = CUSTOM_TOOL_INPUT\n\n"
            "_custom_tool_original_os_system = os.system\n"
            "def _custom_tool_safe_os_system(command):\n"
            "    command_text = str(command or '').strip().lower()\n"
            "    if command_text in {'clear', 'cls'}:\n"
            "        return 0\n"
            "    return _custom_tool_original_os_system(command)\n"
            "os.system = _custom_tool_safe_os_system\n\n"
        )

        env = os.environ.copy()
        env["CUSTOM_TOOL_NAME"] = str(tool_name)
        env["CUSTOM_TOOL_INPUT"] = custom_tool_input
        env.setdefault("CUSTOM_TOOL_NONINTERACTIVE", "1")
        env.setdefault("TERM", "xterm-256color")
        env.setdefault("NO_COLOR", "1")
        env.setdefault("PYTHONUNBUFFERED", "1")

        process = subprocess.run(
            [sys.executable, "-c", bootstrap + str(code)],
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
            timeout=self.CODE_EXEC_TIMEOUT_SECONDS,
            stdin=subprocess.DEVNULL,
            env=env,
        )
        output = self._clean_exec_output(process.stdout or "")

        parts: list[str] = []
        if install_messages:
            parts.append("Package preparation:\n" + "\n".join(install_messages))

        if process.returncode != 0:
            parts.append(
                self._format_execution_output(
                    tool_name=tool_name,
                    output=output,
                    success=False,
                    exit_code=process.returncode,
                )
            )
        else:
            parts.append(
                self._format_execution_output(
                    tool_name=tool_name,
                    output=output or "[OK] Custom tool code executed successfully with no output.",
                    success=True,
                    exit_code=process.returncode,
                )
            )

        return "\n\n".join(parts)

    def run_custom_tool(
        self,
        *,
        agent_type: str,
        tool_name: str,
        tool_input: str | None = None,
        sender: str | None = None,
    ) -> str:
        tool = self._find_tool(agent_type=agent_type, tool_name=tool_name)
        if not tool:
            available_names = ", ".join(item.name for item in self._load_tools(agent_type)) or "(none)"
            return (
                f"Custom tool '{tool_name}' was not found for agent_type='{agent_type}'.\n"
                f"Available custom tools: {available_names}"
            )
        if agent_type != REPLY_AGENT:
        
            self.wa_bot.request_send_message(
                sender,
                f"🔧 Tool call: {tool.name}\n",
                sender
            )
        else:
            logger.info("Tool call for REPLY_AGENT: %s", tool.name)

        response_parts = [
            f"Custom tool: {tool.name}",
            f"Description: {tool.description}",
        ]

        if tool.information:
            response_parts.append(f"Information:\n{tool.information}")

        if tool.code and tool.code.code.strip():
            try:
                result = self.__exec_code_from_external_tools_with_package(
                    tool_name=tool.name,
                    code=tool.code.code,
                    package=tool.code.package,
                    tool_input=tool_input,
                )
            except subprocess.TimeoutExpired:
                result = (
                    f"Execution timed out after {self.CODE_EXEC_TIMEOUT_SECONDS}s "
                    f"for custom tool '{tool.name}'."
                )
            except Exception as exc:
                logger.exception("Error executing custom tool '%s'", tool.name)
                result = f"Error executing custom tool '{tool.name}': {type(exc).__name__}: {exc}"
            response_parts.append(f"Execution result:\n{result}")
        elif not tool.information:
            response_parts.append("No information or executable code is configured for this custom tool.")

        return "\n\n".join(response_parts)

    def list_tools_builder(self, agent_type: str) -> StructuredTool:
        def _list() -> str:
            return self.custom_tools_catalog(agent_type=agent_type) or "No custom database tools are configured."

        return StructuredTool.from_function(
            func=_list,
            name="list_custom_tools",
            description=(
                "List custom database tools available for this agent. "
                "Use this when you need to discover which custom tools exist before selecting one."
            ),
        )

    def access_tools_builder(self, agent_type: str, sender: str) -> StructuredTool:
        def _get(tool_name: str, tool_input: str | None = None) -> str:
            return self.run_custom_tool(
                agent_type=agent_type,
                tool_name=tool_name,
                tool_input=tool_input,
                sender=sender,
            )

        return StructuredTool.from_function(
            func=_get,
            name="access_tools_builder",
            description=(
                "Call one custom database tool by original_name or callable_tool_name from the custom tools catalog. "
                "Use this when the user request matches a custom tool description. "
                "For information-only custom tools, pass only tool_name. "
                "For code-backed custom tools that need input, pass tool_input as plain text or JSON string. "
                "Custom Python code can read `tool_input`, `input_data`, `CUSTOM_TOOL_INPUT`, and `CUSTOM_TOOL_NAME`."
            ),
            args_schema=ToolAccess,
        )

    def find_relevant_tools_builder(self, agent_type: str) -> StructuredTool:
        def _find(query: str, limit: int = 5) -> str:
            return self.relevant_tools_catalog(
                agent_type=agent_type,
                query=query,
                limit=limit,
            )

        return StructuredTool.from_function(
            func=_find,
            name="find_relevant_custom_tools",
            description=(
                "Find relevant custom database tools for this agent from the user's request. "
                "Call this before execute_custom_tool when you are unsure which custom tool should be used."
            ),
            args_schema=ToolFindRequest,
        )

    def execute_tools_builder(self, agent_type: str, sender: str) -> StructuredTool:
        def _execute(tool_name: str, tool_input: str | None = None) -> str:
            return self.run_custom_tool(
                agent_type=agent_type,
                tool_name=tool_name,
                tool_input=tool_input,
                sender=sender,
            )

        return StructuredTool.from_function(
            func=_execute,
            name="execute_custom_tool",
            description=(
                "Execute one custom database tool by original_name or callable_tool_name. "
                "Use this after find_relevant_custom_tools identifies the matching tool. "
                "For information-only tools, pass only tool_name. For code-backed tools, pass tool_input as plain text or JSON string when needed."
            ),
            args_schema=ToolAccess,
        )

    def make_custom_database_tools(
        self,
        *,
        agent_type: str,
        sender: str,
        reserved_names: set[str] | None = None,
    ) -> list[StructuredTool]:
        reserved = set(reserved_names or set())
        tools: list[StructuredTool] = [
            self.list_tools_builder(agent_type=agent_type),
            self.access_tools_builder(agent_type=agent_type, sender=sender),
        ]
        reserved.update(tool.name for tool in tools)

        for custom_tool in self._load_tools(agent_type):
            safe_name = self.safe_tool_name(custom_tool.name)
            if safe_name in reserved:
                digest = hashlib.sha1(custom_tool.name.encode("utf-8")).hexdigest()[:8]
                safe_name = f"{safe_name[:55]}_{digest}"
                if safe_name in reserved:
                    logger.warning("Skipping duplicate custom tool name: %s", custom_tool.name)
                    continue

            def _run(tool_input: str | None = None, *, _tool_name: str = custom_tool.name) -> str:
                return self.run_custom_tool(
                    agent_type=agent_type,
                    tool_name=_tool_name,
                    tool_input=tool_input,
                    sender=sender,
                )

            description = (
                f"Custom database tool. Original name: {custom_tool.name}. "
                f"{self._compact_text(custom_tool.description)} "
                "Call this when the user request matches this description. "
                "Pass tool_input only when user-specific input is needed."
            )
            tools.append(
                StructuredTool.from_function(
                    func=_run,
                    name=safe_name,
                    description=description,
                    args_schema=CustomToolInput,
                )
            )
            reserved.add(safe_name)

        return tools
