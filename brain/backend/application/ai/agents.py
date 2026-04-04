import logging
import os
from pathlib import Path
from pymongo import MongoClient

from backend.application.ai.ai import AI
from backend.application.ai.tools.websearch import WebSearchTool
from backend.application.ai.tools.time import TimeTool
from backend.application.ai.tools.parse import ParseTool

from backend.application.ai.tools.whatsapp_integration import WhatsAppIntegration
from backend.application.ai.instruction import Instruction
from config.config import Config

from langgraph.graph import StateGraph, START
from langgraph.graph import MessagesState
from langgraph.prebuilt import ToolNode
from langgraph.prebuilt import tools_condition
from langchain_core.messages import HumanMessage, AIMessage, SystemMessage

from langgraph.checkpoint.mongodb import MongoDBSaver
from langchain_core.messages import trim_messages



logger = logging.getLogger('uvicorn.error')

class ChatAgentUseCase:
    def __init__(self):
        self.llm = AI()
        self.websearch_tool = WebSearchTool()
        self.time_tool = TimeTool()
        self.parse_tool = ParseTool()
        self.wa_integration_tool = WhatsAppIntegration()
        self.instruction = Instruction()
        self.mongo_cleint = MongoClient("mongodb://localhost:27017/")
        self.checkpointer = MongoDBSaver(self.mongo_cleint, db_name="agent_memory_db", collection_name="agent_memory_collection")
        self.__init_langsmith()
        self.__list_tool = [
            self.instruction.capability_description,
            self.websearch_tool.websearch_tool(),
            self.time_tool.current_time,
            self.parse_tool.parse_phone_number,
            self.wa_integration_tool.get_user_information,
            self.wa_integration_tool.send_message(),
            self.wa_integration_tool.broadcast_message,
        ]
        self.llm_with_tools = self.__llm(reasoning=True).bind_tools(self.__list_tool)
        self.react_graph = self.__agent_logic()
    
    @staticmethod
    def __token_counter(messages: list) -> int:
        """
        Estimate total token based on character length (~4 char ≈ 1 token).
        """
        return sum(len(str(m.content)) // 4 for m in messages)

    def __chat_model_node(self, state: MessagesState):
        # https://colab.research.google.com/github/langchain-ai/langchain-academy/blob/main/module-2/trim-filter-messages.ipynb#scrollTo=fc40d930-3c1f-47fe-8d2a-ce174873353c
        """
        Preprocessing node: trim conversation history before routing.
        """
        trimmed = trim_messages(
                state["messages"],
                max_tokens=10000,
                strategy="last",
                token_counter=self.__token_counter,
                allow_partial=False,
            )
        return {"messages": trimmed}

    
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

    def __latest_user_text(self, state: MessagesState, max_messages: int = 1) -> str:
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

    def __assistant_context(self, state: MessagesState, max_messages: int = 12) -> list:
        """
        Return a trimmed message window for the tool-capable assistant node.

        Limiting context size reduces token usage while keeping recent dialogue
        available for tool reasoning.
        """
        messages = state.get("messages", [])
        if len(messages) <= max_messages:
            return messages
        return messages[-max_messages:]
        
    # only use for testing (skripsi)
    def __save_process_diagram(self, react_graph) -> None:
        """
        Render the current LangGraph process flow and save it as a PNG image.

        The file is written to the repository `assets/artifacts` directory so it
        can be served by the frontend static route and previewed in the web UI.
        """
        repo_root = Path(__file__).resolve().parents[4]
        output_dir = repo_root / "assets" / "artifacts"
        output_file = output_dir / "chat_agent_process_table.png"

        try:
            png_bytes = react_graph.get_graph().draw_mermaid_png()
        except Exception as err:
            logger.warning("Failed to render process diagram: %s", err)
            return

        try:
            output_dir.mkdir(parents=True, exist_ok=True)
            output_file.write_bytes(png_bytes)
            logger.info("Process diagram saved to: %s", output_file)
        except OSError as err:
            logger.warning("Failed to save process diagram image: %s", err)
    
    # Graph state routing (early filtering)
    def __route_query(self, state: MessagesState, max_latest_user_messages: int = 5) -> str:
        """
        Classify the user intent into direct_chat or assistant at graph start.

        Routing happens before the tool-capable path so simple conversational
        turns can bypass tool/database-style work. Any actionable, ambiguous,
        structured-data, or unusual message must go to assistant.

        Returns:
            str: "direct_chat" or "assistant".
        """
        recent_user_messages = self.__latest_user_text(state, max_messages=max_latest_user_messages).strip()
        if not recent_user_messages:
            return "direct_chat"

        classifier_prompt = (
            "Classify the latest user message into exactly one label: "
            "direct_chat or assistant.\n"
            "Output only one label with no extra text.\n"
            "Rules:\n"
            "1) direct_chat: greeting, thanks, short acknowledgement, or light small talk "
            "with no request for action, lookup, processing, or tool usage.\n"
            "2) assistant: everything else, including:\n"
            "   - any request/question that needs reasoning or an action\n"
            "   - any lookup/retrieval or tool-like intent\n"
            "   - ambiguous or unusual messages\n"
            "   - messages sharing contacts, phone numbers, links, personal data, or "
            "other structured information\n"
            "   - messages asking to send, save, verify, parse, or process information\n"
            "Examples:\n"
            "- 'hai', 'makasih', 'ok sip' => direct_chat\n"
            "- 'ini nomor saya 0812..., tolong simpan', 'cek kontak ini', "
            "'kirim informasi user', 'bantu cari data' => assistant\n"
            "Return only one label with no extra text."
        )

        try:
            classification = self.__llm(reasoning=False).invoke([
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

    def __direct_chat(self, state: MessagesState):
        """
        Handle lightweight conversation without tools and without full history.

        This path is intentionally minimal: it sends only the latest user text
        to the base LLM so casual chat does not trigger tool/database operations
        or large context processing.
        """
        last_message = self.__latest_user_text(state)
        payload = [HumanMessage(content=last_message)] if last_message else state["messages"][-1:]
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

    def __assistant(self, state: MessagesState):
        """
        Run the tool-capable assistant node with bounded conversation context.

        The system rule is prepended each turn, while prior messages are
        trimmed to control token growth and keep reasoning focused.
        """
        assistant_messages = self.__assistant_context(state)
        response = self.__llm_with_tools().invoke([self.__system_rule()] + assistant_messages)
        return {"messages": [response]}

    def __llm_with_tools(self):
        """
        Return the pre-bound LLM instance that supports tool calling.

        Binding once in __init__ prevents repeated re-binding overhead on every
        assistant step.
        """
        return self.llm_with_tools

    def __system_rule(self):
        """
        Build the system instruction message for the tool-capable assistant.

        This keeps the policy source centralized so updates are applied
        consistently on every assistant turn.
        """
        build_msg = "SYSTEM RULES AND INSTRUCTIONS:\n" + self.instruction.system_instruction
        capability_description = self.instruction.capability_description()
        if capability_description:
            build_msg += "\n\nCAPABILITY REFERENCE:\n" + capability_description

        sys_msg = SystemMessage(content=build_msg)
        return sys_msg

    def __agent_logic(self):
        """
        Build and compile the LangGraph workflow.

        Flow overview:
        - START -> route_query (early intent filter)
        - route_query -> direct_chat or assistant
        - assistant -> tools only when tool calls are requested
        - tools -> assistant loop until no more tool calls

        Why this design:
        - direct_chat keeps simple messages cheap and fast
        - assistant path is reserved for tasks, lookups, and structured inputs
        - tools are executed only on explicit tool-call outputs
        """
        # Build the state graph container.
        builder = StateGraph(MessagesState)
        
        

        # Register worker nodes.
        # add chat models
        builder.add_node("chat_model", self.__chat_model_node)
        
        # assistant decides whether tools are needed; tools executes tool calls.
        builder.add_node("assistant", self.__assistant)
        # ToolNode runs only the configured tool functions.
        builder.add_node("tools", ToolNode(self.__list_tool))
        builder.add_node("direct_chat", self.__direct_chat)
        
        builder.add_edge(START, "chat_model")

        # Route from START using early filtering.
        builder.add_conditional_edges(
            "chat_model",
            self.__route_query,
            {
                "direct_chat": "direct_chat",  # Lightweight social/chat path.
                "assistant": "assistant"       # Actionable or ambiguous path.
            }
        )
        
        # Route assistant output to tools only for tool-call responses.
        builder.add_conditional_edges(
            "assistant",
            # tools_condition: tool call -> tools node, otherwise graph ends.
            tools_condition,
        )
        # Return to assistant after each tool execution for final response.
        builder.add_edge("tools", "assistant")
        react_graph = builder.compile(checkpointer=self.checkpointer)
        self.__save_process_diagram(react_graph)
        
        return react_graph

    def __extract_ai_content(self, messages: list) -> str:
        """
        Extract the latest non-empty assistant text from graph message state.

        Tool loops can produce multiple AI messages, including empty/tool-call
        placeholders, so this method walks backward to return the final usable
        answer string.
        """
        ai_messages = [message for message in messages if isinstance(message, AIMessage)]
        if not ai_messages:
            return ''

        for ai_message in reversed(ai_messages):
            content = ai_message.content
            if isinstance(content, str):
                text = content.strip()
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
                if text:
                    return text
                continue

            text = str(content).strip()
            if text and text != '[]':
                return text

        return ''
        
    
    async def ChatAgent(self, messages: str, number: str):
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
            input_messages = []
            config = {"configurable": {"thread_id": number}}
            input_messages.append(SystemMessage(content=f"Current user identifier: {number}"))
            input_messages.append(HumanMessage(content=messages))

            result = await self.react_graph.ainvoke({"messages": input_messages}, config=config)
            results = self.__extract_ai_content(result.get('messages', []))

            self.wa_integration_tool.request_send_message(number, results)
        except Exception as e:
            self.wa_integration_tool.request_send_message(number, f"Error: {e}")

