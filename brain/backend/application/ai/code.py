from backend.application.ai.tools.time import TimeTool
import random
import requests
from langchain_core.messages import HumanMessage, SystemMessage as LangChainSystemMessage
from langchain_core.tools import tool
from pydantic import BaseModel, Field
from backend.application.ai.tools.models import LevelSystem
import asyncio
import logging
import os
import re
from collections.abc import AsyncIterator
from pathlib import Path
import time
from typing import Any, cast

from langchain_core.tools import StructuredTool

from claude_agent_sdk import (
    query, ClaudeAgentOptions,
    AssistantMessage as ClaudeAssistantMessage,
    ResultMessage as ClaudeResultMessage,
    TextBlock as ClaudeTextBlock,
    ToolUseBlock as ClaudeToolUseBlock,
    ToolResultBlock as ClaudeToolResultBlock,
    UserMessage as ClaudeUserMessage,
    SystemMessage as ClaudeSystemMessage,
    ThinkingBlock as ClaudeThinkingBlock,
    ProcessError,
    PermissionResultAllow, SettingSource,
    ToolPermissionContext,
    create_sdk_mcp_server, tool
)

from backend.application.ai.ai import AI
from backend.application.ai.tools.filex import FileX
from backend.application.ai.tools.websearch import WebSearchTool
from backend.application.ai.tools.models import InputHistory, WorkerTaskInput
from backend.application.ai.tools.whatsapp_integration import WhatsAppIntegration
from backend.application.ai.tools.dbaccess import DBAcess
from backend.infrastructure.database.database import MongoDB
from backend.application.ai.tools.terimnalx import TerminalX
from config.config import Config, BASE_DIR
from typing import TypedDict

from langgraph.checkpoint.mongodb import MongoDBSaver
from langgraph.graph import END, START, StateGraph

class PromptsType(TypedDict):
    prompts: list[str]

class AgentInnerResult(BaseModel):
    message: str
    session_id: str | None

class AgentState(TypedDict, total=False):
    sender: str
    context: str
    session_id: str | None
    message: str
    work_dir: str

class AgentCodeInput(TypedDict):
    task: str
    session_id: str | None

class AgentCodeOutput(TypedDict):
    message: str
    session_id: str | None

class AgentCodeToolInput(BaseModel):
    context: str = Field(
        description=(
            "Full task instruction for the code worker agent. "
            "append objective, constraints, and expected result if needed. "
            "Do not summarize or replace user intent with internal goals."
            "you can upgrade the instruction by adding more details, but preserve the original user intent in the instruction."
        )
    )
    work_dir: str = "playground/"
    session_id: str | None = None

# --- Monkey-patch SDK parser for Ollama compatibility ---
# Ollama doesn't include 'signature' field in thinking blocks (Claude-specific).
# Patch parse_message to use .get() with default instead of direct key access.
def _patch_sdk_message_parser():
    try:
        from claude_agent_sdk._internal import message_parser
        _original_parse = message_parser.parse_message

        def _patched_parse_message(data: Any) -> Any:
            if isinstance(data, dict):
                data_dict = cast(dict[str, Any], data)
                msg = cast(dict[str, Any], data_dict.get("message", {}))
                content = cast(Any, msg.get("content"))
                if isinstance(content, list):
                    for block in cast(list[Any], content):
                        if not isinstance(block, dict):
                            continue
                        block_dict = cast(dict[str, Any], block)
                        if block_dict.get("type") == "thinking":
                            block_dict.setdefault("signature", "")
            return _original_parse(cast(Any, data))

        message_parser.parse_message = _patched_parse_message
    except Exception:
        pass

_patch_sdk_message_parser()

logger = logging.getLogger("uvicorn.error")


