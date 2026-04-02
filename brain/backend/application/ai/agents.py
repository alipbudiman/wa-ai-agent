import logging
import os

from pymongo import MongoClient

from backend.application.ai.ai import AI
from backend.application.ai.tools.websearch import WebSearchTool
from backend.application.ai.tools.time import TimeTool
from backend.application.ai.tools.parse import ParseTool

from backend.application.ai.instruction import TOOLS_SYSTEM_INSTRUCTION
from backend.application.ai.tools.whatsapp_integration import WhatsAppIntegration
from config.config import Config

from langgraph.graph import StateGraph, START
from langgraph.graph import MessagesState
from langgraph.prebuilt import ToolNode
from langgraph.prebuilt import tools_condition
from langchain_core.messages import HumanMessage, AIMessage, SystemMessage

from langgraph.checkpoint.mongodb import MongoDBSaver


logger = logging.getLogger('uvicorn.error')


class ChatAgentUseCase:
    def __init__(self):
        self.llm = AI(api_key=Config.OPENROUTER_API_KEY).LLM()
        self.websearch_tool = WebSearchTool(ai=self.llm)
        self.time_tool = TimeTool()
        self.parse_tool = ParseTool()
        self.wa_integration_tool = WhatsAppIntegration()
        self.mongo_cleint = MongoClient("mongodb://localhost:27017/")
        self.checkpointer = MongoDBSaver(self.mongo_cleint, db_name="agent_memory_db", collection_name="agent_memory_collection")
        self.__init_langsmith()
        self.__list_tool = [
            self.websearch_tool.websearch,
            self.time_tool.current_time,
            self.parse_tool.parse_phone_number,
            self.wa_integration_tool.get_user_information,
            self.wa_integration_tool.send_message(),
            self.wa_integration_tool.help_menu()

        ]
    
    def __init_langsmith(self):
        if Config.LANGSMITH_API_KEY:
            os.environ['LANGSMITH_API_KEY'] = Config.LANGSMITH_API_KEY
            os.environ['LANGSMITH_TRACING'] = 'true'
            os.environ["LANGSMITH_PROJECT"] = Config.LANGSMITH_PROJECT_NAME

    def __assistant(self, state: MessagesState):
        """The assistant node that processes messages and decides when to call tools."""
        return {"messages": [self.__llm_with_tools().invoke([self.__system_rule()] + state["messages"])]}

    def __llm_with_tools(self):
        llm_with_tools = self.llm.bind_tools(self.__list_tool, parallel_tool_calls=False)
        return llm_with_tools

    def __system_rule(self):
        sys_msg = SystemMessage(content=TOOLS_SYSTEM_INSTRUCTION)
        return sys_msg

    def __tools(self):
        # Graph
        builder = StateGraph(MessagesState)

        # Define nodes: these do the work
        builder.add_node("assistant", self.__assistant)
        builder.add_node("tools", ToolNode(self.__list_tool))

        # Define edges: these determine how the control flow moves
        builder.add_edge(START, "assistant")
        builder.add_conditional_edges(
            "assistant",
            # If the latest message (result) from assistant is a tool call -> tools_condition routes to tools
            # If the latest message (result) from assistant is a not a tool call -> tools_condition routes to END
            tools_condition,
        )
        builder.add_edge("tools", "assistant")
        react_graph = builder.compile(checkpointer=self.checkpointer)
        
        return react_graph

    def __extract_ai_content(self, messages: list) -> str:
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
    
    def ChatAgent(self, messages: str, number: str):
        """ A simple chat agent that can call tools based on user messages.It uses a state graph to manage the flow of messages and tool calls."""
        input_messages = []
        config = {"configurable": {"thread_id": number}}
        react_graph = self.__tools()
        contact_info = self.wa_integration_tool.get_user_information(number)
        if contact_info.success and contact_info.data is not None and contact_info.data.Found:
            input_messages.append(SystemMessage(content=f"Chatting with {contact_info.data.FullName} / {contact_info.data.PushName} ({number})"))
        input_messages.append(HumanMessage(content=messages))

        result = react_graph.invoke({"messages": input_messages}, config=config)
        return self.__extract_ai_content(result.get('messages', []))