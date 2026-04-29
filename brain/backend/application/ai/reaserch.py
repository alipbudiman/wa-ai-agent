import logging

from langchain.messages import HumanMessage, RemoveMessage, AIMessage
from langgraph.checkpoint.mongodb import MongoDBSaver
from langgraph.graph import END, START, MessagesState, StateGraph
from typing import Literal

from backend.application.ai.ai import AI
from backend.infrastructure.database.database import MongoDB
from backend.application.ai.tools.models import LevelSystem
from backend.application.ai.tools.whatsapp_integration import WhatsAppIntegration
from backend.application.ai.agents import ChatAgentUseCase

from backend.application.ai.instruction import Instruction
from backend.application.ai.tools.time import TimeTool
from config.config import Config
class State(MessagesState):
    summary: str = ""

logger = logging.getLogger('uvicorn.error')


class ResearchAgentUsecase:
    def __init__(self):
        self.llm = AI()
        self.instruction = Instruction()        
        self.sys_msg = (
            "Act as a helpful assistant that can summarize conversations and provide responses based on the conversation history."
            "You will be given a series of messages from a conversation, and your task is to generate a response based on the most recent messages."
            "If there are more than 10 messages in the conversation history, you should first summarize the conversation before generating a response."
            "Always provide concise and relevant responses based on the conversation history."
            "infromation you need to know about the project: " + self.instruction.read_skills("CAPABILITY.md") + "\n\n"
            "you are the assistant for a project called Brain, which is a knowledge graph management system."
            "Your role is to assist users in answering questions and interacting with the knowledge graph, providing information, answering questions, and helping with tasks related to the knowledge graph."
            "Always provide concise and relevant responses based on the conversation history."
            "you can't using tools. only provide information base on convertation history and your knowledge."
            
        )
        self.mongo_client = MongoDB.get_client()
        self.checkpointer = None  # Lazily initialized
        self.react_graph = self.__agent_logic()
        self.wa_integration_tool = WhatsAppIntegration()
        self.agent_usecase = ChatAgentUseCase()
        self.time_tool = TimeTool()
        self.config = Config()
    
    def __llm(self, max_retry=3, retry_count=0, set_default_on_error=None):
        try:
            if retry_count >= max_retry:
                raise Exception("Maximum retry attempts reached for LLM configuration. Please check your configurations and try again.")
            
            if not set_default_on_error:
                ai_config =  self.config.get_agent_model_config()
                agent_model_config = ai_config.research_model if ai_config.research_model.agent_provider != "default" else None
            else:
                agent_model_config = None
            return self.llm.LLM(reasoning=True, providers=agent_model_config)
        except Exception as e:
            logger.error(f"Error initializing LLM: {str(e)}. Retry count: {retry_count}")
            if "internal Server Error" in str(e).lower() and "status code: -1" in str(e).lower():
                return self.llm.LLM(reasoning=True, retry_count=retry_count + 1, set_default_on_error=True)
            raise e

    def __conversation(self, state: State):
        return {"messages": [self.__llm().invoke([self.sys_msg] + state["messages"])]}
    
    def should_continue(self, state: State, max_chat_to_summarize: int = 10) -> Literal["summarize_conversation", "__end__"]:
        
        """Return the next node to execute."""
        
        messages = state["messages"]
        
        # If there are more than six messages, then we summarize the conversation
        if len(messages) > max_chat_to_summarize:
            return "summarize_conversation"
        
        # Otherwise we can just end
        return END
    
    def summarize_conversation(self, state: State):
    
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

        # Add prompt to our history
        messages = state["messages"] + [HumanMessage(content=summary_message)]
        response = self.__llm().invoke(messages)
        
        # Delete all but the 2 most recent messages
        delete_messages = [RemoveMessage(id=m.id) for m in state["messages"][:-2]]
        return {"summary": response.content, "messages": delete_messages}
        

    def __agent_logic(self):
        builder = StateGraph(State)
        builder.add_node("conversation", self.__conversation)
        builder.add_node("summarize_conversation", self.summarize_conversation)

        builder.add_edge(START, "conversation")
        builder.add_conditional_edges("conversation", self.should_continue)
        builder.add_edge("summarize_conversation", END)
        
        # Lazy initialize checkpointer only when needed
        if self.checkpointer is None:
            self.checkpointer = MongoDBSaver(self.mongo_client, db_name="agent_memory_db", collection_name="agent_memory_collection")
        
        react_graph = builder.compile(checkpointer=self.checkpointer)
        return react_graph
    
    async def ResearchAgent(
            self, 
            messages: str,
            number: str,
            is_system=False,
            level: LevelSystem=LevelSystem.INFO, 
            group_id: str=None,
            reply_content: str=None,
            image: str=None, 
        ):
        send_to = number
        if group_id:
            send_to = group_id
        try:
            input_messages = []
            
            config = {"configurable": {"thread_id": group_id if group_id else number}}
            contact_info = self.wa_integration_tool.get_user_information(number)
            
            input_messages.append(AIMessage(content=f"Current user identifier: {number}, contact info: {contact_info.data.model_dump() if contact_info.data else 'N/A'} send at {self.time_tool.current_time()}"))            
            input_messages.append(HumanMessage(content=messages))
            result = await self.react_graph.ainvoke({
                "messages": input_messages,
                "sender": group_id if group_id else number,
            }, config=config)
            
            results_message = self.agent_usecase.extract_ai_content(result.get('messages', []))
            logger.info(f"Detail contact information for {number}: {contact_info}")
            self.wa_integration_tool.store_ai_message_to_inbox(results_message, number)
            self.wa_integration_tool.request_send_message(send_to, results_message, send_to)
        except Exception as e:
            logger.error(f"An error occurred while processing the research agent: {str(e)}")
            error_message = f"An error occurred: {str(e)}"
            self.wa_integration_tool.request_send_message(send_to, error_message, send_to)
            