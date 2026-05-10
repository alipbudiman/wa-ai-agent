from typing import Any
import logging
import json
import os
import re
import uuid
from pathlib import Path


from backend.application.ai.ai import AI
from backend.application.ai.tools.websearch import WebSearchTool
from backend.application.ai.tools.time import TimeTool
from backend.application.ai.tools.parse import ParseTool

from backend.application.ai.tools.whatsapp_integration import WhatsAppIntegration
from backend.application.ai.instruction import Instruction
from backend.infrastructure.database.database import MongoDB
from backend.application.ai.tools.dbaccess import DBAcess
from backend.application.ai.tools.models import LevelSystem, LogSystemEntry
from backend.application.ai.tools.terimnalx import TerminalX
from backend.application.ai.tools.filex import FileX
from backend.application.ai.code import AgentCodeUsecase
from backend.application.ai.reaserch import ResearchAgentUsecase
from backend.application.ingestion.loader import DocumentLoader
from backend.application.ai.models import MAIN_AGENT
from backend.application.ai.tools.tools_builder import ToolsBuilder
from config.config import Config, BASE_DIR

from langgraph.graph import StateGraph, START, END # pyright: ignore[reportMissingTypeStubs]
from langgraph.graph import MessagesState
from langgraph.prebuilt import ToolNode
from langchain_core.messages import HumanMessage, AIMessage, SystemMessage, RemoveMessage, ToolMessage
from langchain_community.agent_toolkits import FileManagementToolkit

from langchain_core.messages import trim_messages
from typing_extensions import Literal
from pydantic import BaseModel


logger = logging.getLogger('uvicorn.error')

class State(MessagesState):
    summary: str
    sender: str
    rag_context: str = ""
    rag_query: str = ""

class FeedBackType(BaseModel):
    is_approved: str