class AgentCodeUsecase:
    """Encapsulates the Claude Agent SDK worker logic (run_worker).

    This class owns all worker-related orchestration previously housed in
    TerminalX: workspace resolution, prompt building, skill discovery,
    SDK streaming, progress notifications, and background dispatch.
    """

    WORKER_TIMEOUT_SECONDS = 600  # 10-minute hard cap for worker execution
    PROGRESS_SEND_TIMEOUT = 10   # 10-second cap for each progress notification

    def __init__(self):
        self.llm = AI()
        self.wa_bot = WhatsAppIntegration()
        self.filex_tool = FileX()
        self.terminalx_tool = TerminalX()
        self.web_search_tool = WebSearchTool()
        self.config = Config()
        self.db_access = DBAcess()
        self.time_tool = TimeTool()
        self.checkpointer = MongoDB.get_checkpointer()

        self._repo_root = Path(BASE_DIR).resolve()
        self._playground_root = self.filex_tool._playground_root
        self._run_subagent_root = self.filex_tool._run_subagent_root
        self._claude_instructions_root = self.filex_tool._claude_instructions_root
        self._agent_skills_root = self.filex_tool._agent_skills_root
        self.react_graph = self.agent_code_sub_graph()
        self.react_graph_as_tools = self.react_graph.as_tool(
            name=self.run_worker.__name__,
            description=self.run_worker_tool_description(),
        )
        self._process_diagram_mermaid = ""
        self._reasoning = True

    # ------------------------------------------------------------------
    # MCP tools for worker sub-agent
    # ------------------------------------------------------------------

    def _make_tools(self, sender: str) -> tuple[list[Any], Any, list[str]]:
        @tool(
            "websearch",
            "Search the web and summarize results for the active chat context.",
            {"query": str, "topic": str},
        )
        async def websearch_tool(args: dict[str, Any]) -> dict[str, Any]:
            logger.info("[Worker:%s] WebSearch tool called with args: %s", sender, args)
            query_text = str(args.get("query", "")).strip()
            topic = str(args.get("topic", "general")).strip() or "general"

            if not query_text:
                return {
                    "content": [{"type": "text", "text": "Missing required field: query."}],
                    "is_error": True,
                }

            try:
                result = await asyncio.to_thread(self.web_search_tool.websearch, sender, query_text, topic)
                return {"content": [{"type": "text", "text": result}]}
            except Exception as e:
                logger.exception("[Worker:%s] web_search_tool failed for query=%s topic=%s: %s", sender, query_text, topic, e)
                return {
                    "content": [{"type": "text", "text": f"WebSearch failed: {type(e).__name__}: {e}"}],
                    "is_error": True,
                }
        
        @tool(
            "get_screen_commands",
            (
                "Get tmux commands for managing terminal sessions. "
                "This tool is used for managing terminal sessions, always use if user want to manage terminal sessions "
                "(run system in localhost or see list of sessions), and make sure to use the returned commands for any session management. "
                "All commands are non-interactive and run in detached mode. "
                "To start a session: use 'new' then 'send' to run a command inside it. "
                "To check output: use 'capture'. To stop: use 'kill'."
                "always find and use empty ports for new sessions, do not hardcode common ports like 8000 or 8001 to avoid conflicts with existing services."
            ),
            {}
        )
        async def get_screen_commands(args: dict[str, Any]) -> dict[str, Any]:
            """
            Manage terminal sessions via tmux. Use for starting, monitoring, and stopping
            local sessions. Commands run non-interactively in detached mode.
            Workflow: 'new' → 'send' (run command) → 'capture' (check output) → 'kill' (stop).
            """
            try:
                commands = self.terminalx_tool.screen_command()
                return {"content": [{"type": "text", "text": str(commands)}]}
            except Exception as e:
                logger.exception("[Worker:%s] get_screen_commands failed: %s", sender, e)
                return {
                    "content": [{"type": "text", "text": f"Failed to get screen commands: {type(e).__name__}: {e}"}],
                    "is_error": True,
                }
        
        @tool(  
            "generate_images",
            (
                "Generate images from a list of prompts using AI Image Generation. "
                "Input: a list of text prompts (list[str]). Each prompt produces one image. "
                "Automatically determine a relevant folder path based on the context or topic "
                "(e.g. /generated_images/<topic_name> or if generate for devbelopment system, make folder in project folder). All images saved in that single folder. "
                "Output: returns the folder path (str) and list of saved filenames (list[str]). "
                "Images are saved as .jpg files. "
                "Useful for web, app, or system development projects that need generated image assets."
            ),
            {"prompts": list, "folder_path": str},
        )
        async def generate_images_tool(args: dict[str, Any]) -> dict[str, Any]:
            logger.info("[Worker:%s] generate_images tool called with args: %s", sender, args)

            prompts = args.get("prompts", [])
            folder_path = str(args.get("folder_path", "")).strip()

            if not prompts:
                return {
                    "content": [{"type": "text", "text": "Missing required field: prompts (list of strings)."}],
                    "is_error": True,
                }
            if not folder_path:
                return {
                    "content": [{"type": "text", "text": "Missing required field: folder_path."}],
                    "is_error": True,
                }

            if not isinstance(prompts, list):
                raw_response = self.__llm(reasoning=self._reasoning,structured_output=PromptsType).invoke(f"the prompts must return with list[str] structure: {prompts} ")
                prompts = raw_response.prompts

            return await asyncio.to_thread(self.filex_tool.request_generate_images, prompts, folder_path, sender)
        
        avail_tools = [websearch_tool, get_screen_commands, generate_images_tool]
        server = create_sdk_mcp_server(name="agent_tools", tools=avail_tools)
        return avail_tools, server, ["websearch", "get_screen_commands", "generate_images"]

    # ------------------------------------------------------------------
    # Workspace / path helpers
    # ------------------------------------------------------------------    

    def _normalize_workspace_hint(self, value: str | None) -> str:
        if not value:
            return ""
        return " ".join(value.strip().strip('`"\'').lower().split())

    def _is_auto_workspace_hint(self, value: str | None) -> bool:
        hint = self._normalize_workspace_hint(value)
        auto_hints = {
            "bebas",
            "terserah",
            "tentukan sendiri",
            "silakan tentukan",
            "auto",
            "automatic",
            "default",
            "up to you",
            "you decide",
        }
        return hint in auto_hints

    def _task_slug(self, task: str) -> str:
        slug = re.sub(r"[^a-z0-9]+", "-", task.lower()).strip("-")
        if not slug:
            return "worker-task"
        return slug[:64]

    def _is_windows_abs_path(self, value: str) -> bool:
        return bool(re.match(r"^[a-zA-Z]:[\\/]", (value or "").strip()))

    def safe_worker_dir(self, task: str, work_dir: str | None) -> str:
        """Resolve worker directory to a valid existing folder path.

        This guards Claude SDK `cwd` against invalid values (WinError 267).
        """
        raw = (work_dir or "").strip().strip('`"\'')

        # Default and AI-auto choices are always scoped to playground.
        if not raw or raw in {".", "./", ".\\", "playground", "playground/", "playground\\"}:
            self._playground_root.mkdir(parents=True, exist_ok=True)
            return str(self._playground_root)

        if self._is_auto_workspace_hint(raw):
            auto_dir = (self._playground_root / self._task_slug(task)).resolve()
            auto_dir.mkdir(parents=True, exist_ok=True)
            return str(auto_dir)

        normalized_raw = raw.replace("\\", "/")
        playground_base = self._playground_root.resolve()

        # If user sends a Windows-style absolute path while runtime is Linux container,
        # map safely to playground so files persist on shared Docker volume.
        if os.name != "nt" and self._is_windows_abs_path(raw):
            lower = normalized_raw.lower()
            marker = "/playground"
            idx = lower.find(marker)
            if idx >= 0:
                suffix = normalized_raw[idx + len(marker):].lstrip("/")
                candidate = self._playground_root / suffix if suffix else self._playground_root
            else:
                candidate = self._playground_root / self._task_slug(task)
        else:
            candidate = Path(normalized_raw).expanduser()
            if not candidate.is_absolute():
                # Keep relative workspaces under shared playground volume by default.
                rel_parts = list(candidate.parts)
                if rel_parts and rel_parts[0].lower() == "playground":
                    rel_parts = rel_parts[1:]
                relative_candidate = Path(*rel_parts) if rel_parts else Path(".")
                candidate = self._playground_root / relative_candidate

        try:
            resolved = candidate.resolve(strict=False)

            # Disallow relative-path traversal escaping playground sandbox.
            if not Path(normalized_raw).is_absolute() and not self._is_windows_abs_path(raw):
                resolved.relative_to(playground_base)

            if resolved.exists() and not resolved.is_dir():
                raise OSError(f"Path is not a directory: {resolved}")

            resolved.mkdir(parents=True, exist_ok=True)
            return str(resolved)
        except (OSError, ValueError):
            # Fallback for invalid/unsafe paths (e.g., invalid Windows path chars).
            fallback = (self._playground_root / self._task_slug(task)).resolve()
            fallback.mkdir(parents=True, exist_ok=True)
            return str(fallback)

    # ------------------------------------------------------------------
    # Prompt building
    # ------------------------------------------------------------------

    def build_worker_prompt(self, task: str, work_dir: str) -> str:
        clean_task = (task or "").strip()
        return (
            "WORKSPACE EXECUTION RULES:\n"
            f"- Active workspace: {work_dir}\n"
            f"- Shared playground root: {self._playground_root}\n"
            "- Treat USER TASK below as canonical user intent; do not replace it with unrelated diagnostic/test tasks.\n"
            "- Save all generated files inside the active workspace unless user explicitly requests another location.\n"
            "- After creating or editing files, verify persistence by running LS and reading the saved file(s).\n"
            "- In final output, list concrete saved file paths.\n\n"
            "USER TASK:\n"
            f"{clean_task}"
        )

    def _build_worker_skill_context(self) -> tuple[str, list[str | Path]]:
        """Build robust skill references for worker system prompt and filesystem scope."""
        project_agents_skills = (self._repo_root / ".agents" / "skills").resolve()
        user_agents_skills = (Path.home() / ".agents" / "skills").resolve()
        project_claude_skills = (self._repo_root / ".claude" / "skills").resolve()
        user_claude_skills = (Path.home() / ".claude" / "skills").resolve()

        run_subagent_hint = (os.getenv("RUN_SUBAGENT_DIR") or "").strip().strip('`"\'')
        if not run_subagent_hint:
            run_subagent_dir = self._run_subagent_root
        elif os.name != "nt" and self._is_windows_abs_path(run_subagent_hint):
            normalized_hint = run_subagent_hint.replace("\\", "/")
            marker = "/playground"
            idx = normalized_hint.lower().find(marker)
            if idx >= 0:
                suffix = normalized_hint[idx + len(marker):].lstrip("/")
                run_subagent_dir = self._playground_root / (Path(suffix) if suffix else Path("runSubagent"))
            else:
                run_subagent_dir = self._run_subagent_root
        else:
            candidate = Path(run_subagent_hint.replace("\\", "/")).expanduser()
            if not candidate.is_absolute():
                rel_parts = list(candidate.parts)
                if rel_parts and rel_parts[0].lower() == "playground":
                    rel_parts = rel_parts[1:]
                relative_candidate = Path(*rel_parts) if rel_parts else Path("runSubagent")
                run_subagent_dir = self._playground_root / relative_candidate
            else:
                run_subagent_dir = candidate
        run_subagent_dir = run_subagent_dir.resolve(strict=False)
        try:
            run_subagent_dir.mkdir(parents=True, exist_ok=True)
        except OSError:
            fallback_run_subagent_dir = self._run_subagent_root.resolve()
            try:
                fallback_run_subagent_dir.mkdir(parents=True, exist_ok=True)
            except OSError:
                pass
            run_subagent_dir = fallback_run_subagent_dir

        project_agents_skill_md = project_agents_skills / "find-skills" / "SKILL.md"
        user_agents_skill_md = user_agents_skills / "find-skills" / "SKILL.md"
        project_claude_skill_md = project_claude_skills / "find-skills" / "SKILL.md"
        user_claude_skill_md = user_claude_skills / "find-skills" / "SKILL.md"
        repo_lock = (self._repo_root / "skills-lock.json").resolve()

        existing_refs = [
            str(path)
            for path in (
                project_agents_skill_md,
                user_agents_skill_md,
                project_claude_skill_md,
                user_claude_skill_md,
                repo_lock,
            )
            if path.exists()
        ]

        add_dirs: list[str | Path] = []
        seen_dirs: set[str] = set()
        for path in (
            self._repo_root,
            project_agents_skills,
            user_agents_skills,
            project_claude_skills,
            user_claude_skills,
            self._run_subagent_root,
            run_subagent_dir,
        ):
            if not path.exists() or not path.is_dir():
                continue
            path_str = str(path)
            if path_str in seen_dirs:
                continue
            seen_dirs.add(path_str)
            add_dirs.append(path)

        if existing_refs:
            refs_text = ", ".join(existing_refs)
            prompt = (
                "Use installed skill guidance when relevant. "
                f"Read these files when available: {refs_text}. "
                f"Use runSubagent workspace at: {run_subagent_dir}. "
                "use for discover capabilities or install skills, follow find-skills guidance from those files."
                "max timeout:3 minutes for Tool Bash"
            )
            return prompt, add_dirs

        fallback_prompt = (
            "Use installed skill guidance when relevant. "
            f"Look for find-skills docs in project `{self._agent_skills_root}/find-skills/SKILL.md` "
            f"or user `{self._agent_skills_root}/find-skills/SKILL.md` when those files exist. "
            f"Use runSubagent workspace at: {run_subagent_dir}. "
            f"If Claude native skills are present, also check `{self._claude_instructions_root}` locations."
            "max timeout:3 minutes for Tool Bash"
        )
        return fallback_prompt, add_dirs

    def _build_worker_setting_sources(self) -> list[SettingSource]:
        """Choose SDK setting_sources safely across local and Docker runtimes."""
        project_has_claude_config = (
            (self._repo_root / "CLAUDE.md").exists()
            or (self._repo_root / ".claude" / "CLAUDE.md").exists()
            or (self._repo_root / ".claude" / "skills").exists()
            or (self._repo_root / ".claude" / "output-styles").exists()
        )

        user_home = Path.home()
        user_has_claude_config = (
            (user_home / ".claude" / "CLAUDE.md").exists()
            or (user_home / ".claude" / "skills").exists()
            or (user_home / ".claude" / "output-styles").exists()
        )

        setting_sources: list[SettingSource] = []
        if project_has_claude_config:
            setting_sources.append("project")
        if user_has_claude_config:
            setting_sources.append("user")

        # Keep explicit defaults so SDK can still discover user/project filesystem settings
        # when files are mounted later in Docker runtime.
        if not setting_sources:
            setting_sources = ["project", "user"]

        return setting_sources

    def run_worker_tool_description(self) -> str:
        repo_root = str(self._repo_root)
        playground_root = str(self._playground_root)
        return (
            "Execute complex agent work with planning, tool orchestration, and iterative reasoning."
            "\n\nAI level instruction (routing priority):"
            "\n- Treat this tool as preferred default for non-trivial engineering tasks."
            "\n- This is the advanced worker, and it is an upgrade path from `simple_exec`."
            "\n- Choose `run_worker` when the request needs multi-step execution,"
            " code generation/editing, repository exploration, debugging loops, or chained tasks."
            "\n- Use this tool when task completion depends on agent decisions, not only a single shell command."
            "\n- The worker runs through `ClaudeAgentOptions` with scoped tools"
            " (`Bash`, `Read`, `Write`, `Edit`, `Glob`, `LS`) and persistent `cwd` context."
            f"\n- Runtime repo root: `{repo_root}`."
            f"\n- Runtime shared playground root: `{playground_root}`."
            f"\n- Relative `work_dir` is resolved under playground (example: `my-task` -> `{playground_root}/my-task`)."
            "\n- Start `task` with the user's original intent (verbatim when possible), then add objective, constraints, and expected result."
            "\n- Do not substitute user intent with internal testing goals unless user explicitly asks for testing."
            "\n- Workspace policy: if user has not provided workspace location, ask once before execution."
            "\n- If user says AI may decide (e.g., 'bebas', 'terserah', 'tentukan sendiri'),"
            " create/select a project folder under `playground/` and use it as `work_dir`."
            "\n- Folder name should follow the project/task name in a safe slug format."
            "\n- Never let auto-selected workspace escape `playground/` (no `..`, absolute path, or parent traversal)."
            "\n- For file-creation tasks, verify saved files with `LS` and `Read` before finishing."
            "\n- Do NOT route trivial one-shot shell commands here; use `simple_exec` for those."
            "\n- Default `work_dir` to `playground/` if not specified."
            "\n- Default `wait_for_completion` is true so worker output is returned in the same turn."
            "\n- Set `wait_for_completion=false` only for background dispatch mode."
            "\n- Session resume: if user wants to continue previous work, first call `agent_input_log` with"
            " `func_name='run_worker'` to find the session, then pass `session_id` to resume."
            "\n\nArgs:"
            "\n- task: Preserve user intent first, then provide goal-oriented details for worker execution."
            "\n- session_id: Optional. Session ID from a previous worker run. Pass this to resume"
            " the worker from its prior conversation context (previous tool calls, file edits, decisions)."
            " Get it from `agent_input_log`. Leave None for new sessions."
            "\n- work_dir: Optional workspace path. If omitted, ask location first;"
            " if user lets AI decide the workspace, create/use project folder inside `playground/` only."
            "\n- wait_for_completion: Optional. true waits for worker completion in this turn; false dispatches background worker."
            "\n\nReturns:"
            "\n- str: Worker completion output (blocking mode) or dispatch status (background mode)."
        )

    # ------------------------------------------------------------------
    # Environment synchronisation
    # ------------------------------------------------------------------

    def ossync(self):
        """Sync important env vars from Config to os.environ for subprocesses and agent SDK."""

        anthropic_config = self.config.get_anthropic_config()
        ollama_config = self.config.get_ollama_config()
        ollama_base_url = (ollama_config.base_url or '').strip()

        # Always clear first so stale env values cannot force Anthropic unexpectedly.
        anthropic_env = {
            'ANTHROPIC_BASE_URL': (anthropic_config.base_url or '').strip(),
            'ANTHROPIC_AUTH_TOKEN': (anthropic_config.auth_token or '').strip(),
            'ANTHROPIC_API_KEY': (anthropic_config.api_key or '').strip(),
            'ANTHROPIC_MODEL': (anthropic_config.model or '').strip(),
        }
        for key in anthropic_env:
            os.environ.pop(key, None)

        for key, value in anthropic_env.items():
            if value:
                os.environ[key] = value

        # Claude Agent SDK still expects Anthropic-compatible env vars even when
        # the model/backend is served by Ollama.
        anthropic_model = anthropic_env.get('ANTHROPIC_MODEL', '')
        if not anthropic_model and ollama_base_url:
            os.environ['ANTHROPIC_BASE_URL'] = ollama_base_url
            compat_token = os.getenv('ANTHROPIC_AUTH_TOKEN') or os.getenv('ANTHROPIC_API_KEY') or 'ollama'
            os.environ['ANTHROPIC_AUTH_TOKEN'] = compat_token
            os.environ['ANTHROPIC_API_KEY'] = compat_token

        if ollama_base_url:
            os.environ['OLLAMA_BASE_URL'] = ollama_base_url
        else:
            os.environ.pop('OLLAMA_BASE_URL', None)

    # ------------------------------------------------------------------
    # WhatsApp progress notifications
    # ------------------------------------------------------------------

    async def _send_worker_progress(self, sender: str, step: int, title: str, detail: str = "") -> None:
        title = (title or "").strip()
        detail = (detail or "").strip()

        if not title:
            return

        compact_detail = " ".join(detail.replace("\n", " ").split())
        if len(compact_detail) > 280:
            compact_detail = compact_detail[:280].rstrip() + "..."

        if compact_detail:
            msg = f"🤖✨ *Worker job* [{step}] {title}\n```{compact_detail}```"
        else:
            msg = f"🤖✨ *Worker job* [{step}] {title}"

        try:
            result = await asyncio.wait_for(
                asyncio.to_thread(
                    self.wa_bot.request_send_message,
                    sender,
                    msg,
                    sender,
                ),
                timeout=self.PROGRESS_SEND_TIMEOUT,
            )
            if not result.success:
                logger.warning("worker progress send failed for %s: %s", sender, result.message)
        except asyncio.TimeoutError:
            logger.warning("worker progress send timed out for %s (step %d)", sender, step)
        except Exception as exc:
            logger.warning("worker progress send exception for %s: %s", sender, exc)

    # ------------------------------------------------------------------
    # Core async agent execution
    # ------------------------------------------------------------------

    async def async_agent(self, sender: str, prompt: str, work_dir: str, session_id: str | None = None) -> dict[str, Any]:
        """Core async agent logic with overall timeout protection."""
        try:
            return await asyncio.wait_for(
                self._async_agent_inner(sender, prompt, work_dir, session_id=session_id),
                timeout=self.WORKER_TIMEOUT_SECONDS,
            )
        except asyncio.TimeoutError:
            logger.error("Worker timed out after %ds for %s", self.WORKER_TIMEOUT_SECONDS, sender)
            return {"message": f"[ERROR] Worker execution timed out after {self.WORKER_TIMEOUT_SECONDS // 60} minutes."}
        except Exception as e:
            logger.exception("Worker execution failed for %s", sender)
            return {"message": f"[ERROR] Worker execution failed: {type(e).__name__}: {e}"}

    @staticmethod
    def _redact_sensitive_text(text: str) -> str:
        redacted = text or ""
        redacted = re.sub(
            r"(?i)(api[_-]?key|auth[_-]?token|token|authorization)(\s*[:=]\s*)([^\s,;]+)",
            r"\1\2[REDACTED]",
            redacted,
        )
        redacted = re.sub(r"sk-ant-[A-Za-z0-9_-]+", "sk-ant-[REDACTED]", redacted)
        redacted = re.sub(r"lsv2_[A-Za-z0-9_-]+", "lsv2_[REDACTED]", redacted)
        return redacted

    def _format_sdk_process_error(
        self,
        err: ProcessError,
        stderr_lines: list[str],
        worker_model: str,
        safe_work_dir: str,
    ) -> str:
        stderr_text = "\n".join(stderr_lines[-20:]).strip()
        if len(stderr_text) > 3000:
            stderr_text = stderr_text[-3000:].strip()

        parts = [
            "Claude Agent SDK subprocess exited before completing initialization.",
            f"Model: {worker_model or '(empty)'}",
            f"Workspace: {safe_work_dir}",
            f"Exit code: {getattr(err, 'exit_code', None) or 'unknown'}",
        ]
        if stderr_text:
            parts.append("Recent SDK stderr:\n" + self._redact_sensitive_text(stderr_text))
        else:
            parts.append(
                "Recent SDK stderr: (empty). Check container logs around "
                f"[Worker:{safe_work_dir}] or run Claude Code doctor inside the container."
            )

        return "\n".join(parts)

    async def _async_agent_inner(self, sender: str, prompt: str, work_dir: str, session_id: str | None = None) -> dict[str, Any]:
        """Inner async agent logic — streams SDK messages and sends progress."""
        if not prompt.strip():
            raise ValueError("[ERROR] No task provided for worker execution.")
        if not work_dir.strip():
            raise ValueError("[ERROR] No workspace provided for worker execution.")

        anthropic_config = self.config.get_anthropic_config()
        anthropic_model = (anthropic_config.model or '').strip()
        anthropic_api_key = (anthropic_config.api_key or '').strip()
        anthropic_auth_token = (anthropic_config.auth_token or '').strip()

        if anthropic_model:
            if not anthropic_api_key and not anthropic_auth_token:
                raise ValueError(
                    "Anthropic model is configured but no credential is set. "
                    "Set API key/auth token with `/anthropic-config` or clear the Anthropic model to use Ollama."
                )
            worker_model = anthropic_model
            logger.info("Using Anthropic model for worker: %s", worker_model)
        else:
            ai_config =  self.config.get_agent_model_config()
            ollama_config = self.config.get_ollama_config()
            worker_model = ai_config.agent_code_model.agent_model if ai_config.agent_code_model.agent_provider != "default" else None
            if worker_model is None:
                ollama_model = (ollama_config.model or "").strip()
                if ollama_model == "":
                    raise ValueError("[ERROR] Ollama model not configured. Please set the Ollama model in the configuration. if you in bot, type `/ollama-config` FOR for using claude-agent-sdk using ollama or `/anthropic-config` for using Anthropic's API.")
                else:
                    worker_model = ollama_model

            ollama_base_url = (ollama_config.base_url or '').strip()
            if not ollama_base_url:
                raise ValueError(
                    "[ERROR] Ollama base URL is not configured. "
                    "Please set it with `/ollama-config` before using Ollama fallback."
                )
            os.environ['OLLAMA_BASE_URL'] = ollama_base_url
                        
            logger.info("Using Ollama model for worker: %s", worker_model)
        
        safe_work_dir = self.safe_worker_dir(task=prompt, work_dir=work_dir)
        skill_system_prompt, skill_add_dirs = self._build_worker_skill_context()
        worker_setting_sources = self._build_worker_setting_sources()

        async def _auto_approve_tool(
            tool_name: str,
            tool_input: dict[str, Any],
            context: ToolPermissionContext,
        ) -> PermissionResultAllow:
            """Auto-approve all tool permission requests."""
            _ = (tool_input, context)
            logger.info("[Worker:%s] Auto-approving tool: %s", sender, tool_name)
            return PermissionResultAllow()

        async def _prompt_stream() -> AsyncIterator[dict[str, Any]]:
            """Streaming mode is required when can_use_tool callback is set."""
            yield {
                "type": "user",
                "message": {"role": "user", "content": prompt},
                "parent_tool_use_id": None,
                "session_id": "",
            }
        
        list_tools, server, custom_tools = self._make_tools(sender)
        list_tools_string = "\n".join([str(t.name) + " : " + str(t.description) + "\n" for t in list_tools])
        list_tools_string = f"<available__mcp_tools> {list_tools_string} </available__mcp_tools>"

        

        sdk_stderr_lines: list[str] = []

        def _capture_sdk_stderr(line: str) -> None:
            clean_line = self._redact_sensitive_text(str(line).strip())
            if not clean_line:
                return
            sdk_stderr_lines.append(clean_line)
            del sdk_stderr_lines[:-50]
            logger.warning("[Worker:%s][SDK stderr] %s", sender, clean_line)
        
        await self._send_worker_progress(sender, 0, "Initializing", f"Continuing Session: {session_id if session_id else None}")

        options = ClaudeAgentOptions(
            resume=session_id if session_id else None,
            model=worker_model,
            mcp_servers={
                "agent_tools": server,
            },
            allowed_tools=[
                "Read",
                "Write",
                "Edit",
                "Glob",
                "Grep",
                "Bash",
                "Monitor",
                "WebSearch",
                "WebFetch",
                "Agent",
                "LS",
                "TodoWrite",
                "TodoRead",
                "Skill",
                *custom_tools,
            ],
            setting_sources=worker_setting_sources,
            permission_mode="default",
            can_use_tool=_auto_approve_tool,
            cwd=safe_work_dir,
            add_dirs=skill_add_dirs,
            stderr=_capture_sdk_stderr,
            system_prompt= skill_system_prompt +"\n"+ list_tools_string,
        )

        # Extra debug logging to help diagnose SDK init failures
        try:
            add_dirs_info = [str(d) + f" (type={type(d).__name__})" for d in skill_add_dirs]
        except Exception:
            add_dirs_info = [repr(d) for d in skill_add_dirs]
        logger.debug(
            "[Worker:%s] SDK options: model=%s, cwd=%s, setting_sources=%s, add_dirs=%s, custom_tools=%s",
            sender,
            worker_model,
            safe_work_dir,
            worker_setting_sources,
            add_dirs_info,
            custom_tools,
        )

        logger.info(
            "[Worker:%s] Starting SDK query (model=%s, cwd=%s, setting_sources=%s, add_dirs=%d, permission=default+auto_approve, resume=%s)",
            sender,
            worker_model,
            safe_work_dir,
            worker_setting_sources,
            len(skill_add_dirs),
            session_id or 'new',
        )

        output_parts: list[str] = []
        step = 0
        sent_progress = False
        final_session_id = (session_id or "").strip()

        try:
            async for message in query(prompt=_prompt_stream(), options=options):
                logger.info("[Worker:%s] SDK message type=%s step=%d", sender, type(message).__name__, step)
                if isinstance(message, ClaudeAssistantMessage):
                    for block in message.content:
                        if isinstance(block, ClaudeTextBlock):
                            text = (block.text or "").strip()
                            if not text:
                                continue
                            step += 1
                            sent_progress = True
                            output_parts.append(text)
                            await self._send_worker_progress(sender, step, "Assistant Response", text)
                        elif isinstance(block, ClaudeToolUseBlock):
                            step += 1
                            sent_progress = True
                            tool_name = (block.name or "tool").strip()
                            await self._send_worker_progress(sender, step, f"Using Tool: {tool_name}", str(block.input))
                        elif isinstance(block, ClaudeThinkingBlock):
                            step += 1
                            sent_progress = True
                            await self._send_worker_progress(sender, step, "Thinking", block.thinking)
                elif isinstance(message, ClaudeUserMessage):
                    blocks = message.content if isinstance(message.content, list) else []
                    for block in blocks:
                        if isinstance(block, ClaudeToolResultBlock):
                            step += 1
                            sent_progress = True
                            preview = ""
                            if block.content is not None:
                                preview = str(block.content)
                            if block.is_error:
                                await self._send_worker_progress(sender, step, "Tool Result Error", preview)
                            else:
                                await self._send_worker_progress(sender, step, "Tool Result", preview)
                elif isinstance(message, ClaudeSystemMessage):
                    subtype = (message.subtype or "").strip()
                    if subtype in {"task_started", "task_progress", "task_notification"}:
                        step += 1
                        sent_progress = True
                        description = str(
                            message.data.get("description")
                            or message.data.get("summary")
                            or message.data.get("status")
                            or ""
                        )
                        await self._send_worker_progress(sender, step, f"Task {subtype}", description)
                    else:
                        await self._send_worker_progress(sender, step, f"System Message: {subtype}", f"{message.data if isinstance(message.data, str) else str(message)}")
                elif isinstance(message, ClaudeResultMessage) and message.subtype == "success":
                    result_message = "" if message.result is None else str(message.result)
                    # add or update session_id
                    final_session_id = (message.session_id or options.resume or "").strip()

                    if result_message:
                        result_text = result_message.strip()
                        if result_text:
                            if not output_parts or output_parts[-1] != result_text:
                                output_parts.append(result_text)
                            if not sent_progress:
                                step += 1
                                await self._send_worker_progress(sender, step, "Completed", result_text)
        except ProcessError as err:
            detail = self._format_sdk_process_error(
                err,
                stderr_lines=sdk_stderr_lines,
                worker_model=worker_model,
                safe_work_dir=safe_work_dir,
            )
            logger.error("[Worker:%s] %s", sender, detail)
            raise RuntimeError(detail) from err

        logger.info("[Worker:%s] SDK query loop finished. total_steps=%d, output_parts=%d", sender, step, len(output_parts))
        results = AgentInnerResult(message="\n".join(output_parts), session_id=final_session_id or None)
        return results.__dict__

    # ------------------------------------------------------------------
    # Background worker dispatch
    # ------------------------------------------------------------------

    async def worker_background(self, sender: str, task: str, safe_work_dir: str, worker_prompt: str, session_id: str | None = None) -> None:
        """Background worker — runs independently and sends results via WhatsApp."""
        try:
            result = await self.async_agent(sender, worker_prompt, work_dir=safe_work_dir, session_id=session_id)
            # Handle both dict (normal) and string (error) returns from async_agent
            if isinstance(result, str):
                final_msg = f"✅ *Worker Completed*\n📋 Task: `{task}`\n\n{result}"
            elif result and result.get("message", "").strip():
                summary = result["message"].strip()
                if len(summary) > 1500:
                    summary = summary[:1500].rstrip() + "\n... _(truncated)_"
                final_msg = f"✅ *Worker Completed*\n📋 Task: `{task}`\n\n{summary}"
            else:
                final_msg = f"✅ *Worker Completed*\n📋 Task: `{task}`\n\n_(No output produced)_"
            
            await asyncio.wait_for(
                asyncio.to_thread(self.wa_bot.request_send_message, sender, final_msg, sender),
                timeout=self.PROGRESS_SEND_TIMEOUT,
            )
        except asyncio.CancelledError:
            logger.warning("[Worker:%s] Background task was cancelled", sender)
        except Exception as exc:
            logger.exception("[Worker:%s] Background task failed", sender)
            try:
                err_msg = f"❌ *Worker Failed*\n📋 Task: `{task}`\n\n`{type(exc).__name__}: {exc}`"
                await asyncio.wait_for(
                    asyncio.to_thread(self.wa_bot.request_send_message, sender, err_msg, sender),
                    timeout=self.PROGRESS_SEND_TIMEOUT,
                )
            except Exception:
                logger.warning("[Worker:%s] Failed to send error notification", sender)

    # ------------------------------------------------------------------
    # run_worker StructuredTool factory
    # ------------------------------------------------------------------

    def run_worker(self, sender: str) -> StructuredTool:
        """
        run_worker is used for more complex tasks that may require multi-step reasoning,
        file access, or persistent working-directory context.

        By default, this tool waits for worker completion so the assistant can
        continue reasoning with worker results in the same turn.

        Background mode is still available via wait_for_completion=False.
        """
        self.ossync()  # Ensure env vars are set for subprocesses and agent SDK
        async def _worker_async(task: str, session_id: str = None, work_dir: str = "playground/", wait_for_completion: bool = True) -> str:

            safe_work_dir = self.safe_worker_dir(task=task, work_dir=work_dir)
            worker_prompt = self.build_worker_prompt(task=task, work_dir=safe_work_dir)

            # Log resume status
            resume_label = f"resume={session_id}" if session_id else "new session"
            logger.info("[Worker:%s] run_worker called: %s, task=%s", sender, resume_label, task[:80])

            # Send start notification
            try:
                resume_info = f"\n🔄 Resuming session: `{session_id}`" if session_id else ""
                await asyncio.wait_for(
                    asyncio.to_thread(
                        self.wa_bot.request_send_message,
                        sender,
                        f"🤖🛠️ *Worker Executing task:* `{task}`\n📁 Workspace: `{safe_work_dir}`{resume_info}",
                        sender,
                    ),
                    timeout=self.PROGRESS_SEND_TIMEOUT,
                )
            except (asyncio.TimeoutError, Exception) as exc:
                logger.warning("Worker start notification failed for %s: %s", sender, exc)

            if not wait_for_completion:
                # Launch worker as background task — do NOT await it
                asyncio.create_task(
                    self.worker_background(sender, task, safe_work_dir, worker_prompt, session_id=session_id),
                    name=f"worker-{sender}-{task[:30]}",
                )

                # Return immediately so main agent can continue
                return (
                    f"ℹ️ *Worker task has been dispatched in the background.*\n"
                    f"Task: {task}\n"
                    f"Workspace: {safe_work_dir}\n"
                    f"Session: `{resume_label}`\n"
                    f"The worker will send progress updates and final results directly via WhatsApp. "
                    f"You may continue with other tasks."
                )

            result = await self.async_agent(sender, worker_prompt, work_dir=safe_work_dir, session_id=session_id)
            normalized_result = (result["message"] or "").strip()
            if not normalized_result:
                normalized_result = "(No output produced)"

            return (
                f"Worker task completed in this turn.\n"
                f"Task: {task}\n"
                f"Workspace: {safe_work_dir}\n"
                f"Session: {resume_label}\n"
                f"Result:\n{normalized_result}"
            )
            

        def _worker_sync(task: str, session_id: str = None, work_dir: str = "playground/", wait_for_completion: bool = True) -> str:
            """Sync fallback — should not be called when coroutine is available."""
            return "[ERROR] Worker must be called asynchronously. Check tool node configuration."

        return StructuredTool.from_function(
            func=_worker_sync,
            coroutine=_worker_async,
            name="run_worker",
            description=self.run_worker_tool_description(),
            args_schema=WorkerTaskInput
        )
    
    def __llm(self, reasoning: bool = True, structured_output: Any | None = None) -> Any:
        ai_config =  self.config.get_agent_model_config()
        agent_code_model_config = ai_config.agent_code_model if ai_config.agent_code_model.agent_provider != "default" else None
        return self.llm.LLM(reasoning=reasoning, providers=agent_code_model_config, structured_output=structured_output)
    
    @staticmethod
    def __content_to_text(content: Any) -> str:
        if isinstance(content, str):
            return content.strip()

        if isinstance(content, list):
            text_parts: list[str] = []
            for part in content:
                if isinstance(part, str):
                    text_parts.append(part)
                    continue
                if isinstance(part, dict) and part.get("text"):
                    text_parts.append(str(part["text"]))
            return "\n".join(text_parts).strip()

        return str(content).strip()

    def __save_process_diagram(self, react_graph: Any) -> None:
        try:
            self._process_diagram_mermaid = react_graph.get_graph().draw_mermaid()
            logger.info(
                "Code agent process diagram mermaid cached (%d chars)",
                len(self._process_diagram_mermaid),
            )
        except Exception as err:
            logger.warning("Failed to render code agent process diagram mermaid: %s", err)
            self._process_diagram_mermaid = ""

    def get_process_diagram_mermaid(self) -> str:
        diagram = (self._process_diagram_mermaid or "").strip()
        if diagram:
            return diagram

        try:
            fallback = self.react_graph.get_graph().draw_mermaid()
        except Exception as err:
            logger.warning("Failed to build code agent process diagram mermaid on demand: %s", err)
            return ""

        self._process_diagram_mermaid = fallback
        return fallback.strip()

    def get_all_code_session_logs(self, state: AgentState | None = None):
        return self.__select_session_id(state or {})

    def __route_code_request(self, state: AgentState) -> str:
        context = (state.get("context") or "").strip()
        if not context:
            return "run_worker"

        # Attach recent conversation history if available in state
        history_text = ""
        try:
            raw_history = state.get("history") or state.get("conversation") or None
            if raw_history:
                history_text = "\n\nPrevious conversation history:\n" + str(raw_history).strip()
        except Exception:
            history_text = ""

        # Also include recent worker sessions from agent_input_log to help choose resume
        sessions_text = ""
        try:
            sender = (state.get("sender") or "").strip()
            if sender:
                recent = self.db_access.get_agent_input_log(sender=sender, func_name=self.run_worker.__name__)
                if recent:
                    lines = []
                    for s in recent[-6:]:
                        msg = " ".join((s.message or "").split())
                        lines.append(f"- session_id: {s.session_id or '(none)'} | msg: {msg[:200]}")
                    sessions_text = "\n\nRecent worker sessions:\n" + "\n".join(lines)
        except Exception:
            sessions_text = ""

        prompt = (
            "Classify the latest user request for the CodeChatAgent workflow (worker session routing).\n"
            "\n"
            "CONTEXT: The agent can either:\n"
            "1. Show the user past code-worker session history (list_sessions)\n"
            "2. Execute a new/existing coding task via the worker (run_worker)\n"
            "\n"
            "IMPORTANT: Before deciding, review the previous conversation history and user messages to understand intent.\n"
            "Use the conversation context to detect references to earlier sessions, filenames, artifacts, or prior outputs.\n"
            "If a suitable prior session exists, prefer resuming it; otherwise decide to start a new session.\n"
            "\n"
            "PENTING: Sebelum memutuskan, lihat percakapan terdahulu pengguna untuk memahami konteks — cari referensi ke session sebelumnya, nama file, atau hasil kerja sebelumnya.\n"
            "Gunakan riwayat percakapan untuk memilih session yang paling relevan untuk dilanjutkan, atau putuskan untuk memulai session baru jika tidak ada yang cocok.\n"
            "\n"
            "DEFINITION OF `list_sessions` (Worker Session History ONLY):\n"
            "- User asks to VIEW, LIST, INSPECT, CHECK, SHOW, or QUERY existing code-worker sessions.\n"
            "- Keywords: 'show sessions', 'list history', 'check past work', 'what sessions', 'session IDs',\n"
            "  'previous work', 'past tasks', 'resume from', 'show my work history'.\n"
            "- Purpose: Display session_id, timestamp, and task name from Redis session log.\n"
            "- SCOPE: This is ONLY for code-worker session records, NOT for other types of lists\n"
            "  (e.g., files, projects, commands, artifacts, etc.).\n"
            "\n"
            "DEFINITION OF `run_worker` (Task Execution):\n"
            "- User asks the worker to CREATE, EDIT, DEBUG, CONTINUE, RUN, EXPLAIN, or GENERATE code/files.\n"
            "- Keywords: 'create', 'edit', 'fix', 'debug', 'run', 'test', 'explain', 'implement',\n"
            "  'write', 'generate', 'build', 'solve', 'continue from', 'resume work'.\n"
            "- Purpose: Execute a coding task via the Claude Agent SDK worker.\n"
            "- Includes resume from a known session (e.g., 'continue session ABC123').\n"
            "\n"
            "ROUTING DECISION RULES:\n"
            "1. If user wants to see past work/session history → list_sessions.\n"
            "2. If user wants the worker to DO something (code, edit, debug, etc.) → run_worker.\n"
            "3. If ambiguous → default to run_worker (task execution is more common).\n"
            "4. Do not confuse 'list' with worker session history (e.g., 'list files' → run_worker, not list_sessions).\n"
            "\n"
            "Return exactly one label: list_sessions or run_worker. Do not explain."
        )
        # Build combined content for the LLM: include latest context plus optional history and session list
        combined_content = context + (history_text or "") + (sessions_text or "")
        try:
            response = self.__llm(reasoning=False).invoke([
                LangChainSystemMessage(content=prompt),
                HumanMessage(content=combined_content),
            ])
            logger.info("Code agent request classified with response: %s", self.__content_to_text(getattr(response, "content", response)))
        except Exception as err:
            logger.warning("Failed to classify code agent request; defaulting to run_worker. detail=%s", err)
            return "run_worker"

        label = self.__content_to_text(getattr(response, "content", response)).lower()
        if "list_sessions" in label:
            return "list_sessions"
        return "run_worker"

    def __format_session_list(self, sessions: list[InputHistory]) -> str:
        if not sessions:
            return "Tidak ada session code worker yang tersimpan untuk sender ini."

        lines = [
            "Session code worker yang tersimpan untuk sender ini:",
        ]
        for index, session in enumerate(sessions, start=1):
            message = " ".join((session.message or "").split())
            if len(message) > 180:
                message = message[:180].rstrip() + "..."
            lines.append(
                f"{index}. session_id: {session.session_id}\n"
                f"   timestamp: {session.timestamp}\n"
                f"   message: {message or '(empty)'}"
            )

        lines.append(
            "Catatan: data ini berasal dari Redis session log untuk resume worker, "
            "bukan status proses OS realtime."
        )
        return "\n".join(lines)

    def __list_sessions(self, state: AgentState) -> AgentState:
        sender = (state.get("sender") or "").strip()
        if not sender:
            return {"message": "[ERROR] Missing sender for session lookup."}

        sessions = self.db_access.get_agent_input_log(
            sender=sender,
            func_name=self.run_worker.__name__,
        )
        return {
            "message": self.__format_session_list(sessions),
            "session_id": None,
        }

    def __select_session_id(self, state: AgentState) -> str | None:
        sender = (state.get("sender") or "").strip()
        context = (state.get("context") or "").strip()
        if not sender or not context:
            return None

        sess_list = self.db_access.get_agent_input_log(
            sender=sender,
            func_name=self.run_worker.__name__,
        )
        if not sess_list:
            return None

        session_ids = {session.session_id for session in sess_list if session.session_id}
        if not session_ids:
            return None

        list_str = "\n".join(
            f"- session_id: {sess.session_id}, timestamp: {sess.timestamp}, message: {sess.message}"
            for sess in sess_list
        )
        prompt = (
            "You select a previous code-worker session for automatic resume.\n"
            "Return exactly one session_id from the list if it is clearly relevant to the latest user request.\n"
            "Return an empty string if the latest request should start a new session.\n\n"
            "Available sessions:\n"
            f"{list_str}"
        )

        try:
            response = self.__llm(reasoning=False).invoke([
                LangChainSystemMessage(content=prompt),
                HumanMessage(content=context),
            ])
        except Exception as err:
            logger.warning("Failed to select code worker session for %s: %s", sender, err)
            return None

        selected_text = self.__content_to_text(getattr(response, "content", response))
        for session_id in session_ids:
            if session_id in selected_text:
                return session_id

        selected_text = selected_text.strip().strip("`'\"")
        return selected_text if selected_text in session_ids else None

    def __select_session(self, state: AgentState) -> AgentState:
        explicit_session_id = (state.get("session_id") or "").strip()
        if explicit_session_id:
            return {"session_id": explicit_session_id}

        selected_session_id = self.__select_session_id(state)
        return {"session_id": selected_session_id}

    async def __run_worker_node(self, state: AgentState) -> AgentState:
        sender = (state.get("sender") or "").strip()
        context = (state.get("context") or "").strip()
        work_dir = (state.get("work_dir") or "playground/").strip() or "playground/"
        session_id = (state.get("session_id") or "").strip() or None

        if not sender:
            return {"message": "[ERROR] Missing sender for code worker execution."}
        if not context:
            return {"message": "[ERROR] Missing task context for code worker execution."}

        self.ossync()
        safe_work_dir = self.safe_worker_dir(task=context, work_dir=work_dir)
        worker_prompt = self.build_worker_prompt(task=context, work_dir=safe_work_dir)
        result = await self.async_agent(sender, worker_prompt, safe_work_dir, session_id=session_id)

        if isinstance(result, dict):
            result_message = str(result.get("message") or "").strip()
            result_session_id = result.get("session_id") or session_id
        else:
            result_message = str(result).strip()
            result_session_id = session_id

        if not result_message:
            result_message = "(No output produced)"
            
        self.store_session_log(sender, result_message, result_session_id or "")

        return {
            "message": result_message,
            "session_id": result_session_id,
            "work_dir": safe_work_dir,
        }
    
    def store_session_log(self, sender: str, result_message: str, session_id: str) -> None:
        # add or update session_id
        if session_id:
            logger.info("Storing session log for sender=%s, session_id=%s", sender, session_id)
            # Call LLM to generate a concise summary; measure duration for debugging
            try:
                t0 = time.time()
                logger.debug("[StoreSession] Invoking LLM to summarize result for sender=%s, session_id=%s", sender, session_id)
                response = self.__llm(reasoning=False).invoke([
                    HumanMessage(content="write summary of this worker execution result, create a concise summary and title (create short but descriptive): " + result_message),
                ])
                duration = time.time() - t0
                logger.debug("[StoreSession] LLM summary completed in %.2fs for sender=%s, session_id=%s", duration, sender, session_id)
                selected_text = self.__content_to_text(getattr(response, "content", response))
            except Exception as e:
                logger.exception("[StoreSession] LLM summarization failed for sender=%s, session_id=%s: %s", sender, session_id, e)
                selected_text = ""
            
            existing = self.db_access.find_session_agent_input_log(
                sender=sender,
                func_name=self.run_worker.__name__,
                session_id=session_id or "",
            )
            
            result_message = selected_text if selected_text else result_message
            
            if existing:
                self.db_access.update_agent_status_input_log(
                    sender=sender,
                    session_id=session_id,
                    func_name=self.run_worker.__name__,
                    message=result_message or "",
                )
            else:
                self.db_access.add_agent_input_log(InputHistory(
                    session_id=session_id,
                    timestamp="",
                    sender=sender,
                    message=result_message or "",
                    func_name=self.run_worker.__name__,
                ))
        else:
            logger.info("No session_id to store for sender=%s. Skipping session log storage.", sender)
        
    
    def agent_code_sub_graph(self):
        builder = StateGraph(AgentState)
        builder.add_node("list_sessions", self.__list_sessions)
        builder.add_node("select_session", self.__select_session)
        builder.add_node("run_worker", self.__run_worker_node)

        builder.add_conditional_edges(
            START,
            self.__route_code_request,
            {
                "list_sessions": "list_sessions",
                "run_worker": "select_session",
            },
        )
        builder.add_edge("list_sessions", END)
        builder.add_edge("select_session", "run_worker")
        builder.add_edge("run_worker", END)

        react_graph = builder.compile(checkpointer=self.checkpointer)
        self.__save_process_diagram(react_graph)

        return react_graph
    
    def make_agent_code_tool(self, sender: str):
        """Factory — inject sender via closure, context dari LLM."""

        async def _run(context: str, work_dir: str = "playground/", session_id: str | None = None) -> str:
            result = await self.react_graph.ainvoke(
                {
                    "sender": sender,       # <-- inject dari closure
                    "context": context,     # <-- dari LLM / parent graph
                    "session_id": session_id,
                    "work_dir": work_dir,
                }
            )
            if isinstance(result, dict):
                return str(result.get("message") or "(No output produced)")
            return str(result)

        def _run_sync(*args, **kwargs) -> str:
            return "[ERROR] Must be called async."

        return StructuredTool.from_function(
            func=_run_sync,
            coroutine=_run,
            name=self.run_worker.__name__,
            description=self.run_worker_tool_description(),
            args_schema=AgentCodeToolInput,
        )
    
    def get_agent_code_tool(self, sender: str) -> StructuredTool:
        return self.make_agent_code_tool(sender)
            
    
    async def CodeChatAgent(
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
            _ = (is_system, level, image)
            context = (messages or "").strip()
            if reply_content:
                contact_info = self.wa_bot.get_user_information(number)
                context = (
                    "User information: " + 
                    f"{contact_info.data.model_dump() if contact_info.data else 'N/A'}" + 
                    " send at " + 
                    f"{self.time_tool.current_time()}" + 
                    ". Previous Message (if any): " + 
                    f"{reply_content}" + 
                    ". Latest user request: " + 
                    f"{context}"
                )

            config = {"configurable": {"thread_id": group_id if group_id else number}}
            result = await self.react_graph.ainvoke(
                {
                    "sender": send_to,
                    "context": context,
                    "session_id": None,
                    "work_dir": "playground/",
                },
                config=config,
            )
            result_message = ""
            if isinstance(result, dict):
                result_message = str(result.get("message") or "").strip()
            else:
                result_message = str(result).strip()
            if not result_message:
                result_message = "(No output produced)"
            await asyncio.to_thread(
                self.wa_bot.request_send_message,
                send_to,
                f"🤖💬 *CodeChatAgent Response:*\n{result_message}",
                send_to,
            )
        except Exception as e:
            logger.exception("CodeChatAgent execution failed for %s", send_to)
            await asyncio.to_thread(
                self.wa_bot.request_send_message,
                send_to,
                f"❌ *CodeChatAgent Failed:*\n`{type(e).__name__}: {e}`",
                send_to,
            )
