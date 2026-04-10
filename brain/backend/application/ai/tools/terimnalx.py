import asyncio
import os
import subprocess
import threading
from typing import Any
from langchain_core.tools import StructuredTool, tool
from claude_agent_sdk import (
    query, ClaudeAgentOptions,
    AssistantMessage, ResultMessage, TextBlock
)

from backend.application.ai.tools.models import CommandInput
from backend.application.ai.tools.whatsapp_integration import WhatsAppIntegration
from config.config import Config

class TerminalX:
    MAX_SIMPLE_EXEC_OUTPUT_CHARS = 12000
    MAX_SIMPLE_EXEC_HEAD_LINES = 120
    MAX_SIMPLE_EXEC_TAIL_LINES = 40

    def __init__(self):
        self.wa_bot = WhatsAppIntegration()
        self._ossync()

    def _ossync(self):
        if Config.ANTHROPIC_BASE_URL:
            os.environ['ANTHROPIC_BASE_URL'] = Config.ANTHROPIC_BASE_URL

        if Config.ANTHROPIC_AUTH_TOKEN:
            os.environ['ANTHROPIC_AUTH_TOKEN'] = Config.ANTHROPIC_AUTH_TOKEN

        if Config.ANTHROPIC_API_KEY:
            os.environ['ANTHROPIC_API_KEY'] = Config.ANTHROPIC_API_KEY
    
    def _run_agent_in_thread(self, prompt: str, work_dir: str = ".") -> str:
        """Jalankan async agent di thread baru agar aman di dalam LangGraph event loop."""
        result_container = []
        exception_container = []

        def thread_target():
            loop = asyncio.new_event_loop()
            asyncio.set_event_loop(loop)
            try:
                result = loop.run_until_complete(self._async_agent(prompt, work_dir))
                result_container.append(result)
            except Exception as e:
                exception_container.append(e)
            finally:
                loop.close()

        t = threading.Thread(target=thread_target)
        t.start()
        t.join()

        if exception_container:
            raise exception_container[0]

        return result_container[0] if result_container else ""
        
    async def _async_agent(self, prompt: str, work_dir: str) -> str:
        """Core async agent logic."""
        options = ClaudeAgentOptions(
            model="qwen3-coder",
            allowed_tools=["Bash", "Read", "Write", "Edit", "Glob", "LS"],
            permission_mode="acceptEdits",
            cwd=work_dir,
        )

        output_parts = []

        async for message in query(prompt=prompt, options=options):
            if isinstance(message, AssistantMessage):
                for block in message.content:
                    if isinstance(block, TextBlock):
                        output_parts.append(block.text)
            elif isinstance(message, ResultMessage) and message.subtype == "success":
                if message.result:
                    output_parts.append(message.result)

        return "\n".join(output_parts)

    def agent_execute_task(self, task: str, work_dir: str = ".") -> str:
        """
        Jalankan task kompleks yang butuh akses file dan terminal sekaligus.
        Gunakan untuk task multi-step seperti: setup project, install + config, dll.
        """
        return self._run_agent_in_thread(task, work_dir=work_dir)

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
                "Execute a single low-context terminal command."
                "\n\nAI instruction:"
                "\n- Use this method for one-shot commands that do not require multi-step reasoning,"
                " file edits, or persistent working-directory context."
                "\n- Keep the command explicit and focused on a single action."
                "\n- Long command output may be truncated to keep chat context stable."
                "\n- For setup, configuration, or chained workflows, use `agent_execute_task` instead."
                "\n- Default to `playground/`. Check existence before creating."
                "\n- Never create outside `playground/` unless user explicitly requests another path."
                "\n\nArgs:"
                "\n- command: The terminal command to execute."
                "\n\nReturns:"
                "\n- str: Standard output from the executed command."
            ),
            args_schema=CommandInput
        )
        

    
    