class ChatAgentUseCase:
    TUNNING_PARAM = 3
    RAG_TOP_K = 5
    RAG_SCORE_THRESHOLD = 0.6
    
    ASSISTANT_CONTEXT_MAX_TOKENS = 8000 * TUNNING_PARAM
    DIRECT_CHAT_CONTEXT_MAX_TOKENS = 2048 * TUNNING_PARAM
    SUMMARIZE_TRIGGER_TOKENS = 12000 * TUNNING_PARAM
    SUMMARIZE_MIN_MESSAGES = 12 * TUNNING_PARAM
    SUMMARIZE_KEEP_RECENT_MESSAGES = 12 * TUNNING_PARAM
    SUMMARIZE_SOURCE_MAX_TOKENS = 12000 * TUNNING_PARAM
    
    logger.info(
        "ChatAgentUseCase initialized with tunning_param=%s, assistant_context_max_tokens=%s, direct_chat_context_max_tokens=%s, summarize_trigger_tokens=%s, summarize_min_messages=%s, summarize_keep_recent_messages=%s, summarize_source_max_tokens=%s",
        TUNNING_PARAM,
        ASSISTANT_CONTEXT_MAX_TOKENS,
        DIRECT_CHAT_CONTEXT_MAX_TOKENS,
        SUMMARIZE_TRIGGER_TOKENS,
        SUMMARIZE_MIN_MESSAGES,
        SUMMARIZE_KEEP_RECENT_MESSAGES,
        SUMMARIZE_SOURCE_MAX_TOKENS,
    )

    @staticmethod
    def __resolve_langgraph_root_dir() -> str:
        """
        Resolve a writable playground root for FileManagementToolkit.

        Priority:
        1) PLAYGROUND_ROOT env override
        2) repository root playground (BASE_DIR/playground)
        3) Docker shared mount default (/app/playground)
        4) cwd playground fallback
        """
        candidates = []

        env_root = (os.getenv("PLAYGROUND_ROOT") or "").strip()
        if env_root:
            candidates.append(Path(env_root).expanduser())

        candidates.append(Path(BASE_DIR).resolve() / "playground")
        candidates.append(Path("/app/playground"))
        candidates.append(Path.cwd() / "playground")

        for candidate in candidates:
            try:
                resolved = candidate.resolve(strict=False)
                resolved.mkdir(parents=True, exist_ok=True)
                return str(resolved)
            except OSError:
                continue

        # Last-resort fallback should still be writable in most environments.
        fallback = Path.cwd() / "playground"
        fallback.mkdir(parents=True, exist_ok=True)
        return str(fallback.resolve(strict=False))

    def __init__(self):
        self.llm = AI()
        self.config = Config()
        self.websearch_tool = WebSearchTool()
        self.time_tool = TimeTool()
        self.parse_tool = ParseTool()
        self.wa_integration_tool = WhatsAppIntegration()
        self.db_access_tool = DBAcess()
        self.terminalx_tool = TerminalX()
        self.filex_tool = FileX()
        self.tools_builder = ToolsBuilder()
        self.agent_code = AgentCodeUsecase()
        self.agent_reaserch = ResearchAgentUsecase()
        self.doc_loader = DocumentLoader(collection_name=MAIN_AGENT)
        self.instruction = Instruction()
        self.langgraph_file_manager_root = self.__resolve_langgraph_root_dir()
        self.langgraph_file_manager = FileManagementToolkit(root_dir=self.langgraph_file_manager_root)
        self.langgraph_file_manager_tools = self.langgraph_file_manager.get_tools()
        logger.info("FileManagementToolkit root_dir=%s", self.langgraph_file_manager_root)
        self.checkpointer = MongoDB.get_checkpointer()
        self.__init_langsmith()
        self.react_graph = self.__agent_logic()
        
        # sensitive tools that require extra caution and should need user confirmation before use can be listed here for additional guardrails or monitoring; currently we rely on the assistant's judgment and system instructions to use tools appropriately, but this list can be used to trigger extra checks or alerts when certain tools are invoked
        self._sensitive_tools = [
            self.filex_tool.delete_file_dir.__name__,
        ]
        
        self._need_decide = {
            str(self.terminalx_tool.simple_exec.__name__): {
                "description": "Execute simple terminal commands without complex chaining or scripting. Use with caution and avoid commands that can cause harm or require elevated permissions.",
                "sensitive": ["delete", "rm", "shutdown", "reboot", "format", "mkfs", "dd", ">:","|","&&",";"],
            }

        }
        
        # static tools that do not require state access can be initialized once and reused across turns/users; dynamic tools that require state access (e.g. sender) must be generated per turn in __get_tools
        self.__static_tools = [
            self.instruction.capability_description,
            self.time_tool.current_time,
            self.parse_tool.parse_phone_number_tool(),
            self.wa_integration_tool.get_user_information,
            self.wa_integration_tool.broadcast_message,
            self.filex_tool.compress,
            self.filex_tool.delete_file_dir,
            self.websearch_tool.search_wikipedia,
            self.terminalx_tool.screen_command,
            *self.langgraph_file_manager_tools,
        ]
        
        self._llm_cache: dict[str, Any] = {}
        self._tool_node_cache: dict[str, ToolNode] = {}
        self._human_interrup_chche: dict[str, str] = {} # user number -> interruption message
        self._pending_sensitive_tool_calls: dict[str, list[dict]] = {}
        self._process_diagram_mermaid = ""
        self._reasoning = True
    
    @staticmethod
    def __token_counter(messages: list) -> int:
        """
        Estimate total token usage for message batches.

        This remains an approximation, but it accounts for structured content,
        per-message overhead, and optional tool calls to reduce large undercount
        risks on long-running conversations.
        """
        total_tokens = 0

        for message in messages:
            raw_content = getattr(message, "content", "")

            if isinstance(raw_content, list):
                parts = []
                for part in raw_content:
                    if isinstance(part, str):
                        parts.append(part)
                        continue

                    if isinstance(part, dict):
                        text = part.get("text")
                        parts.append(str(text) if text else str(part))
                        continue

                    parts.append(str(part))

                content_text = "\n".join(parts)
            else:
                content_text = str(raw_content)

            if content_text.strip():
                total_tokens += max(len(content_text) // 4, 1)

            # Include lightweight envelope overhead to avoid too-optimistic sizing.
            total_tokens += 6

            tool_calls = getattr(message, "tool_calls", None) or []
            if tool_calls:
                total_tokens += len(tool_calls) * 20

        return total_tokens

    def __summarize_conversation(self, state: State):
        # First, we get any existing summary
        summary = state.get("summary", "")

        # Create our summarization prompt 
        if summary:
            
            # A summary already exists
            summary_message = (
                f"This is summary of the conversation to date: {summary}\n\n"
                "Extend the summary by taking into account the new messages above:"
            )
            
        else:
            summary_message = "Create a summary of the conversation above:"

        # Summarize from a bounded recent window to avoid recursive context blowups.
        summary_source = trim_messages(
            state.get("messages", []),
            max_tokens=self.SUMMARIZE_SOURCE_MAX_TOKENS,
            strategy="last",
            token_counter=self.__token_counter,
            allow_partial=False,
        )

        # Add prompt to our history
        messages = summary_source + [HumanMessage(content=summary_message)]
        response = self.__llm(reasoning=self._reasoning).invoke(messages)
        
        # Keep a short recent window and rely on summary for older history.
        keep_recent = self.SUMMARIZE_KEEP_RECENT_MESSAGES
        delete_messages = [
            RemoveMessage(id=m.id)
            for m in state.get("messages", [])[:-keep_recent]
            if getattr(m, "id", None)
        ]

        summary_text = self.__content_to_text(response.content) or summary
        return {"summary": summary_text, "messages": delete_messages}

    def __should_summarize(self, state: State) -> bool:
        """
        Decide if conversation should be summarized based on size signals.

        We gate on both message count and token estimate to avoid summarizing
        too early while still protecting long-running sessions.
        """
        messages = state.get("messages", [])
        if len(messages) < self.SUMMARIZE_MIN_MESSAGES:
            return False

        return self.__token_counter(messages) >= self.SUMMARIZE_TRIGGER_TOKENS

    def __should_continue(self, state: State) -> Literal ["summarize_conversation", "assistant"]:
        """Return the next node to execute."""

        if self.__should_summarize(state):
            return "summarize_conversation"

        # Otherwise we continue to the assistant node
        return "assistant"

    def __pre_assistant(self, state: State):
        """
        Pass-through node used to centralize pre-assistant routing checks.

        Keeping this as an explicit node lets the graph reuse the same
        summarize-or-assistant decision before every assistant execution,
        including cycles after tool calls.
        """
        return {}

    def __has_pending_tool_calls(self, state: State) -> bool:
        """Detect tool calls from the latest AI output in state."""
        for message in reversed(state.get("messages", [])):
            if not isinstance(message, AIMessage):
                continue

            tool_calls = getattr(message, "tool_calls", None) or []
            if tool_calls:
                return True

            additional_kwargs = getattr(message, "additional_kwargs", {}) or {}
            if additional_kwargs.get("tool_calls"):
                return True

            return False

        return False

    def __route_after_assistant(self, state: State) -> Literal["tools", "human_feedback", "end"]:
        """
        Route assistant output deterministically.

        Priority:
        1) request human confirmation for sensitive/risky tool calls
        2) execute tools when safe to proceed
        3) finish turn when no further work is needed
        """
        if self.__pending_confirmation_calls(state):
            return "human_feedback"

        if self.__has_pending_tool_calls(state):
            return "tools"

        return "end"
    
    def __llm(self, reasoning: bool = True, structured_output: Any = None):
        ai_config =  self.config.get_agent_model_config()
        agent_model_config = ai_config.agent_model if ai_config.agent_model.agent_provider != "default" else None
        return self.llm.LLM(reasoning=reasoning, providers=agent_model_config, structured_output=structured_output)

    def __content_to_text(self, content) -> str:
        """
        Normalize message/model content into plain text.

        LangChain messages may contain either a string or a list payload. This
        helper keeps routing logic stable across both formats.
        """
        if isinstance(content, str):
            return content.strip()

        if isinstance(content, list):
            text_parts = []
            for part in content:
                if isinstance(part, str):
                    text_parts.append(part)
                    continue

                if isinstance(part, dict):
                    text = part.get("text")
                    if text:
                        text_parts.append(str(text))

            return "\n".join(text_parts).strip()

        return str(content).strip()

    @staticmethod
    def __is_internal_system_log_text(text: str) -> bool:
        """Identify internal synthetic logs that must stay hidden from users."""
        normalized = (text or "").strip()
        if not normalized:
            return False

        lowered = normalized.lower()
        return lowered.startswith("system execution log:") or lowered.startswith("[systemevent]")

    def __latest_user_text(self, state: State, max_messages: int = 5) -> str:
        """
        Return concatenated latest user text messages from graph state.

        The graph state can include system and assistant messages from
        checkpointed history, so this function explicitly searches for recent
        HumanMessage entries only. The returned text keeps chronological order
        (oldest-to-newest within the selected window).
        """
        if max_messages < 1:
            max_messages = 1

        collected = []
        for message in reversed(state.get("messages", [])):
            if not isinstance(message, HumanMessage):
                continue

            text = self.__content_to_text(message.content)
            if not text:
                continue

            collected.append(text)
            if len(collected) >= max_messages:
                break

        if not collected:
            return ""

        return "\n".join(reversed(collected))

    def __retrieve(self, state: State):
        """
        Retrieve relevant uploaded documents for the main chat agent.

        This is read-only against the agent-specific vector collection and is
        intentionally fail-open so normal chat/tool routing still works when
        no document context is available.
        """
        try:
            query = str(state.get("rag_query") or "").strip()
            if not query:
                query = self.__latest_user_text(state, max_messages=1)

            if not query:
                return {"rag_context": ""}

            docs_with_scores = self.doc_loader.similarity_search_with_relevance_scores(
                query=query,
                k=self.RAG_TOP_K,
                score_threshold=self.RAG_SCORE_THRESHOLD,
            )

            if not docs_with_scores:
                logger.info("Main agent RAG: no relevant documents passed the threshold")
                return {"rag_context": ""}

            context_parts = [
                (
                    f"[source: {doc.metadata.get('source', '?')} | "
                    f"chunk: {doc.metadata.get('chunk_index', doc.metadata.get('chunk_id', '?'))} | "
                    f"score: {score:.3f}]\n{doc.page_content}"
                )
                for doc, score in docs_with_scores
            ]
            logger.info("Main agent RAG: %d relevant documents found", len(docs_with_scores))
            return {"rag_context": "\n\n".join(context_parts)}

        except Exception as err:
            logger.error("Main agent RAG retrieve error: %s", err)
            return {"rag_context": ""}

    @staticmethod
    def __coerce_tool_args(args: any) -> dict:
        """Normalize tool-call args into dict shape."""
        if isinstance(args, dict):
            return args

        if isinstance(args, str):
            stripped = args.strip()
            if not stripped:
                return {}

            try:
                parsed = json.loads(stripped)
            except json.JSONDecodeError:
                return {"raw": stripped}

            if isinstance(parsed, dict):
                return parsed

            return {"value": parsed}

        if args is None:
            return {}

        return {"value": str(args)}

    def __normalize_tool_call(self, raw_call: any) -> dict | None:
        """Normalize heterogeneous tool-call payloads into one structure."""
        if not isinstance(raw_call, dict):
            return None

        name = str(raw_call.get("name") or "").strip()
        args = raw_call.get("args")
        call_id = raw_call.get("id")
        call_type = raw_call.get("type", "tool_call")

        # OpenAI-style wrapper: {"function": {"name": ..., "arguments": ...}}
        function_payload = raw_call.get("function")
        if isinstance(function_payload, dict):
            if not name:
                name = str(function_payload.get("name") or "").strip()
            if args is None:
                args = function_payload.get("arguments")

        if not name:
            return None

        normalized = {
            "name": name,
            "args": self.__coerce_tool_args(args),
            "id": str(call_id) if call_id else f"toolcall-{uuid.uuid4().hex[:12]}",
            "type": str(call_type or "tool_call"),
        }
        return normalized

    @staticmethod
    def __looks_like_textual_tool_call(text: str) -> bool:
        normalized = (text or "").strip().upper()
        if "[TOOL_CALL]" in normalized and "[/TOOL_CALL]" in normalized:
            return True
        if "<MINIMAX:TOOL_CALL>" in normalized and "</MINIMAX:TOOL_CALL>" in normalized:
            return True
        return False

    def __extract_textual_tool_calls(self, text: str) -> list[dict]:
        """
        Parse textual tool-call markup emitted by non-native tool-call models.

        Supported formats:
        1. [TOOL_CALL]{tool => "name", args => {...}}[/TOOL_CALL]
        2. <minimax:tool_call><invoke name="name"><parameter name="p">val</parameter></invoke></minimax:tool_call>
        """
        source = (text or "").strip()
        if not self.__looks_like_textual_tool_call(source):
            return []

        parsed_calls: list[dict] = []

        # 1. Handle Minimax / XML tag style
        minimax_blocks = re.findall(r"<minimax:tool_call>(.*?)</minimax:tool_call>", source, flags=re.IGNORECASE | re.DOTALL)
        for block in minimax_blocks:
            invokes = re.findall(r"<invoke name=\"(.*?)\">(.*?)</invoke>", block, flags=re.IGNORECASE | re.DOTALL)
            for tool_name, params_block in invokes:
                args = {}
                params = re.findall(r"<parameter name=\"(.*?)\">(.*?)</parameter>", params_block, flags=re.IGNORECASE | re.DOTALL)
                for p_name, p_val in params:
                    args[p_name] = p_val.strip()
                
                parsed_calls.append({
                    "name": tool_name.strip(),
                    "args": args,
                    "id": f"minimax-{uuid.uuid4().hex[:12]}",
                    "type": "tool_call",
                })

        # 2. Handle [TOOL_CALL] custom pseudo-map style
        blocks = re.findall(r"\[TOOL_CALL\](.*?)\[/TOOL_CALL\]", source, flags=re.IGNORECASE | re.DOTALL)
        for block in blocks:
            tool_match = re.search(r"tool\s*=>\s*[\"']?([a-zA-Z0-9_.:-]+)", block, flags=re.IGNORECASE)
            if not tool_match:
                continue

            tool_name = tool_match.group(1).strip()
            args: dict[str, any] = {}

            # Extract common quoted key/value pairs first.
            for key in ("command", "file_path", "format", "base_name", "task", "work_dir", "number", "message", "sender"):
                value_match = re.search(
                    rf'"{key}"\s*:\s*"((?:\\.|[^"\\])*)"',
                    block,
                    flags=re.IGNORECASE | re.DOTALL,
                )
                if value_match:
                    args[key] = value_match.group(1)

            # Fallback attempt: convert pseudo-map syntax into JSON-like dict.
            if not args:
                args_match = re.search(r"args\s*=>\s*\{(.*)\}", block, flags=re.IGNORECASE | re.DOTALL)
                if args_match:
                    raw_args = args_match.group(1)
                    raw_args = re.sub(r"--[a-zA-Z0-9_-]+--", "", raw_args)
                    json_like = "{" + raw_args + "}"
                    json_like = json_like.replace("=>", ":")
                    try:
                        maybe_args = json.loads(json_like)
                        if isinstance(maybe_args, dict):
                            args = maybe_args
                    except json.JSONDecodeError:
                        args = {}

            parsed_calls.append(
                {
                    "name": tool_name,
                    "args": args,
                    "id": f"toolcall-{uuid.uuid4().hex[:12]}",
                    "type": "tool_call",
                }
            )

        return parsed_calls

    def __coerce_textual_tool_response(self, response: any):
        """Convert textual [TOOL_CALL] responses into structured tool calls."""
        if not isinstance(response, AIMessage):
            return response

        native_calls = getattr(response, "tool_calls", None) or []
        if native_calls:
            return response

        text_content = self.__content_to_text(response.content)
        parsed_calls = self.__extract_textual_tool_calls(text_content)
        if not parsed_calls:
            return response

        logger.warning("Assistant returned textual tool-call markup; coercing to structured tool call.")
        return AIMessage(content="", tool_calls=parsed_calls)

    @staticmethod
    def __coerce_bool(value: any) -> bool | None:
        """Convert mixed bool-like values to bool when possible."""
        if isinstance(value, bool):
            return value
        if isinstance(value, str):
            normalized = value.strip().lower()
            if normalized in {"true", "1", "yes"}:
                return True
            if normalized in {"false", "0", "no"}:
                return False
        return None

    def __parse_send_tool_payload(self, tool_text: str) -> dict:
        """Parse send tool output from JSON or repr-like text into dict."""
        text = (tool_text or "").strip()
        if not text:
            return {}

        try:
            payload = json.loads(text)
            if isinstance(payload, dict):
                return payload
        except json.JSONDecodeError:
            pass

        payload: dict[str, any] = {}

        # Common repr patterns from Pydantic/BaseModel string output.
        message_match = re.search(r"message[=:]\s*['\"]([^'\"]*)['\"]", text, flags=re.IGNORECASE)
        if message_match:
            payload["message"] = message_match.group(1).strip()

        status_match = re.search(r"status[=:]\s*['\"]?([a-zA-Z_]+)['\"]?", text, flags=re.IGNORECASE)
        if status_match:
            payload["status"] = status_match.group(1).strip().lower()

        success_match = re.search(r"success[=:]\s*(true|false)", text, flags=re.IGNORECASE)
        if success_match:
            payload["success"] = success_match.group(1).lower() == "true"

        on_hold_match = re.search(r"on_hold[=:]\s*(true|false)", text, flags=re.IGNORECASE)
        if on_hold_match:
            payload["on_hold"] = on_hold_match.group(1).lower() == "true"

        return payload

    def __format_send_tool_feedback(self, tool_name: str, tool_text: str) -> str:
        """Build concise user-facing text from send tool output."""
        payload = self.__parse_send_tool_payload(tool_text)
        message = str(payload.get("message") or "").strip()
        status = str(payload.get("status") or "").strip().lower()
        on_hold = self.__coerce_bool(payload.get("on_hold"))
        success = self.__coerce_bool(payload.get("success"))

        if on_hold is True or status == "on_hold":
            return message or "Your request is still on-hold and has not been sent yet."

        if success is True or status in {"sent", "success"}:
            if message:
                return message
            if tool_name == "send_file":
                return "File has been sent successfully."
            if tool_name == "send_message":
                return "Message has been sent successfully."
            return "Request completed successfully."

        if message:
            return message

        return (tool_text or "").strip()

    def __send_tool_outcome_kind(self, tool_text: str) -> str:
        """Classify send tool outcome to enforce deterministic user messaging."""
        raw_text = str(tool_text or "").strip()
        lowered = raw_text.lower()
        payload = self.__parse_send_tool_payload(tool_text)
        status = str(payload.get("status") or "").strip().lower()
        on_hold = self.__coerce_bool(payload.get("on_hold"))
        success = self.__coerce_bool(payload.get("success"))

        if on_hold is True or status == "on_hold":
            return "on_hold"

        if success is True or status in {"sent", "success"}:
            return "sent"

        if success is False or status in {"failed", "error", "invalid"}:
            return "failed"

        # Fallback heuristics for plain-text tool outputs that do not expose
        # structured fields (status/on_hold/success).
        if any(marker in lowered for marker in {
            "on-hold",
            "on_hold",
            "not been sent yet",
            "waiting for confirmation",
            "pending confirmation",
        }):
            return "on_hold"

        if any(marker in lowered for marker in {
            "failed to send",
            "error sending",
            "request failed",
            "status=failed",
            "status: failed",
        }):
            return "failed"

        if any(marker in lowered for marker in {
            "sent successfully",
            "has been sent",
            "status=sent",
            "status: sent",
        }):
            return "sent"

        return "unknown"

    def __latest_ai_tool_calls(self, state: State) -> list[dict]:
        """Get normalized tool calls from the most recent AI message."""
        for message in reversed(state.get("messages", [])):
            if not isinstance(message, AIMessage):
                continue

            direct_calls = getattr(message, "tool_calls", None) or []
            normalized_direct = [
                normalized
                for normalized in (self.__normalize_tool_call(call) for call in direct_calls)
                if normalized
            ]
            if normalized_direct:
                return normalized_direct

            additional_kwargs = getattr(message, "additional_kwargs", {}) or {}
            raw_calls = additional_kwargs.get("tool_calls") or []
            normalized_raw = [
                normalized
                for normalized in (self.__normalize_tool_call(call) for call in raw_calls)
                if normalized
            ]
            if normalized_raw:
                return normalized_raw

            return []

        return []

    def __tool_call_requires_confirmation(self, tool_call: dict) -> tuple[bool, str]:
        """Determine whether a tool call requires explicit human confirmation."""
        tool_name = str(tool_call.get("name") or "").strip()
        if not tool_name:
            return False, ""

        if tool_name in self._sensitive_tools:
            return True, "sensitive_action"

        decision_rule = self._need_decide.get(tool_name)
        if not decision_rule:
            return False, ""

        args_text = json.dumps(self.__coerce_tool_args(tool_call.get("args")), ensure_ascii=False).lower()
        markers = [str(marker).lower() for marker in decision_rule.get("sensitive", [])]
        if any(marker and marker in args_text for marker in markers):
            return True, "risky_command"

        return False, ""

    def __pending_confirmation_calls(self, state: State) -> list[tuple[dict, str]]:
        """Collect tool calls that should be gated by human confirmation."""
        flagged: list[tuple[dict, str]] = []
        for tool_call in self.__latest_ai_tool_calls(state):
            needs_confirmation, reason = self.__tool_call_requires_confirmation(tool_call)
            if needs_confirmation:
                flagged.append((tool_call, reason))
        return flagged

    def __human_feedback_reason_label(self, reason: str) -> str:
        if reason == "sensitive_action":
            return "This action is sensitive and requires explicit approval."

        if reason == "risky_command":
            return "Potentially risky/destructive command pattern detected."

        return reason or "Confirmation is required."

    def __human_feedback_action_label(self, tool_call: dict) -> str:
        """Build user-friendly action text for confirmation prompts."""
        tool_name = str(tool_call.get("name") or "").strip()
        args = self.__coerce_tool_args(tool_call.get("args"))

        if tool_name == self.filex_tool.delete_file_dir.__name__:
            target = str(args.get("file_path") or "").strip()
            return f"Delete file/folder: {target or 'unknown target'}"

        if tool_name == self.terminalx_tool.simple_exec.__name__:
            command = str(args.get("command") or "").strip()
            return f"Run terminal command: {command or 'unknown command'}"

        return "Run a sensitive action"

    def __build_human_feedback_message(self, flagged_calls: list[tuple[dict, str]]) -> str:
        """Create a deterministic confirmation message for sensitive actions."""
        lines = ["Before I run this, please confirm the following action:"]

        for index, (tool_call, reason) in enumerate(flagged_calls, start=1):
            lines.append(f"{index}. {self.__human_feedback_action_label(tool_call)}")
            if reason:
                reason_label = self.__human_feedback_reason_label(reason)
                lines.append(f"   Why confirmation is required: {reason_label}")

        lines.append("Should I continue?")
        lines.append("Reply with: continue / ok (approve) or cancel / no (reject).")
        return "\n".join(lines)

    @staticmethod
    def __requires_context_route(text: str) -> bool:
        """
        Force assistant route for status-oriented delivery follow-up queries.

        These intents rely on checkpointed history and tool outcomes, so they
        should not be handled by direct_chat's minimal context path.
        """
        normalized = (text or "").strip().lower()
        if not normalized:
            return False

        status_markers = (
            "sudah kirim",
            "udah kirim",
            "sudah terkirim",
            "belum terkirim",
            "status pesan",
            "pesan sudah",
            "on hold",
            "pending",
            "menunggu konfirmasi",
        )
        return any(marker in normalized for marker in status_markers)

    def __assistant_context(self, state: State, max_tokens: int | None = None) -> list:
        """
        Return a trimmed message window for the tool-capable assistant node.

        Limiting context size reduces token usage while keeping recent dialogue
        available for tool reasoning.
        """
        if max_tokens is None:
            max_tokens = self.ASSISTANT_CONTEXT_MAX_TOKENS

        messages = []
        for message in state.get("messages", []):
            if not isinstance(message, AIMessage):
                messages.append(message)
                continue

            content_text = self.__content_to_text(message.content)
            if self.__is_internal_system_log_text(content_text):
                continue

            messages.append(message)

        trimmed_messages = trim_messages(
            messages,
            max_tokens=max_tokens,
            strategy="last",
            token_counter=self.__token_counter,
            allow_partial=False,
        )

        summary = state.get("summary", "").strip()
        if summary:
            summary_message = SystemMessage(content=f"Conversation summary: {summary}")
            return [summary_message] + trimmed_messages

        return trimmed_messages

    @staticmethod
    def __has_non_system_message(messages: list) -> bool:
        """Check whether a message list includes at least one non-system message."""
        return any(not isinstance(message, SystemMessage) for message in messages)

    def __latest_non_system_message(self, state: State):
        """Return the latest message that is safe to include in assistant payload."""
        for message in reversed(state.get("messages", [])):
            if isinstance(message, SystemMessage):
                continue

            if isinstance(message, AIMessage):
                content_text = self.__content_to_text(message.content)
                if self.__is_internal_system_log_text(content_text):
                    continue

            return message

        return None

    def __latest_human_message(self, state: State):
        """Return the latest human message if available."""
        for message in reversed(state.get("messages", [])):
            if isinstance(message, HumanMessage):
                return message
        return None

    def __message_guard_payload(self, state: State, use_tools: bool) -> list:
        """
        Build model payload with a guaranteed non-system message.

        Some Ollama backends can fail when invoked with only system messages,
        which can happen after aggressive context trimming.
        """
        max_tokens = self.ASSISTANT_CONTEXT_MAX_TOKENS if use_tools else self.DIRECT_CHAT_CONTEXT_MAX_TOKENS
        assistant_messages = self.__assistant_context(state, max_tokens=max_tokens)

        if not use_tools:
            # Scrub tool calls and convert ToolMessages for plain-text LLMs.
            # Many providers (like Ollama) drop history or fail if tool_calls are
            # present but no tools are bound to the request.
            scrubbed_messages = []
            for msg in assistant_messages:
                if isinstance(msg, AIMessage):
                    tool_calls = getattr(msg, "tool_calls", [])
                    if tool_calls:
                        tool_names = [call.get("name", "unknown") for call in tool_calls]
                        content = self.__content_to_text(getattr(msg, "content", ""))
                        text = f"{content}\n[Executed tools: {', '.join(tool_names)}]" if content else f"[Executed tools: {', '.join(tool_names)}]"
                        scrubbed_messages.append(AIMessage(content=text.strip()))
                    else:
                        scrubbed_messages.append(msg)
                elif isinstance(msg, ToolMessage):
                    tool_name = getattr(msg, "name", "unknown")
                    content = self.__content_to_text(getattr(msg, "content", ""))
                    scrubbed_messages.append(SystemMessage(content=f"Tool '{tool_name}' result: {content}"))
                else:
                    scrubbed_messages.append(msg)
            assistant_messages = scrubbed_messages

        if not self.__has_non_system_message(assistant_messages):
            # Prefer latest human input to keep fallback context aligned
            # with the current turn and avoid older assistant echoes.
            fallback_message = self.__latest_human_message(state)
            if fallback_message is None:
                fallback_message = self.__latest_non_system_message(state)
            if fallback_message is not None:
                logger.warning(
                    "Assistant context trimmed to system-only for %s; injecting fallback message.",
                    state.get("sender", ""),
                )
                assistant_messages = assistant_messages + [fallback_message]

        payload = [self.__system_rule(use_tools=use_tools)]
        rag_context = str(state.get("rag_context") or "").strip()
        if rag_context:
            payload.append(SystemMessage(content=(
                "DOCUMENT CONTEXT:\n"
                "Use this uploaded document context when it is relevant to the user request. "
                "Prefer it over general knowledge, but ignore it if it is unrelated.\n\n"
                f"{rag_context}"
            )))

        return payload + assistant_messages
        
    def __save_process_diagram(self, react_graph) -> None:
        """
        Cache the current LangGraph process flow as Mermaid text.

        Mermaid text is safer than filesystem PNG outputs in Docker because it
        can be served directly through API without shared-volume coupling.
        """
        try:
            self._process_diagram_mermaid = react_graph.get_graph().draw_mermaid()
            logger.info(
                "Process diagram mermaid cached (%d chars)",
                len(self._process_diagram_mermaid),
            )
        except Exception as err:
            logger.warning("Failed to render process diagram mermaid: %s", err)
            self._process_diagram_mermaid = ""

    def get_process_diagram_mermaid(self) -> str:
        """Return cached process diagram Mermaid source text."""
        diagram = (self._process_diagram_mermaid or "").strip()
        if diagram:
            return diagram

        try:
            fallback = self.react_graph.get_graph().draw_mermaid()
        except Exception as err:
            logger.warning("Failed to build process diagram mermaid on demand: %s", err)
            return ""

        self._process_diagram_mermaid = fallback
        return fallback.strip()
    
    # Graph state routing (early filtering)
    def __route_query(self, state: State, max_latest_user_messages: int = 1) -> str:
        """
        Classify the next node at graph start.

        Routing happens before tool execution. direct_chat is the no-tool path,
        while assistant is the tool-enabled path. Ambiguous/uncertain requests
        must route to assistant for safety.

        Returns:
            str: direct_chat, assistant, human_feedback,
            confirm_sensitive_tools, or cancel_sensitive_tools.
        """
        sender = state.get("sender", "")
        # Use recent context for route classification so short replies (like typos) are understood
        recent_user_messages = self.__latest_user_text(state, max_messages=1).strip()
        recent_context = self.__fallback_state_excerpt(state.get("messages", []), max_messages=4, max_chars=1000)

        if self._pending_sensitive_tool_calls.get(sender):
            decision_text = self.__latest_user_text(state, max_messages=1)
            decision = self.__llm(reasoning=True,structured_output=FeedBackType).invoke(f"analyze the following user input ```{decision_text}```. return are in this inout, user agree or not, if agree return is_approved='approve', else return is_approved='cancel' ")

            if decision.is_approved == "approve":
                return "confirm_sensitive_tools"
            if decision.is_approved == "cancel":
                return "cancel_sensitive_tools"
            return "human_feedback"

        if not recent_user_messages:
            return "direct_chat"

        if self.__requires_context_route(recent_user_messages):
            return "assistant"

        classifier_prompt = self.instruction.route_query_instruction()
        custom_tools_catalog = self.tools_builder.custom_tools_catalog(agent_type=MAIN_AGENT)
        if custom_tools_catalog:
            classifier_prompt += (
                "\n\nCustom database tool routing rule:\n"
                "- If the latest user message matches any custom database tool name, callable tool name, "
                "or description below, return assistant.\n"
                "- Custom tools may provide information or execute configured code, so they require the tool-enabled path.\n\n"
                f"{custom_tools_catalog}"
            )

        try:
            classification = self.__llm(reasoning=self._reasoning).invoke([
                AIMessage(content=classifier_prompt),
                HumanMessage(content=recent_context),
            ])

            route = self.__content_to_text(classification.content).strip().lower()
            if route.startswith("direct_chat"):
                return "direct_chat"
            if route.startswith("assistant"):
                return "assistant"
        except Exception as err:
            logger.warning("Route classification failed, defaulting to assistant: %s", err)

        return "assistant"

    def __direct_chat(self, state: State):
        """
        Handle no-tool conversation with bounded multi-turn context.

        direct_chat can still see conversation history (trimmed by
        __assistant_context), but it does not bind tools.
        """
        sender = state.get("sender", "")
        payload = self.__message_guard_payload(state, use_tools=False)
        
        try:
            response = self.__llm(reasoning=self._reasoning).invoke(payload)
        except Exception as err:
            # Handle Ollama Service Unavailable (503) or other temporary API failures.
            logger.warning("Assistant direct_chat invoke failed with %s for %s; retrying with reasoning=%s.", err, sender, self._reasoning)
            try:
                # Fallback to a reasoning model or different endpoint.
                retry_llm = self.__llm(reasoning=self._reasoning)
                response = retry_llm.invoke(payload)
            except Exception as retry_err:
                logger.error("Fallback direct chat retry also failed for %s: %s", sender, retry_err)
                return {
                    "messages": [
                        AIMessage(content="Sorry, the service is temporarily unavailable. Please try again soon.")
                    ]
                }

        response_text = self.__content_to_text(getattr(response, "content", ""))
        if self.__looks_like_textual_tool_call(response_text):
            logger.warning(
                "direct_chat produced textual tool-call markup for %s; retrying plain-text response.",
                sender,
            )
            plain_text_guard = SystemMessage(
                content=(
                    "Tools are unavailable in this turn. "
                    "Do not emit tool-call syntax. "
                    "Reply with one concise user-facing plain-text answer only."
                )
            )

            try:
                retry_response = self.__llm(reasoning=False).invoke(payload + [plain_text_guard])
                retry_text = self.__content_to_text(getattr(retry_response, "content", ""))
                if not self.__looks_like_textual_tool_call(retry_text):
                    response = retry_response
                else:
                    response = AIMessage(
                        content="I need the tool-enabled path for this request. Please repeat your request."
                    )
            except Exception as retry_err:
                logger.warning("direct_chat plain-text retry failed for %s: %s", sender, retry_err)
                response = AIMessage(
                    content="I could not complete this in no-tool mode. Please repeat your request."
                )
        
        return {"messages": [response]}

    def __fallback_state_excerpt(
        self,
        graph_messages: list,
        max_messages: int = 10,
        max_chars: int = 3000,
    ) -> str:
        """Build a compact excerpt from the latest turn for fallback responses."""
        if not graph_messages:
            return ""

        latest_human_index = -1
        for index in range(len(graph_messages) - 1, -1, -1):
            if isinstance(graph_messages[index], HumanMessage):
                latest_human_index = index
                break

        latest_turn = graph_messages[latest_human_index:] if latest_human_index >= 0 else graph_messages
        latest_turn = latest_turn[-max_messages:]

        lines: list[str] = []
        for message in latest_turn:
            if isinstance(message, SystemMessage):
                continue

            role = "message"
            if isinstance(message, HumanMessage):
                role = "user"
            elif isinstance(message, AIMessage):
                role = "assistant"
            elif isinstance(message, ToolMessage):
                tool_name = str(getattr(message, "name", "") or "").strip()
                role = f"tool:{tool_name or 'unknown'}"

            text = self.__content_to_text(getattr(message, "content", ""))
            if isinstance(message, AIMessage) and self.__is_internal_system_log_text(text):
                continue

            if not text:
                if isinstance(message, AIMessage):
                    tool_calls = getattr(message, "tool_calls", None) or []
                    if tool_calls:
                        tool_names = [str(call.get("name") or "") for call in tool_calls if isinstance(call, dict)]
                        if tool_names:
                            lines.append(f"{role}: tool_calls={tool_names}")
                continue

            compact_text = " ".join(text.split())
            if len(compact_text) > 280:
                compact_text = compact_text[:277] + "..."

            lines.append(f"{role}: {compact_text}")

        excerpt = "\n".join(lines).strip()
        if len(excerpt) <= max_chars:
            return excerpt

        return excerpt[-max_chars:]
    
    def __init_langsmith(self):
        """
        Configure LangSmith tracing environment variables when a key is present.

        Keeping this setup isolated avoids side effects in other methods and
        makes startup behavior predictable across local and production runs.
        """
        langsmith_key = self.config.get_langsmith_config().api_key
        if langsmith_key:
            os.environ['LANGSMITH_API_KEY'] = langsmith_key
            os.environ['LANGSMITH_TRACING'] = 'true'
            os.environ["LANGSMITH_PROJECT"] = self.config.get_langsmith_config().project_name or "default"

    @staticmethod
    def __tool_object_name(tool: Any) -> str:
        return str(getattr(tool, "name", None) or getattr(tool, "__name__", "") or "").strip()

    def __tool_cache_key(self, state: State | None) -> str:
        sender = state.get("sender", "") if state else ""
        custom_signature = self.tools_builder.catalog_signature(agent_type=MAIN_AGENT)
        return f"{sender}:{custom_signature}"

    def __get_tools(self, state: State | None = None) -> list:
        sender = state.get("sender", "") if state else ""
        
        dynamic_tools = [
            # dynamic tool that needs access to state (sender) must be generated here
            self.websearch_tool.websearch_tool(sender=sender),
            self.db_access_tool.get_current_pending_message(sender=sender),
            self.db_access_tool.get_historical_pending_messages(sender=sender),
            self.wa_integration_tool.send_message(sender=sender),
            self.wa_integration_tool.send_file(sender=sender),
            self.agent_code.make_agent_code_tool(sender=sender),
            self.agent_reaserch.make_agent_reaserch_tool(sender=sender),
            self.terminalx_tool.simple_exec(sender=sender),
            self.filex_tool.generate_images(sender=sender)
        ]
        
        custom_tool_catalog = self.tools_builder.get_tools_builder_list(agent_type=MAIN_AGENT)
        if custom_tool_catalog:
            reserved_names = {
                name
                for name in (
                    self.__tool_object_name(tool)
                    for tool in [*self.__static_tools, *dynamic_tools]
                )
                if name
            }
            dynamic_tools.extend(
                self.tools_builder.make_custom_database_tools(
                    agent_type=MAIN_AGENT,
                    reserved_names=reserved_names,
                )
            )
        
        return self.__static_tools + dynamic_tools
    
    async def __tool_node(self, state: State):
        cache_key = self.__tool_cache_key(state)
        
        if cache_key not in self._tool_node_cache:
            tools = self.__get_tools(state)
            self._tool_node_cache[cache_key] = ToolNode(tools)
        
        return await self._tool_node_cache[cache_key].ainvoke(state)

    def __assistant(self, state: State):
        """
        Run the tool-capable assistant node with bounded conversation context.

        The system rule is prepended each turn, while prior messages are
        trimmed to control token growth and keep reasoning focused.
        """
        sender = state.get("sender", "")
        cache_key = self.__tool_cache_key(state)
    
        # ambil dari cache, kalau belum ada baru bind
        if cache_key not in self._llm_cache:
            tools = self.__get_tools(state)
            self._llm_cache[cache_key] = self.__llm(reasoning=self._reasoning).bind_tools(tools)
        
        llm_with_tools = self._llm_cache[cache_key]
        payload = self.__message_guard_payload(state, use_tools=True)

        if not self.__has_non_system_message(payload):
            logger.warning("Assistant payload is system-only for %s; returning safe fallback response.", sender)
            return {"messages": [AIMessage(content="Sorry, conversation context is empty. Please resend your message.")]}

        try:
            response = llm_with_tools.invoke(payload)
            response = self.__coerce_textual_tool_response(response)
        except Exception as err:
            if err.__class__.__name__ != "ResponseError":
                raise

            logger.warning(
                "Assistant invoke failed with ResponseError for %s; retrying with reasoning=%s. detail=%s",
                sender,
                self._reasoning,
                err,
            )

            retry_llm = self.__llm(reasoning=self._reasoning).bind_tools(self.__get_tools(state))
            response = retry_llm.invoke(payload)
            response = self.__coerce_textual_tool_response(response)

        return {"messages": [response]}

    def __system_rule(self, use_tools: bool = True):
        """
        Build the system instruction message for the current runtime mode.

        This keeps the policy source centralized so updates are applied
        consistently on every assistant turn.
        """
        build_msg = "SYSTEM RULES AND INSTRUCTIONS:\n" + self.instruction.system_instruction
        
        runtime_mode = "assistant" if use_tools else "direct_chat"
        build_msg += f"\n\nRUNTIME MODE: {runtime_mode}"
        if use_tools:
            build_msg += "\n- Tools are available for this turn. Use them when required by the task."
        else:
            build_msg += "\n- Tools are NOT available for this turn. Solve using reasoning and context only."

        if use_tools:
            custom_tools_catalog = self.tools_builder.custom_tools_catalog(agent_type=MAIN_AGENT)
            if custom_tools_catalog:
                build_msg += "\n\n" + custom_tools_catalog
                build_msg += (
                    "\n\nCUSTOM TOOL USAGE:"
                    "\n- Match the user's request against each custom tool description."
                    "\n- If there is a clear match, call the listed callable_tool_name directly."
                    "\n- If a provider cannot call that native custom tool name, call `access_tools_builder` with `tool_name` set to original_name."
                    "\n- For information-only custom tools, call the tool and use its returned information; do not guess missing facts."
                    "\n- For code-backed custom tools, pass `tool_input` when the custom code needs data from the user request."
                    "\n- After a code-backed custom tool returns, preserve its CUSTOM_TOOL_OUTPUT_BEGIN/CUSTOM_TOOL_OUTPUT_END content in the final answer."
                    "\n- Final answer format for code-backed custom tools: `output:` then `---`, the captured output, `---`, then a short description."
                    "\n- Do not dismiss decorative, ASCII-art, animation-frame, or repeated-line custom tool output as meaningless; describe what it shows."
                )

            capability_description = self.instruction.capability_description()
            if capability_description:
                build_msg += "\n\nCAPABILITY REFERENCE:\n" + capability_description
            build_msg += "\n\nCOMMANDS: Use /help for command list."

        sys_msg = SystemMessage(content=build_msg)
        return sys_msg
    
    def __human_feedback(self, state: State):
        """Ask for confirmation before executing sensitive/risky tool calls."""
        sender = state.get("sender", "")

        latest_tool_calls = self.__latest_ai_tool_calls(state)
        flagged_calls = self.__pending_confirmation_calls(state)

        if latest_tool_calls and flagged_calls:
            self._pending_sensitive_tool_calls[sender] = latest_tool_calls
        else:
            cached_calls = self._pending_sensitive_tool_calls.get(sender, [])
            flagged_calls = []
            for tool_call in cached_calls:
                needs_confirmation, reason = self.__tool_call_requires_confirmation(tool_call)
                if needs_confirmation:
                    flagged_calls.append((tool_call, reason))

        if not flagged_calls:
            self._pending_sensitive_tool_calls.pop(sender, None)
            self._human_interrup_chche.pop(sender, None)
            return {
                "messages": [
                    AIMessage(
                        content="There is no sensitive action waiting for confirmation."
                    )
                ]
            }

        feedback_message = self.__build_human_feedback_message(flagged_calls)
        self._human_interrup_chche[sender] = feedback_message
        return {"messages": [AIMessage(content=feedback_message)]}

    def __confirm_sensitive_tools(self, state: State):
        """Resume execution by replaying previously approved tool calls."""
        sender = state.get("sender", "")
        pending_calls = self._pending_sensitive_tool_calls.pop(sender, [])
        self._human_interrup_chche.pop(sender, None)

        if not pending_calls:
            return {
                "messages": [
                    AIMessage(
                        content="There is no sensitive action waiting for confirmation."
                    )
                ]
            }

        normalized_calls = [
            normalized
            for normalized in (self.__normalize_tool_call(call) for call in pending_calls)
            if normalized
        ]
        if not normalized_calls:
            return {
                "messages": [
                    AIMessage(
                        content="Confirmation received, but the sensitive action is invalid."
                    )
                ]
            }

        return {"messages": [AIMessage(content="", tool_calls=normalized_calls)]}

    def __cancel_sensitive_tools(self, state: State):
        """Cancel pending sensitive execution after human rejection."""
        sender = state.get("sender", "")
        self._pending_sensitive_tool_calls.pop(sender, None)
        self._human_interrup_chche.pop(sender, None)
        return {
            "messages": [
                AIMessage(content="Okay, sensitive action canceled.")
            ]
        }

    def __agent_logic(self):
        """
        Build and compile the LangGraph workflow.

        Flow overview:
        - START -> route_query (early intent filter)
        - route_query -> direct_chat or assistant
        - assistant -> tools only when tool calls are requested
        - tools -> assistant loop until no more tool calls

        Why this design:
        - direct_chat handles turns that can be solved without tools
        - assistant path handles tool-enabled tasks and uncertain cases
        - tools are executed only on explicit tool-call outputs
        """
        # Build the state graph container.
        builder = StateGraph(State)
        
        

        # Register worker nodes.
        builder.add_node("retrieve", self.__retrieve)
        builder.add_node("summarize_conversation", self.__summarize_conversation)
        
        # assistant decides whether tools are needed; tools executes tool calls.
        builder.add_node("pre_assistant", self.__pre_assistant)
        builder.add_node("assistant", self.__assistant)
        builder.add_node("direct_chat", self.__direct_chat)
        builder.add_node("human_feedback", self.__human_feedback)
        builder.add_node("confirm_sensitive_tools", self.__confirm_sensitive_tools)
        builder.add_node("cancel_sensitive_tools", self.__cancel_sensitive_tools)
        # ToolNode runs only the configured tool functions.
        builder.add_node("tools", self.__tool_node)
        
        # Route from START using early filtering.
        builder.add_edge(START, "retrieve")
        builder.add_conditional_edges(
            "retrieve",
            self.__route_query,
            {
                "direct_chat": "direct_chat",  # No-tool path.
                "assistant": "pre_assistant",   # Tool-enabled path.
                "human_feedback": "human_feedback",
                "confirm_sensitive_tools": "confirm_sensitive_tools",
                "cancel_sensitive_tools": "cancel_sensitive_tools",
            }
        )

        builder.add_edge("human_feedback", END)
        builder.add_edge("cancel_sensitive_tools", END)
        builder.add_edge("confirm_sensitive_tools", "tools")

        # Before every assistant run, decide whether we should summarize first.
        builder.add_conditional_edges(
            "pre_assistant",
            self.__should_continue,
            {
                "summarize_conversation": "summarize_conversation",
                "assistant": "assistant",
            }
        )
        
        builder.add_conditional_edges(
            "assistant",
            self.__route_after_assistant,
            {
                "tools": "tools",
                "human_feedback": "human_feedback",
                "end": END,
            }
        )

        # Re-check summarize gate before re-entering assistant after tools.
        builder.add_edge("tools", "pre_assistant")
        # After summarization, continue through assistant with compact state.
        builder.add_edge("summarize_conversation", "assistant")
        react_graph = builder.compile(checkpointer=self.checkpointer)
        self.__save_process_diagram(react_graph)
        
        return react_graph

    def extract_ai_content(self, messages: list) -> str:
        """
        Extract the latest non-empty assistant text from graph message state.

        Tool loops can produce multiple AI messages, including empty/tool-call
        placeholders. The extractor only considers AI messages created after
        the latest HumanMessage to avoid leaking stale replies from older turns.
        """
        if not messages:
            return ''

        latest_human_index = -1
        for index in range(len(messages) - 1, -1, -1):
            if isinstance(messages[index], HumanMessage):
                latest_human_index = index
                break

        current_turn_messages = messages[latest_human_index + 1:] if latest_human_index >= 0 else messages

        # run_worker may run in blocking or background mode.
        # If background dispatch text is present, return it directly to avoid
        # post-tool LLM paraphrases that can overclaim completion.
        run_worker_tool_fallback = ""
        send_tool_fallback = ""
        send_tool_outcome = "unknown"
        for tool_message in reversed(current_turn_messages):
            if not isinstance(tool_message, ToolMessage):
                continue

            tool_name = str(getattr(tool_message, "name", "") or "").strip().lower()
            tool_text = self.__content_to_text(getattr(tool_message, "content", ""))
            if not tool_text:
                continue

            if tool_name in {"send_file", "send_message", "broadcast_message"} and not send_tool_fallback:
                send_tool_fallback = self.__format_send_tool_feedback(tool_name, tool_text)
                send_tool_outcome = self.__send_tool_outcome_kind(tool_text)

            if tool_name != "run_worker":
                continue

            if "dispatched in the background" in tool_text.lower():
                return tool_text

            # Keep as fallback only when assistant has no final message.
            run_worker_tool_fallback = tool_text
            break

        ai_messages = [message for message in current_turn_messages if isinstance(message, AIMessage)]
        if not ai_messages:
            if run_worker_tool_fallback:
                return run_worker_tool_fallback
            if send_tool_fallback:
                return send_tool_fallback
            return ''

        # Preserve true delivery state from tool output when request is not sent yet.
        if send_tool_fallback and send_tool_outcome in {"on_hold", "failed"}:
            return send_tool_fallback

        for ai_message in reversed(ai_messages):
            content = ai_message.content
            if isinstance(content, str):
                text = content.strip()
                if self.__is_internal_system_log_text(text):
                    continue
                if self.__looks_like_textual_tool_call(text):
                    continue
                if text:
                    return text
                continue

            if isinstance(content, list):
                text_parts = []
                for part in content:
                    if isinstance(part, str):
                        text_parts.append(part)
                        continue

                    if isinstance(part, dict):
                        text = part.get('text')
                        if text:
                            text_parts.append(text)

                text = '\n'.join(text_parts).strip()
                if self.__is_internal_system_log_text(text):
                    continue
                if self.__looks_like_textual_tool_call(text):
                    continue
                if text:
                    return text
                continue

            text = str(content).strip()
            if self.__is_internal_system_log_text(text):
                continue
            if self.__looks_like_textual_tool_call(text):
                continue
            if text and text != '[]':
                return text

        if run_worker_tool_fallback:
            return run_worker_tool_fallback

        if send_tool_fallback:
            return send_tool_fallback

        return ''

    def __command_guard(self, text: str) -> str:
        """
        Heuristic guard to prevent tool-call-looking text from being treated as final output.

        Some models can produce tool calls in text form instead of structured fields,
        which can cause confusion in the output state. This guard checks for telltale
        patterns to avoid misinterpreting tool call descriptions as user-facing replies.
        """
        if text.startswith("/"): 
            if text.count(" ") > 0:
                text = text.split(" ")[0]
            
            commands = self.wa_integration_tool.get_all_commands()
            for command in commands:
                if text.strip() == command:
                    return ""
            return "Unknown command. Type /help to see available commands."
        return ""
    
    async def ChatAgent(
            self, 
            messages: str,
            number: str,
            is_system=False,
            level: LevelSystem=LevelSystem.INFO, 
            group_id: str=None,
            reply_content: str=None,
            image: str=None, 
        ) -> None:
        send_to = number
        if group_id:
            send_to = group_id
        try:
            """
            Execute one chat turn through the compiled graph for one user thread.

            The method injects the user identifier as a system context message,
            appends the latest human message, then invokes the graph with thread_id
            so conversation memory remains isolated per user.

            Runtime behavior:
            - route_query performs early filtering (direct_chat vs assistant)
            - direct_chat uses only the latest user message
            - assistant may call tools, then loops until final AI response

            Args:
                messages: Latest user input text.
                number: User identifier used for thread scoping and WhatsApp tools.

            Returns:
                str: Final assistant text extracted from graph output state.
            """
            if isinstance(level, str):
                level_name = level.strip().upper()
                level = LevelSystem.__members__.get(level_name, LevelSystem.INFO)
            elif not isinstance(level, LevelSystem):
                level = LevelSystem.INFO
            
            command_response = self.__command_guard(messages)
            if command_response != "":
                self.wa_integration_tool.request_send_message(send_to, command_response, send_to)
                return

            input_messages = []
            config = {"configurable": {"thread_id": group_id if group_id else number}}
            
            
            if is_system:
                self.db_access_tool.db_set_log_system(LogSystemEntry(
                    timestamp=self.time_tool.current_time(),
                    level=level.value,
                    message=messages,
                    metadata={"image": image} if image else None
                ))
                self.react_graph.update_state(
                    config,
                    {
                        "messages": [
                            AIMessage(content=f"[SYSTEMEVENT] {level.value} log from {number} at {self.time_tool.current_time()}: {messages}")
                        ]
                    }
                )
                return
            
            contact_info = self.wa_integration_tool.get_user_information(number)
            if reply_content:
                input_messages.append(AIMessage(content=f"User Replying Message from AI:\n{reply_content}"))
            input_messages.append(HumanMessage(content=f"User information: {contact_info.data.model_dump() if contact_info.data else 'N/A'}\nSend at {self.time_tool.current_time()}. Message: {messages}"))

            result = await self.react_graph.ainvoke({
                "messages": input_messages,
                "sender": group_id if group_id else number,
                "rag_query": messages,
                "rag_context": "",
            }, config=config)
            
            results_message = self.extract_ai_content(result.get('messages', []))

            # Some reasoning/tool-enabled model runs can end with empty text.
            # Retry once with a plain no-reasoning completion to avoid blank or stale replies.
            if not results_message:
                err = "Assistant returned empty content for '%s'; running fallback completion."
                logger.warning(err, number)

                latest_user_message = (messages or "").strip()
                state_excerpt = self.__fallback_state_excerpt(
                    result.get("messages", []),
                    max_messages=10,
                    max_chars=3000,
                )

                message_sys = (
                    "The previous run produced no final user-visible response.\n"
                    "Focus only on the latest user request and ignore stale earlier turns.\n\n"
                    f"Latest user request:\n{latest_user_message or '(empty)'}\n\n"
                    "Latest-turn state excerpt:\n"
                    f"{state_excerpt or '(empty)'}\n\n"
                    "Generate one concise user-facing reply in plain text."
                )
                results = ""
                try:
                    fallback_response = self.__llm(reasoning=False).invoke([
                        SystemMessage(content=message_sys),
                        HumanMessage(content=latest_user_message or "Please answer the latest user request."),
                    ])
                    results = self.__content_to_text(getattr(fallback_response, "content", ""))
                except Exception as fallback_err:
                    logger.warning("Fallback completion failed for %s: %s", number, fallback_err)
                    if fallback_err.__class__.__name__ == "ResponseError" and "503" in str(fallback_err):
                        results = "Sorry, model service is temporarily unavailable (503). Please retry in a moment."

                if not results:
                    results = "Sorry, I couldn't generate a response yet. Please resend your question."
                    self.db_access_tool.db_set_log_system(LogSystemEntry(
                        timestamp=self.time_tool.current_time(),
                        level=LevelSystem.ERROR.value,
                        message=f"Message processed for {number} failed to generate response"
                    ))
                
                self.wa_integration_tool.request_send_message(send_to, results, send_to)
                return

            self.wa_integration_tool.request_send_message(send_to, results_message, send_to)
        except Exception as e:
            logger.exception("ChatAgent failed for %s", number)
            self.db_access_tool.db_set_log_system(LogSystemEntry(
                timestamp=self.time_tool.current_time(),
                level=LevelSystem.ERROR.value,
                message=f"Error occurred while processing message for {number}: {str(e)}",
            ))
            self.react_graph.update_state(
                    config,
                    {
                        "messages": [
                            AIMessage(content=f"[SYSTEMEVENT] {LevelSystem.ERROR.value} log from {number} at {self.time_tool.current_time()}: Error occurred while processing message for {number}: {str(e)}")
                        ]
                    }
                )
            self.wa_integration_tool.request_send_message(
                send_to,
                f"😭 Sorry, an internal error occurred while processing your message. Please try again\nDetail: {str(e)}",
                send_to,
            )
