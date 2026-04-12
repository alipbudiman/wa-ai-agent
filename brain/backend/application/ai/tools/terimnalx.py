import asyncio
import logging
import os
import re
import shutil
import subprocess
from pathlib import Path
from langchain_core.tools import StructuredTool
from claude_agent_sdk import (
    query, ClaudeAgentOptions,
    AssistantMessage, ResultMessage, TextBlock,
    ToolUseBlock, ToolResultBlock,
    UserMessage, SystemMessage, ThinkingBlock,
    PermissionResultAllow,
)

# --- Monkey-patch SDK parser for Ollama compatibility ---
# Ollama doesn't include 'signature' field in thinking blocks (Claude-specific).
# Patch parse_message to use .get() with default instead of direct key access.
def _patch_sdk_message_parser():
    try:
        from claude_agent_sdk._internal import message_parser
        _original_parse = message_parser.parse_message

        def _patched_parse_message(data):
            if isinstance(data, dict):
                msg = data.get("message", {})
                if isinstance(msg, dict) and isinstance(msg.get("content"), list):
                    for block in msg["content"]:
                        if isinstance(block, dict) and block.get("type") == "thinking":
                            block.setdefault("signature", "")
            return _original_parse(data)

        message_parser.parse_message = _patched_parse_message
    except Exception:
        pass

_patch_sdk_message_parser()

from backend.application.ai.tools.models import CommandInput, WorkerTaskInput
from backend.application.ai.tools.whatsapp_integration import WhatsAppIntegration
from config.config import Config, BASE_DIR


logger = logging.getLogger("uvicorn.error")

