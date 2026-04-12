import logging
import json
import os
import re
import uuid

from langchain_core import messages
from numpy import number

from backend.application.ai.ai import AI
from backend.application.ai.tools.websearch import WebSearchTool
from backend.application.ai.tools.time import TimeTool
from backend.application.ai.tools.parse import ParseTool

from backend.application.ai.tools.whatsapp_integration import WhatsAppIntegration
from backend.application.ai.instruction import Instruction
from backend.infrastructure.database.database import MongoDB
from backend.application.ai.tools.dbaccess import DBAcess
from backend.application.ai.tools.models import InputHistory, LevelSystem, LogSystemEntry
from backend.application.ai.tools.terimnalx import TerminalX
from backend.application.ai.tools.filex import Filex
from config.config import Config

from langgraph.graph import StateGraph, START, END
from langgraph.graph import MessagesState
from langgraph.prebuilt import ToolNode
from langchain_core.messages import HumanMessage, AIMessage, SystemMessage, RemoveMessage

from langgraph.checkpoint.mongodb import MongoDBSaver
from langchain_core.messages import trim_messages
from typing_extensions import Literal


logger = logging.getLogger('uvicorn.error')

class State(MessagesState):
    summary: str
    sender: str

class ChatAgentUseCase:
    ASSISTANT_CONTEXT_MAX_TOKENS = 32000
    SUMMARIZE_TRIGGER_TOKENS = 40000
    SUMMARIZE_MIN_MESSAGES = 20
    SUMMARIZE_KEEP_RECENT_MESSAGES = 20
    SUMMARIZE_SOURCE_MAX_TOKENS = 40000

    def __init__(self):
        self.llm = AI()
        self.websearch_tool = WebSearchTool()
        self.time_tool = TimeTool()
        self.parse_tool = ParseTool()
        self.wa_integration_tool = WhatsAppIntegration()
        self.db_access_tool = DBAcess()
        self.terminalx_tool = TerminalX()
        self.filex_tool = Filex()
        self.instruction = Instruction()
        self._command_aliases = self.__build_command_aliases(self.instruction.chatbot_commands)
        self.mongo_cleint = MongoDB.get_client()
        self.checkpointer = MongoDBSaver(self.mongo_cleint, db_name="agent_memory_db", collection_name="agent_memory_collection")
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
            self.websearch_tool.websearch_tool(),
            self.time_tool.current_time,
            self.parse_tool.parse_phone_number_tool(),
            self.wa_integration_tool.get_user_information,
            self.wa_integration_tool.broadcast_message,
            self.filex_tool.compress,
            self.filex_tool.delete_file_dir,
        ]
        self._llm_cache: dict[str, any] = {}
        self._tool_node_cache: dict[str, ToolNode] = {}
        self._human_interrup_chche: dict[str, str] = {} # user number -> interruption message
        self._pending_sensitive_tool_calls: dict[str, list[dict]] = {}
        self._process_diagram_mermaid = ""
    
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
        response = self.__llm(reasoning=False).invoke(messages)
        
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
    
    def __llm(self, reasoning: bool = True):
        return self.llm.LLM(reasoning=reasoning)

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

    @staticmethod
    def __build_command_aliases(command_text: str) -> dict[str, str]:
        """
        Build a command lookup map from markdown command documentation.

        The map accepts both slash form (`/help`) and bare form (`help`) and
        resolves both into a canonical slash command value.
        """
        aliases: dict[str, str] = {}
        for command in re.findall(r"/[a-zA-Z0-9_-]+", command_text or ""):
            canonical = command.lower()
            aliases[canonical] = canonical
            bare = canonical.lstrip("/")
            if bare:
                aliases[bare] = canonical

        return aliases

    def __match_command_intent(self, text: str) -> tuple[bool, str, bool]:
        """
        Detect command-like input, including malformed prefix attempts.

        Returns:
            tuple[bool, str, bool]:
            - matched flag
            - canonical command (e.g. /help)
            - malformed flag (True when missing/wrong prefix)
        """
        if not self._command_aliases:
            return False, "", False

        user_text = (text or "").strip()
        if not user_text:
            return False, "", False

        parts = user_text.split()
        if not parts:
            return False, "", False

        first = parts[0].strip().lower()
        if not first:
            return False, "", False

        punctuation = ".,!?;:"

        if first.startswith("/"):
            candidate = first.rstrip(punctuation)
            canonical = self._command_aliases.get(candidate)
            if canonical:
                return True, canonical, False
            return False, "", False

        malformed_prefixes = ("!", ".", "#", "$")
        if first.startswith(malformed_prefixes):
            normalized = first[1:].strip().rstrip(punctuation)
            candidate = f"/{normalized}" if normalized else ""
            canonical = self._command_aliases.get(candidate)
            if canonical:
                return True, canonical, True
            return False, "", False

        # Bare command words are treated as command intent only if standalone,
        # so normal sentences such as "help saya ..." are not hijacked.
        if len(parts) == 1:
            candidate = f"/{first.rstrip(punctuation)}"
            canonical = self._command_aliases.get(candidate)
            if canonical:
                return True, canonical, True

        return False, "", False

    def __command_guard_message(self, detected_command: str, malformed: bool) -> str:
        """Return a deterministic command-help response for command-like input."""
        command_menu = (self.instruction.chatbot_commands or "").strip()

        if malformed:
            intro = (
                "I detected a command attempt with invalid format. "
                "Use slash prefix for commands."
            )
        else:
            intro = "I detected command context in your message."

        response_lines = [
            intro,
            f"Try this command: {detected_command}",
            "Use /help to see the complete command guide.",
        ]

        if command_menu:
            response_lines.append("\nAvailable command reference:\n" + command_menu)

        return "\n".join(response_lines)

    def __command_guard(self, state: State):
        """Handle command-like input without entering tool execution loops."""
        latest_text = self.__latest_user_text(state)
        matched, detected_command, malformed = self.__match_command_intent(latest_text)
        if not matched:
            guard_reply = "Use /help to see available commands."
        else:
            guard_reply = self.__command_guard_message(detected_command, malformed)

        return {"messages": [AIMessage(content=guard_reply)]}

    def __latest_user_text(self, state: State, max_messages: int = 1) -> str:
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
            return True, "Sensitive action requires explicit confirmation."

        decision_rule = self._need_decide.get(tool_name)
        if not decision_rule:
            return False, ""

        args_text = json.dumps(self.__coerce_tool_args(tool_call.get("args")), ensure_ascii=False).lower()
        markers = [str(marker).lower() for marker in decision_rule.get("sensitive", [])]
        if any(marker and marker in args_text for marker in markers):
            return True, "Potentially risky command pattern detected."

        return False, ""

    def __pending_confirmation_calls(self, state: State) -> list[tuple[dict, str]]:
        """Collect tool calls that should be gated by human confirmation."""
        flagged: list[tuple[dict, str]] = []
        for tool_call in self.__latest_ai_tool_calls(state):
            needs_confirmation, reason = self.__tool_call_requires_confirmation(tool_call)
            if needs_confirmation:
                flagged.append((tool_call, reason))
        return flagged

    @staticmethod
    def __human_feedback_intent(text: str) -> Literal["approve", "reject", "unknown"]:
        """Interpret human confirmation response for pending sensitive actions."""
        normalized = " ".join((text or "").strip().lower().split())
        if not normalized:
            return "unknown"

        reject_markers = (
            "batal",
            "cancel",
            "jangan",
            "tidak",
            "no",
            "stop",
            "ga jadi",
            "gak jadi",
            "nggak jadi",
        )
        if any(marker in normalized for marker in reject_markers):
            return "reject"

        approve_markers = (
            "lanjut",
            "setuju",
            "yes",
            "ya",
            "ok",
            "oke",
            "confirm",
            "konfirmasi",
            "proceed",
        )
        if any(marker in normalized for marker in approve_markers):
            return "approve"

        return "unknown"

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
        lines = [
            "Saya butuh konfirmasi sebelum menjalankan aksi sensitif ini:",
        ]

        for index, (tool_call, reason) in enumerate(flagged_calls, start=1):
            lines.append(f"{index}. {self.__human_feedback_action_label(tool_call)}")
            if reason:
                lines.append(f"   Reason: {reason}")

        lines.append("Balas 'lanjut' untuk menyetujui, atau 'batal' untuk membatalkan.")
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

    def __assistant_context(self, state: State) -> list:
        """
        Return a trimmed message window for the tool-capable assistant node.

        Limiting context size reduces token usage while keeping recent dialogue
        available for tool reasoning.
        """
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
            max_tokens=self.ASSISTANT_CONTEXT_MAX_TOKENS,
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

    def __message_guard_payload(self, state: State, use_tools: bool) -> list:
        """
        Build model payload with a guaranteed non-system message.

        Some Ollama backends can fail when invoked with only system messages,
        which can happen after aggressive context trimming.
        """
        assistant_messages = self.__assistant_context(state)

        if not self.__has_non_system_message(assistant_messages):
            fallback_message = self.__latest_non_system_message(state)
            if fallback_message is not None:
                logger.warning(
                    "Assistant context trimmed to system-only for %s; injecting fallback message.",
                    state.get("sender", ""),
                )
                assistant_messages = assistant_messages + [fallback_message]

        return [self.__system_rule(use_tools=use_tools)] + assistant_messages
        
    # only use for testing (skripsi)
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
    def __route_query(self, state: State, max_latest_user_messages: int = 5) -> str:
        """
        Classify the next node at graph start.

        Routing happens before tool execution. direct_chat is the no-tool path,
        while assistant is the tool-enabled path. Ambiguous/uncertain requests
        must route to assistant for safety.

        Returns:
            str: command_guard, direct_chat, assistant, human_feedback,
            confirm_sensitive_tools, or cancel_sensitive_tools.
        """
        sender = state.get("sender", "")
        recent_user_messages = self.__latest_user_text(state, max_messages=max_latest_user_messages).strip()

        if self._pending_sensitive_tool_calls.get(sender):
            decision_text = self.__latest_user_text(state, max_messages=1)
            decision = self.__human_feedback_intent(decision_text)
            if decision == "approve":
                return "confirm_sensitive_tools"
            if decision == "reject":
                return "cancel_sensitive_tools"
            return "human_feedback"

        if not recent_user_messages:
            return "direct_chat"

        if self.__requires_context_route(recent_user_messages):
            return "assistant"

        command_intent, _, _ = self.__match_command_intent(recent_user_messages)
        if command_intent:
            return "command_guard"

        classifier_prompt = self.instruction.route_query_instruction()

        try:
            classification = self.__llm(reasoning=True).invoke([
                SystemMessage(content=classifier_prompt),
                HumanMessage(content=recent_user_messages),
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
        payload = self.__message_guard_payload(state, use_tools=False)
        response = self.__llm(reasoning=False).invoke(payload)
        
        return {"messages": [response]}
    
    def __init_langsmith(self):
        """
        Configure LangSmith tracing environment variables when a key is present.

        Keeping this setup isolated avoids side effects in other methods and
        makes startup behavior predictable across local and production runs.
        """
        if Config.LANGSMITH_API_KEY:
            os.environ['LANGSMITH_API_KEY'] = Config.LANGSMITH_API_KEY
            os.environ['LANGSMITH_TRACING'] = 'true'
            os.environ["LANGSMITH_PROJECT"] = Config.LANGSMITH_PROJECT_NAME
    
    def __get_tools(self, state: State = "") -> list:
        sender = state.get("sender", "")
        
        return self.__static_tools + [
            # dynamic tool that needs access to state (sender) must be generated here
            self.db_access_tool.get_current_pending_message(sender=sender),
            self.db_access_tool.get_historical_pending_messages(sender=sender),
            self.wa_integration_tool.send_message(sender=sender),
            self.wa_integration_tool.send_file(sender=sender),
            self.terminalx_tool.delegate_to_worker(sender=sender),
            self.terminalx_tool.simple_exec(sender=sender),
        ]
    
    async def __tool_node(self, state: State):
        sender = state.get("sender", "")
        
        if sender not in self._tool_node_cache:
            tools = self.__get_tools(state)
            self._tool_node_cache[sender] = ToolNode(tools)
        
        return await self._tool_node_cache[sender].ainvoke(state)

    def __assistant(self, state: State):
        """
        Run the tool-capable assistant node with bounded conversation context.

        The system rule is prepended each turn, while prior messages are
        trimmed to control token growth and keep reasoning focused.
        """
        sender = state.get("sender", "")
    
        # ambil dari cache, kalau belum ada baru bind
        if sender not in self._llm_cache:
            tools = self.__get_tools(state)
            self._llm_cache[sender] = self.__llm(reasoning=True).bind_tools(tools)
        
        llm_with_tools = self._llm_cache[sender]
        payload = self.__message_guard_payload(state, use_tools=True)

        if not self.__has_non_system_message(payload):
            logger.warning("Assistant payload is system-only for %s; returning safe fallback response.", sender)
            return {"messages": [AIMessage(content="Maaf, konteks percakapan kosong. Tolong kirim ulang pesan Anda.")]}

        try:
            response = llm_with_tools.invoke(payload)
        except Exception as err:
            if err.__class__.__name__ != "ResponseError":
                raise

            logger.warning(
                "Assistant invoke failed with ResponseError for %s; retrying with reasoning=True. detail=%s",
                sender,
                err,
            )
            retry_llm = self.__llm(reasoning=True).bind_tools(self.__get_tools(state))
            response = retry_llm.invoke(payload)

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

        capability_description = self.instruction.capability_description()
        if capability_description:
            build_msg += "\n\nCAPABILITY REFERENCE:\n" + capability_description

        command_reference = (self.instruction.chatbot_commands or "").strip()
        if command_reference:
            build_msg += "\n\nCHATBOT COMMAND REFERENCE:\n" + command_reference

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
                    AIMessage(content="Tidak ada aksi sensitif yang menunggu konfirmasi.")
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
                    AIMessage(content="Tidak ada aksi sensitif yang menunggu konfirmasi.")
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
                    AIMessage(content="Konfirmasi diterima, tetapi aksi sensitif tidak valid.")
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
                AIMessage(content="Baik, aksi sensitif dibatalkan.")
            ]
        }

    def __agent_logic(self):
        """
        Build and compile the LangGraph workflow.

        Flow overview:
        - START -> route_query (early intent filter)
        - route_query -> command_guard, direct_chat, or assistant
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
        builder.add_node("summarize_conversation", self.__summarize_conversation)
        
        # assistant decides whether tools are needed; tools executes tool calls.
        builder.add_node("pre_assistant", self.__pre_assistant)
        builder.add_node("assistant", self.__assistant)
        builder.add_node("direct_chat", self.__direct_chat)
        builder.add_node("command_guard", self.__command_guard)
        builder.add_node("human_feedback", self.__human_feedback)
        builder.add_node("confirm_sensitive_tools", self.__confirm_sensitive_tools)
        builder.add_node("cancel_sensitive_tools", self.__cancel_sensitive_tools)
        # ToolNode runs only the configured tool functions.
        builder.add_node("tools", self.__tool_node)
        
        # Route from START using early filtering.
        builder.add_conditional_edges(
            START,
            self.__route_query,
            {
                "command_guard": "command_guard",
                "direct_chat": "direct_chat",  # No-tool path.
                "assistant": "pre_assistant",   # Tool-enabled path.
                "human_feedback": "human_feedback",
                "confirm_sensitive_tools": "confirm_sensitive_tools",
                "cancel_sensitive_tools": "cancel_sensitive_tools",
            }
        )

        # Command-like requests are handled with deterministic command guidance.
        builder.add_edge("command_guard", END)
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

    def __extract_ai_content(self, messages: list) -> str:
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
        ai_messages = [message for message in current_turn_messages if isinstance(message, AIMessage)]
        if not ai_messages:
            return ''

        for ai_message in reversed(ai_messages):
            content = ai_message.content
            if isinstance(content, str):
                text = content.strip()
                if self.__is_internal_system_log_text(text):
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
                if text:
                    return text
                continue

            text = str(content).strip()
            if self.__is_internal_system_log_text(text):
                continue
            if text and text != '[]':
                return text

        return ''
    
    async def ChatAgent(self, messages: str, number: str, is_system=False, level: LevelSystem=LevelSystem.INFO, image: str=None):
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

            input_messages = []
            config = {"configurable": {"thread_id": number}}
            contact_info = self.wa_integration_tool.get_user_information(number)
            
            
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
                
            task_id = str(uuid.uuid4())
            self.db_access_tool.add_agent_input_log(InputHistory(
                id=task_id,
                timestamp=self.time_tool.current_time(),
                sender=number,
                message=messages,
                status="processing",
            ))
            
            input_messages.append(SystemMessage(content=f"Current user identifier: {number}, contact info: {contact_info.data.model_dump() if contact_info.data else 'N/A'} send at {self.time_tool.current_time()}\n{self.instruction.formatting_rule()}\n"))
            input_messages.append(HumanMessage(content=messages))

            result = await self.react_graph.ainvoke({
                "messages": input_messages,
                "sender": number
            }, config=config)
            
            results_message = self.__extract_ai_content(result.get('messages', []))

            # Some reasoning/tool-enabled model runs can end with empty text.
            # Retry once with a plain no-reasoning completion to avoid blank or stale replies.
            if not results_message:
                err = "Assistant returned empty content for '%s'; running fallback completion."
                logger.warning(err, number)
                message_sys = f"[SYSTEM MESSAGE] {err}. Please provide more information. Json Output:\n {str(result)}"
                fallback_response = self.__llm(reasoning=True).invoke([
                    AIMessage(content=message_sys)
                ])
                results = self.__content_to_text(getattr(fallback_response, "content", ""))

                if not results:
                    results = "Maaf, saya belum bisa menghasilkan jawaban. Coba kirim ulang pertanyaannya."
                    self.db_access_tool.db_set_log_system(LogSystemEntry(
                        timestamp=self.time_tool.current_time(),
                        level=LevelSystem.ERROR.value,
                        message=f"Message processed for {number} failed to generate response"
                    ))
                
                self.db_access_tool.delete_agent_input_log(task_id)
                self.wa_integration_tool.request_send_message(number, results, number)
                return
            
            self.db_access_tool.delete_agent_input_log(task_id)
            self.wa_integration_tool.request_send_message(number, results_message, number)
        except Exception as e:
            logger.exception("ChatAgent failed for %s", number)
            self.db_access_tool.delete_agent_input_log(task_id)
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
                number,
                "😭 Maaf, terjadi gangguan internal saat memproses pesan. Silakan coba lagi.",
                number,
            )
