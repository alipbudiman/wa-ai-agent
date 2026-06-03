import logging
import os
import platform
import shutil
import subprocess
from pathlib import Path

from langchain_core.tools import StructuredTool

from backend.application.ai.tools.filex import FileX
from backend.application.ai.tools.models import CommandInput
from backend.application.ai.tools.whatsapp_integration import WhatsAppIntegration
from config.config import Config, BASE_DIR


logger = logging.getLogger("uvicorn.error")

class TerminalX:
    MAX_SIMPLE_EXEC_OUTPUT_CHARS = 12000
    MAX_SIMPLE_EXEC_HEAD_LINES = 120
    MAX_SIMPLE_EXEC_TAIL_LINES = 40

    def __init__(self):
        self.wa_bot = WhatsAppIntegration()
        self.filex_tool = FileX()
        self.config = Config()
        
        self._repo_root = Path(BASE_DIR).resolve()
        self._playground_root = self.filex_tool._playground_root
    
    def __should_run_worker_from_simple_exec(self, command: str) -> tuple[bool, list[str]]:
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
    
    def do_exec(self, command: str, timeout: int = 120) -> str:
        # Auto-inject flags non-interactive
        command = self.__prepare_command(command)

        env = os.environ.copy()
        env.update({
            "DEBIAN_FRONTEND": "noninteractive",
            "CI": "true",
            "GIT_TERMINAL_PROMPT": "0",
            "PIP_YES": "1",
        })

        process: subprocess.Popen[str] | None = None
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

            output, _ = process.communicate(timeout=timeout)
            output_lines = [
                line.rstrip()
                for line in (output or "").splitlines()
            ]
            compact_output = self.__compact_exec_output(output_lines)

            if process.returncode != 0:
                return (
                    f"[ERROR] Exit code: {process.returncode}\n"
                    f"{compact_output or '[NO OUTPUT]'}"
                )

            return compact_output or "[OK] Command executed successfully with no output."

        except subprocess.TimeoutExpired:
            if process is not None:
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
            should_run_worker, reasons = self.__should_run_worker_from_simple_exec(command)
            if should_run_worker:
                reason_text = ", ".join(reasons)
                return (
                    "[ROUTE_TO_WORKER] Command rejected by simple_exec policy. "
                    f"Detected: {reason_text}. "
                    "Use `run_worker` with a goal-oriented `task` and optional `work_dir`."
                )

            self.wa_bot.request_send_message(
                sender,
                f"🤖 *Executing command:* `{command}`",
                sender
            )
            return self.do_exec(command)

        return StructuredTool.from_function(  # pyright: ignore[reportUnknownMemberType]
            func=_exec,
            name="simple_exec",
            description=(
                "Execute one explicit shell command without agent planning."
                "\n\nAI level instruction (routing priority):"
                "\n- Use `simple_exec` only for one-shot commands with direct intent and immediate output."
                f"\n - Playground folder: {self._playground_root}"
                "\n- Do not use it for coding tasks, multi-step setup, complex troubleshooting,"
                " or workflows requiring file reasoning/editing loops."
                "\n- If the user asks for create code, refactor, analyze files, or solve a chained task,"
                " route to `run_worker` instead."
                "\n- Hard deny in `simple_exec` for multiline/chained/redirection-heavy commands;"
                " these must be routed to `run_worker`."
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
        base_dir = Path(base_path).resolve()
        if not base_dir.exists() or not base_dir.is_dir():
            return f"[ERROR] Base path does not exist or is not a directory: {base_dir}"

        archive_path = base_dir.parent / f"{archive_name}.zip"
        try:
            shutil.make_archive(
                base_name=str(archive_path.with_suffix("")),
                format="zip",
                root_dir=str(base_dir)
            )
            return f"[OK] Archive created at: {archive_path}"
        except Exception as e:
            return f"[ERROR] Failed to create archive: {str(e)}"

    def screen_command(self) -> dict[str, str]:
        """
        Get tmux commands for managing terminal sessions.
        This tool is used for managing terminal sessions, always use if user want to manage terminal sessions
        (like run system in localhost or see list of sessions), and make sure to use the returned commands for any session management.
        All commands are non-interactive and run in detached mode.
        To start a session: use 'new' then 'send' to run a command inside it.
        To check output: use 'capture'. To stop: use 'kill'.
        Always find and use empty ports for new sessions, do not hardcode common ports like 8000 or 8001 to avoid conflicts with existing services.
        If run in tmux, always use `cloudflared tunnel --url http://127.0.0.1:<port> > tunnel.log 2>&1` and return the tunnel URL from tunnel.log.
        the tunnel connection will take a few seconds - minute. Be patience for the tunnel to be ready.
        """
        port_instructions = "always find and use empty ports for new sessions, do not hardcode common ports like 8000 or 8001 to avoid conflicts with existing services."
        tunnel_instructions = "if run in tmux, always use `cloudflared tunnel --url http://127.0.0.1:<port> > tunnel.log 2>&1` (port: your app port). The `.trycloudflare.com` URL will be saved in tunnel.log. Wait a few seconds, then run `cat tunnel.log` to find the public URL."
        tunnel_time_waiting = "the tunnel connection will take a few seconds - minute. Be patience for the tunnel to be ready."
        system = platform.system()
        if system == "Windows":
            wsl_available = shutil.which("wsl") is not None
            os_type = "windows_wsl" if wsl_available else "windows"
        elif system == "Linux":
            os_type = "linux"
        else:
            return {"error": f"Unsupported OS: {system}. This tool only supports Windows and Linux."}

        commands = {}

        if os_type == "linux":
            has_tmux = shutil.which("tmux") is not None
            if has_tmux:
                commands = {
                    "new":     lambda name: f"tmux new-session -d -s {name}",
                    "send":    lambda name, command: f"tmux send-keys -t {name} '{command}' Enter",
                    "capture": lambda name: f"tmux capture-pane -pS - -t {name}",
                    "list":    lambda: "tmux list-sessions",
                    "kill":    lambda name: f"tmux kill-session -t {name}",
                    "rename":  lambda old, new: f"tmux rename-session -t {old} {new}",
                    "exists":  lambda name: f"tmux has-session -t {name} 2>/dev/null && echo yes || echo no",
                }
            else:
                commands = {"error": "tmux tidak terinstall. Jalankan: sudo apt install tmux"}

        elif os_type == "windows_wsl":
            commands = {
                "new":     lambda name: f"wsl tmux new-session -d -s {name}",
                "send":    lambda name, command: f"wsl tmux send-keys -t {name} '{command}' Enter",
                "capture": lambda name: f"wsl tmux capture-pane -pS - -t {name}",
                "list":    lambda: "wsl tmux list-sessions",
                "kill":    lambda name: f"wsl tmux kill-session -t {name}",
                "rename":  lambda old, new: f"wsl tmux rename-session -t {old} {new}",
                "exists":  lambda name: f"wsl tmux has-session -t {name} 2>nul && echo yes || echo no",
            }

        elif os_type == "windows":
            commands = {
                "error":   "WSL tidak ditemukan. tmux membutuhkan WSL di Windows.",
                "install": "wsl --install",
            }
                                 
        return {"os": system, "commands": commands, "important_note": port_instructions + tunnel_instructions + tunnel_time_waiting}
