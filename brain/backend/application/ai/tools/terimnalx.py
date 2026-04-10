import asyncio
import os
import re
import subprocess
import threading
from pathlib import Path
from langchain_core.tools import StructuredTool
from claude_agent_sdk import (
    query, ClaudeAgentOptions,
    AssistantMessage, ResultMessage, TextBlock
)

from backend.application.ai.tools.models import CommandInput, WorkerTaskInput
from backend.application.ai.tools.whatsapp_integration import WhatsAppIntegration
from config.config import Config, BASE_DIR

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

        candidate = Path(raw).expanduser()
        if not candidate.is_absolute():
            candidate = self._repo_root / candidate

        try:
            resolved = candidate.resolve(strict=False)

            if resolved.exists() and not resolved.is_dir():
                raise OSError(f"Path is not a directory: {resolved}")

            resolved.mkdir(parents=True, exist_ok=True)
            return str(resolved)
        except OSError:
            # Fallback for invalid/unsafe paths (e.g., invalid Windows path chars).
            fallback = (self._playground_root / self.__task_slug(task)).resolve()
            fallback.mkdir(parents=True, exist_ok=True)
            return str(fallback)

    def _ossync(self):
        """Sync important env vars from Config to os.environ for subprocesses and agent SDK."""
        if Config.ANTHROPIC_BASE_URL:
            os.environ['ANTHROPIC_BASE_URL'] = Config.ANTHROPIC_BASE_URL

        if Config.ANTHROPIC_AUTH_TOKEN:
            os.environ['ANTHROPIC_AUTH_TOKEN'] = Config.ANTHROPIC_AUTH_TOKEN

        if Config.ANTHROPIC_API_KEY:
            os.environ['ANTHROPIC_API_KEY'] = Config.ANTHROPIC_API_KEY
    
    def _run_agent_in_thread(self, sender: str, prompt: str, work_dir: str = ".") -> str:
        """Run the async agent logic in a separate thread and wait for the result."""
        result_container = []
        exception_container = []

        def thread_target():
            loop = asyncio.new_event_loop()
            asyncio.set_event_loop(loop)
            try:
                result = loop.run_until_complete(self._async_agent(sender, prompt, work_dir))
                result_container.append(result)
            except Exception as e:
                exception_container.append(e)
            finally:
                loop.close()

        t = threading.Thread(target=thread_target)
        t.start()
        t.join()

        if exception_container:
            err = exception_container[0]
            return f"[ERROR] Worker execution failed: {type(err).__name__}: {err}"

        return result_container[0] if result_container else ""
        
    # Spinner frames ditampilkan bergantian tiap step untuk kesan "hidup"
    _SPINNER_FRAMES = ["⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"]

    # Pasangan (kata-kunci-dalam-teks, label, emoji) untuk konteks otomatis
    _STEP_LABELS = [
        (["write", "creat", "generat", "saving", "output"],   "Writing",   "📝"),
        (["read", "open", "load", "fetch"],                    "Reading",   "📖"),
        (["run", "exec", "bash", "command", "install"],        "Running",   "⚡"),
        (["analyz", "check", "inspect", "review", "scan"],    "Analyzing", "🔍"),
        (["test", "verif", "valid"],                           "Testing",   "🧪"),
        (["debug", "fix", "error", "fail", "patch"],           "Debugging", "🐛"),
        (["plan", "think", "reason", "decid", "step"],         "Thinking",  "💭"),
        (["edit", "modif", "updat", "refactor"],               "Editing",   "✏️"),
        (["search", "find", "look", "glob", "list"],           "Searching", "🗂️"),
        (["done", "complet", "finish", "success"],             "Done",      "✅"),
    ]
    _MAX_PREVIEW_CHARS = 180

    def __classify_step(self, text: str) -> tuple[str, str]:
        """Return (label, emoji) based on keywords found in text."""
        lower = text.lower()
        for keywords, label, emoji in self._STEP_LABELS:
            if any(kw in lower for kw in keywords):
                return label, emoji
        return "Working", "🔧"

    def __format_worker_update(self, step: int, text: str) -> str:
        """Build an interactive worker progress message for WhatsApp."""
        spinner = self._SPINNER_FRAMES[step % len(self._SPINNER_FRAMES)]
        label, emoji = self.__classify_step(text)

        # Potong teks panjang agar rapi di chat
        preview = text.strip().replace("\n", " ")
        if len(preview) > self._MAX_PREVIEW_CHARS:
            preview = preview[:self._MAX_PREVIEW_CHARS].rstrip() + "…"

        lines = [
            f"{spinner} *Worker — Step {step}* {emoji} _{label}_",
            f"```{preview}```",
        ]
        return "\n".join(lines)

    async def _async_agent(self, sender: str, prompt: str, work_dir: str) -> str:
        """Core async agent logic."""
        if not prompt.strip():
            raise ValueError("[ERROR] No task provided for worker execution.")
        if not work_dir.strip():
            raise ValueError("[ERROR] No workspace provided for worker execution.")
        if Config.OLLAMA_MODEL is None or Config.OLLAMA_MODEL.strip() == "":
            raise ValueError("[ERROR] Ollama model not configured. Please set OLLAMA_MODEL in environment variables.")
        safe_work_dir = self.__safe_worker_dir(task=prompt, work_dir=work_dir)
        options = ClaudeAgentOptions(
            model=Config.OLLAMA_MODEL,
            allowed_tools=["Bash", "Read", "Write", "Edit", "Glob", "LS"],
            permission_mode="acceptEdits",
            cwd=safe_work_dir,
        )

        output_parts: list[str] = []
        step = 0

        async for message in query(prompt=prompt, options=options):
            if isinstance(message, AssistantMessage):
                for block in message.content:
                    if isinstance(block, TextBlock):
                        step += 1
                        output_parts.append(block.text)
                        update_msg = self.__format_worker_update(step, block.text)
                        self.wa_bot.request_send_message(sender, update_msg, sender)
            elif isinstance(message, ResultMessage) and message.subtype == "success":
                if message.result:
                    output_parts.append(message.result)

        return "\n".join(output_parts)


    
    def delegate_to_worker(self, sender: str):
        """
        delegate_to_worker is used for more complex tasks that may require multi-step reasoning,
        file access, or persistent working-directory context.
        """
        def _worker(task: str, work_dir: str = "playground/") -> str:
            safe_work_dir = self.__safe_worker_dir(task=task, work_dir=work_dir)
            self.wa_bot.request_send_message(
                sender,
                f"🤖🛠️ *Worker Executing task:* `{task}`\n📁 Workspace: `{safe_work_dir}`",
                sender
            )
            return self._run_agent_in_thread(sender, task, work_dir=safe_work_dir)


        return StructuredTool.from_function(
            func=_worker,
            name="delegate_to_worker",
            description=(
                "Execute complex agent work with planning, tool orchestration, and iterative reasoning."
                "\n\nAI level instruction (routing priority):"
                "\n- Treat this tool as preferred default for non-trivial engineering tasks."
                "\n- This is the advanced worker, and it is an upgrade path from `simple_exec`."
                "\n- Choose `delegate_to_worker` when the request needs multi-step execution,"
                " code generation/editing, repository exploration, debugging loops, or chained tasks."
                "\n- Use this tool when task completion depends on agent decisions, not only a single shell command."
                "\n- The worker runs through `ClaudeAgentOptions` with scoped tools"
                " (`Bash`, `Read`, `Write`, `Edit`, `Glob`, `LS`) and persistent `cwd` context."
                "\n- Put the objective, constraints, and expected result in `task` so the worker can plan steps."
                "\n- Workspace policy: if user has not provided workspace location, ask once before execution."
                "\n- If user says AI may decide (e.g., 'bebas', 'terserah', 'tentukan sendiri'),"
                " create/select a project folder under `playground/` and use it as `work_dir`."
                "\n- Folder name should follow the project/task name in a safe slug format."
                "\n- Never let auto-selected workspace escape `playground/` (no `..`, absolute path, or parent traversal)."
                "\n- Do NOT route trivial one-shot shell commands here; use `simple_exec` for those."
                "\n- Default `work_dir` to `playground/` if not specified."
                "\n\nArgs:"
                "\n- task: Goal-oriented task description for the worker agent."
                "\n- work_dir: Optional workspace path. If omitted, ask location first;"
                " if user delegates decision, create/use project folder inside `playground/` only."
                "\n\nReturns:"
                "\n- str: Consolidated worker output after agent execution completes."
            ),
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
        

    
    