class TerminalX:
    MAX_SIMPLE_EXEC_OUTPUT_CHARS = 12000
    MAX_SIMPLE_EXEC_HEAD_LINES = 120
    MAX_SIMPLE_EXEC_TAIL_LINES = 40

    def __init__(self):
        self.wa_bot = WhatsAppIntegration()
        self._repo_root = Path(BASE_DIR).resolve()
        self._playground_root = (self._repo_root / "playground").resolve()
        self._ossync()

    def __normalize_workspace_hint(self, value: str | None) -> str:
        if not value:
            return ""
        return " ".join(value.strip().strip('`\"\'').lower().split())

    def __is_auto_workspace_hint(self, value: str | None) -> bool:
        hint = self.__normalize_workspace_hint(value)
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

    def __task_slug(self, task: str) -> str:
        slug = re.sub(r"[^a-z0-9]+", "-", task.lower()).strip("-")
        if not slug:
            return "worker-task"
        return slug[:64]

    def __is_windows_abs_path(self, value: str) -> bool:
        return bool(re.match(r"^[a-zA-Z]:[\\/]", (value or "").strip()))

    def __should_delegate_from_simple_exec(self, command: str) -> tuple[bool, list[str]]:
        """Detect commands that are too complex for simple_exec one-shot mode."""
        cmd = (command or "").strip()
        lower = cmd.lower()
        reasons: list[str] = []

        if not cmd:
            return False, reasons

        if "\n" in cmd:
            reasons.append("multiline command")

        if len(cmd) > 220:
            reasons.append("long command")

        if any(token in cmd for token in ["&&", "||", ";"]):
            reasons.append("chained operations")

        if any(token in cmd for token in [">>", " >", "<", "|", "<<"]):
            reasons.append("redirection or piping")

        if lower.count("echo ") >= 3:
            reasons.append("bulk file-generation pattern")

        if any(token in lower for token in ["cat <<", "python - <<", "npm init", "npm create", "git clone"]):
            reasons.append("project setup or scripted generation")

        creates_code_file = any(ext in lower for ext in [
            ".html", ".css", ".js", ".ts", ".tsx", ".jsx", ".py", ".go", ".json", ".md"
        ]) and any(token in cmd for token in [">", ">>"])
        if creates_code_file:
            reasons.append("code file creation/edit")

        return (len(reasons) > 0), reasons

    def __safe_worker_dir(self, task: str, work_dir: str | None) -> str:
        """Resolve worker directory to a valid existing folder path.

        This guards Claude SDK `cwd` against invalid values (WinError 267).
        """
        raw = (work_dir or "").strip().strip('`\"\'')

        # Default and AI-auto choices are always scoped to playground.
        if not raw or raw in {".", "./", ".\\", "playground", "playground/", "playground\\"}:
            self._playground_root.mkdir(parents=True, exist_ok=True)
            return str(self._playground_root)

        if self.__is_auto_workspace_hint(raw):
            auto_dir = (self._playground_root / self.__task_slug(task)).resolve()
            auto_dir.mkdir(parents=True, exist_ok=True)
            return str(auto_dir)

        normalized_raw = raw.replace("\\", "/")
        playground_base = self._playground_root.resolve()

        # If user sends a Windows-style absolute path while runtime is Linux container,
        # map safely to playground so files persist on shared Docker volume.
        if os.name != "nt" and self.__is_windows_abs_path(raw):
            lower = normalized_raw.lower()
            marker = "/playground"
            idx = lower.find(marker)
            if idx >= 0:
                suffix = normalized_raw[idx + len(marker):].lstrip("/")
                candidate = self._playground_root / suffix if suffix else self._playground_root
            else:
                candidate = self._playground_root / self.__task_slug(task)
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
            if not Path(normalized_raw).is_absolute() and not self.__is_windows_abs_path(raw):
                resolved.relative_to(playground_base)

            if resolved.exists() and not resolved.is_dir():
                raise OSError(f"Path is not a directory: {resolved}")

            resolved.mkdir(parents=True, exist_ok=True)
            return str(resolved)
        except (OSError, ValueError):
            # Fallback for invalid/unsafe paths (e.g., invalid Windows path chars).
            fallback = (self._playground_root / self.__task_slug(task)).resolve()
            fallback.mkdir(parents=True, exist_ok=True)
            return str(fallback)

    def __build_worker_prompt(self, task: str, work_dir: str) -> str:
        clean_task = (task or "").strip()
        return (
            "WORKSPACE EXECUTION RULES:\n"
            f"- Active workspace: {work_dir}\n"
            f"- Shared playground root: {self._playground_root}\n"
            "- Save all generated files inside the active workspace unless user explicitly requests another location.\n"
            "- After creating or editing files, verify persistence by running LS and reading the saved file(s).\n"
            "- In final output, list concrete saved file paths.\n\n"
            "USER TASK:\n"
            f"{clean_task}"
        )

    def __delegate_tool_description(self) -> str:
        repo_root = str(self._repo_root)
        playground_root = str(self._playground_root)
        return (
            "Execute complex agent work with planning, tool orchestration, and iterative reasoning."
            "\n\nAI level instruction (routing priority):"
            "\n- Treat this tool as preferred default for non-trivial engineering tasks."
            "\n- This is the advanced worker, and it is an upgrade path from `simple_exec`."
            "\n- Choose `delegate_to_worker` when the request needs multi-step execution,"
            " code generation/editing, repository exploration, debugging loops, or chained tasks."
            "\n- Use this tool when task completion depends on agent decisions, not only a single shell command."
            "\n- The worker runs through `ClaudeAgentOptions` with scoped tools"
            " (`Bash`, `Read`, `Write`, `Edit`, `Glob`, `LS`) and persistent `cwd` context."
            f"\n- Runtime repo root: `{repo_root}`."
            f"\n- Runtime shared playground root: `{playground_root}`."
            f"\n- Relative `work_dir` is resolved under playground (example: `my-task` -> `{playground_root}/my-task`)."
            "\n- Put the objective, constraints, and expected result in `task` so the worker can plan steps."
            "\n- Workspace policy: if user has not provided workspace location, ask once before execution."
            "\n- If user says AI may decide (e.g., 'bebas', 'terserah', 'tentukan sendiri'),"
            " create/select a project folder under `playground/` and use it as `work_dir`."
            "\n- Folder name should follow the project/task name in a safe slug format."
            "\n- Never let auto-selected workspace escape `playground/` (no `..`, absolute path, or parent traversal)."
            "\n- For file-creation tasks, verify saved files with `LS` and `Read` before finishing."
            "\n- Do NOT route trivial one-shot shell commands here; use `simple_exec` for those."
            "\n- Default `work_dir` to `playground/` if not specified."
            "\n\nArgs:"
            "\n- task: Goal-oriented task description for the worker agent."
            "\n- work_dir: Optional workspace path. If omitted, ask location first;"
            " if user delegates decision, create/use project folder inside `playground/` only."
            "\n\nReturns:"
            "\n- str: Consolidated worker output after agent execution completes."
        )

    def _ossync(self):
        """Sync important env vars from Config to os.environ for subprocesses and agent SDK."""
        if Config.ANTHROPIC_BASE_URL:
            os.environ['ANTHROPIC_BASE_URL'] = Config.ANTHROPIC_BASE_URL

        if Config.ANTHROPIC_AUTH_TOKEN:
            os.environ['ANTHROPIC_AUTH_TOKEN'] = Config.ANTHROPIC_AUTH_TOKEN

        if Config.ANTHROPIC_API_KEY:
            os.environ['ANTHROPIC_API_KEY'] = Config.ANTHROPIC_API_KEY
    
    WORKER_TIMEOUT_SECONDS = 600  # 10-minute hard cap for worker execution
    PROGRESS_SEND_TIMEOUT = 10   # 10-second cap for each progress notification

    async def __send_worker_progress(self, sender: str, step: int, title: str, detail: str = "") -> None:
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

    async def _async_agent(self, sender: str, prompt: str, work_dir: str) -> str:
        """Core async agent logic with overall timeout protection."""
        try:
            return await asyncio.wait_for(
                self._async_agent_inner(sender, prompt, work_dir),
                timeout=self.WORKER_TIMEOUT_SECONDS,
            )
        except asyncio.TimeoutError:
            logger.error("Worker timed out after %ds for %s", self.WORKER_TIMEOUT_SECONDS, sender)
            return f"[ERROR] Worker execution timed out after {self.WORKER_TIMEOUT_SECONDS // 60} minutes."
        except Exception as e:
            logger.exception("Worker execution failed for %s", sender)
            return f"[ERROR] Worker execution failed: {type(e).__name__}: {e}"

    async def _async_agent_inner(self, sender: str, prompt: str, work_dir: str) -> str:
        """Inner async agent logic — streams SDK messages and sends progress."""
        if not prompt.strip():
            raise ValueError("[ERROR] No task provided for worker execution.")
        if not work_dir.strip():
            raise ValueError("[ERROR] No workspace provided for worker execution.")
        if Config.OLLAMA_MODEL is None or Config.OLLAMA_MODEL.strip() == "":
            raise ValueError("[ERROR] Ollama model not configured. Please set OLLAMA_MODEL in environment variables.")
        safe_work_dir = self.__safe_worker_dir(task=prompt, work_dir=work_dir)

        async def _auto_approve_tool(tool_name, tool_input, context):
            """Auto-approve all tool permission requests."""
            logger.info("[Worker:%s] Auto-approving tool: %s", sender, tool_name)
            return PermissionResultAllow()

        async def _prompt_stream():
            """Wrap string prompt as AsyncIterable for streaming mode (required by can_use_tool)."""
            yield {
                "type": "user",
                "message": {"role": "user", "content": prompt},
                "parent_tool_use_id": None,
                "session_id": "",
            }

        options = ClaudeAgentOptions(
            model=Config.OLLAMA_MODEL,
            allowed_tools=["Bash", "Read", "Write", "Edit", "Glob", "LS", "TodoWrite", "TodoRead"],
            permission_mode="default",
            can_use_tool=_auto_approve_tool,
            cwd=safe_work_dir,
        )

        logger.info("[Worker:%s] Starting SDK query (model=%s, cwd=%s, permission=default+auto_approve)", sender, Config.OLLAMA_MODEL, safe_work_dir)

        output_parts: list[str] = []
        step = 0
        sent_progress = False

        async for message in query(prompt=_prompt_stream(), options=options):
            logger.info("[Worker:%s] SDK message type=%s step=%d", sender, type(message).__name__, step)
            if isinstance(message, AssistantMessage):
                for block in message.content:
                    if isinstance(block, TextBlock):
                        text = (block.text or "").strip()
                        if not text:
                            continue
                        step += 1
                        sent_progress = True
                        output_parts.append(text)
                        await self.__send_worker_progress(sender, step, "Assistant Response", text)
                    elif isinstance(block, ToolUseBlock):
                        step += 1
                        sent_progress = True
                        tool_name = (block.name or "tool").strip()
                        await self.__send_worker_progress(sender, step, f"Using Tool: {tool_name}", str(block.input))
                    elif isinstance(block, ThinkingBlock):
                        step += 1
                        sent_progress = True
                        await self.__send_worker_progress(sender, step, "Thinking", block.thinking)
            elif isinstance(message, UserMessage):
                blocks = message.content if isinstance(message.content, list) else []
                for block in blocks:
                    if isinstance(block, ToolResultBlock):
                        step += 1
                        sent_progress = True
                        preview = ""
                        if block.content is not None:
                            preview = str(block.content)
                        if block.is_error:
                            await self.__send_worker_progress(sender, step, "Tool Result Error", preview)
                        else:
                            await self.__send_worker_progress(sender, step, "Tool Result", preview)
            elif isinstance(message, SystemMessage):
                subtype = (message.subtype or "").strip()
                if subtype in {"task_started", "task_progress", "task_notification"}:
                    step += 1
                    sent_progress = True
                    description = ""
                    if isinstance(message.data, dict):
                        description = str(
                            message.data.get("description")
                            or message.data.get("summary")
                            or message.data.get("status")
                            or ""
                        )
                    await self.__send_worker_progress(sender, step, f"Task {subtype}", description)
            elif isinstance(message, ResultMessage) and message.subtype == "success":
                if message.result:
                    result_text = str(message.result).strip()
                    if result_text:
                        if not output_parts or output_parts[-1] != result_text:
                            output_parts.append(result_text)
                        if not sent_progress:
                            step += 1
                            await self.__send_worker_progress(sender, step, "Completed", result_text)

        logger.info("[Worker:%s] SDK query loop finished. total_steps=%d, output_parts=%d", sender, step, len(output_parts))
        return "\n".join(output_parts)


    
    async def _worker_background(self, sender: str, task: str, safe_work_dir: str, worker_prompt: str) -> None:
        """Background worker — runs independently and sends results via WhatsApp."""
        try:
            result = await self._async_agent(sender, worker_prompt, work_dir=safe_work_dir)
            # Send final result directly via WhatsApp
            if result and result.strip():
                summary = result.strip()
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

    def delegate_to_worker(self, sender: str):
        """
        delegate_to_worker is used for more complex tasks that may require multi-step reasoning,
        file access, or persistent working-directory context.
        
        The worker runs as a background task and reports results directly via WhatsApp.
        The main agent continues immediately without waiting.
        """
        async def _worker_async(task: str, work_dir: str = "playground/") -> str:
            safe_work_dir = self.__safe_worker_dir(task=task, work_dir=work_dir)
            worker_prompt = self.__build_worker_prompt(task=task, work_dir=safe_work_dir)

            # Send start notification
            try:
                await asyncio.wait_for(
                    asyncio.to_thread(
                        self.wa_bot.request_send_message,
                        sender,
                        f"🤖🛠️ *Worker Executing task:* `{task}`\n📁 Workspace: `{safe_work_dir}`",
                        sender,
                    ),
                    timeout=self.PROGRESS_SEND_TIMEOUT,
                )
            except (asyncio.TimeoutError, Exception) as exc:
                logger.warning("Worker start notification failed for %s: %s", sender, exc)

            # Launch worker as background task — do NOT await it
            asyncio.create_task(
                self._worker_background(sender, task, safe_work_dir, worker_prompt),
                name=f"worker-{sender}-{task[:30]}",
            )

            # Return immediately so main agent can continue
            return (
                f"Worker task has been dispatched in the background.\n"
                f"Task: {task}\n"
                f"Workspace: {safe_work_dir}\n"
                f"The worker will send progress updates and final results directly via WhatsApp. "
                f"You may continue with other tasks."
            )

        def _worker_sync(task: str, work_dir: str = "playground/") -> str:
            """Sync fallback — should not be called when coroutine is available."""
            return "[ERROR] Worker must be called asynchronously. Check tool node configuration."

        return StructuredTool.from_function(
            func=_worker_sync,
            coroutine=_worker_async,
            name="delegate_to_worker",
            description=self.__delegate_tool_description(),
            args_schema=WorkerTaskInput
        )

    def __compact_exec_output(self, output_lines: list[str]) -> str:
        """
        Keep command output compact so it fits downstream LLM context.

        Commands like `tasklist` can produce large output that overwhelms
        model context windows and causes follow-up assistant failures.
        """
        if not output_lines:
            return ""

        full_output = "\n".join(output_lines)
        if len(full_output) <= self.MAX_SIMPLE_EXEC_OUTPUT_CHARS:
            return full_output

        head = output_lines[:self.MAX_SIMPLE_EXEC_HEAD_LINES]
        tail = output_lines[-self.MAX_SIMPLE_EXEC_TAIL_LINES:]
        omitted_lines = max(len(output_lines) - len(head) - len(tail), 0)

        compact_lines = head
        if omitted_lines > 0:
            compact_lines = compact_lines + [f"... [truncated {omitted_lines} lines] ..."] + tail

        compact_output = "\n".join(compact_lines)
        if len(compact_output) > self.MAX_SIMPLE_EXEC_OUTPUT_CHARS:
            compact_output = (
                compact_output[: self.MAX_SIMPLE_EXEC_OUTPUT_CHARS].rstrip()
                + "\n... [truncated by character limit] ..."
            )

        return (
            "[TRUNCATED] Command output is too large for chat context.\n"
            f"[stats] total_lines={len(output_lines)} total_chars={len(full_output)}\n"
            + compact_output
        )
    
    def __do_exec(self, command: str, timeout: int = 120) -> str:
        # Auto-inject flags non-interactive
        command = self.__prepare_command(command)

        env = os.environ.copy()
        env.update({
            "DEBIAN_FRONTEND": "noninteractive",
            "CI": "true",
            "GIT_TERMINAL_PROMPT": "0",
            "PIP_YES": "1",
        })

        try:
            process = subprocess.Popen(
                command,
                shell=True,
                stdout=subprocess.PIPE,
                stderr=subprocess.STDOUT,  # gabung stderr ke stdout
                stdin=subprocess.DEVNULL,  # tutup stdin, bypass semua prompt
                text=True,
                bufsize=1,
                env=env,
            )

            output_lines = []
            for line in process.stdout:
                line = line.rstrip()
                output_lines.append(line)

            process.wait(timeout=timeout)
            compact_output = self.__compact_exec_output(output_lines)

            if process.returncode != 0:
                return (
                    f"[ERROR] Exit code: {process.returncode}\n"
                    f"{compact_output or '[NO OUTPUT]'}"
                )

            return compact_output or "[OK] Command executed successfully with no output."

        except subprocess.TimeoutExpired:
            process.kill()
            return f"[ERROR] Command timeout setelah {timeout}s: {command}"

        except Exception as e:
            return f"[ERROR] Exception: {str(e)}"


    def __prepare_command(self, command: str) -> str:
        rules = [
            ("apt-get install", "apt-get install -y"),
            ("apt install",     "apt install -y"),
            ("pip install",     "pip install -q"),
            ("npm install",     "npm install --yes"),
            ("rm ",             "rm -f "),
            ("cp ",             "cp -f "),
            ("mkdir ",          "mkdir -p "),
        ]
        cmd = command.strip()
        for keyword, replacement in rules:
            if keyword in cmd:
                cmd = cmd.replace(keyword, replacement, 1)
        return cmd


    def simple_exec(self, sender: str):
        """
        simple exec is used for one-shot command execution
        that doesn't require multi-step reasoning or file access.
        """
        def _exec(command: str) -> str:
            should_delegate, reasons = self.__should_delegate_from_simple_exec(command)
            if should_delegate:
                reason_text = ", ".join(reasons)
                return (
                    "[ROUTE_TO_WORKER] Command rejected by simple_exec policy. "
                    f"Detected: {reason_text}. "
                    "Use `delegate_to_worker` with a goal-oriented `task` and optional `work_dir`."
                )

            self.wa_bot.request_send_message(
                sender,
                f"🤖 *Executing command:* `{command}`",
                sender
            )
            return self.__do_exec(command)

        return StructuredTool.from_function(
            func=_exec,
            name="simple_exec",
            description=(
                "Execute one explicit shell command without agent planning."
                "\n\nAI level instruction (routing priority):"
                "\n- Use `simple_exec` only for one-shot commands with direct intent and immediate output."
                "\n- Do not use it for coding tasks, multi-step setup, complex troubleshooting,"
                " or workflows requiring file reasoning/editing loops."
                "\n- If the user asks for create code, refactor, analyze files, or solve a chained task,"
                " route to `delegate_to_worker` instead."
                "\n- Hard deny in `simple_exec` for multiline/chained/redirection-heavy commands;"
                " these must be routed to `delegate_to_worker`."
                "\n- Keep command focused on a single action."
                "\n- Output may be truncated to protect chat context stability."
                "\n\nArgs:"
                "\n- command: Terminal command string to execute directly."
                "\n\nReturns:"
                "\n- str: Combined stdout/stderr result from that single command."
            ),
            args_schema=CommandInput
        )
    
    
    def make_archive(self, base_path: str, archive_name: str) -> str:
        """Create a ZIP archive from the specified base path."""
        base_path = Path(base_path).resolve()
        if not base_path.exists() or not base_path.is_dir():
            return f"[ERROR] Base path does not exist or is not a directory: {base_path}"

        archive_path = base_path.parent / f"{archive_name}.zip"
        try:
            shutil.make_archive(
                base_name=str(archive_path.with_suffix("")),
                format="zip",
                root_dir=str(base_path)
            )
            return f"[OK] Archive created at: {archive_path}"
        except Exception as e:
            return f"[ERROR] Failed to create archive: {str(e)}"
    