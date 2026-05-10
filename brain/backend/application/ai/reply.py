import logging
from typing import Any

from langchain.messages import HumanMessage, RemoveMessage, SystemMessage
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
from backend.application.ingestion.loader import DocumentLoader
from backend.application.ai.models import REPLY_AGENT

class State(MessagesState):
    summary: str = ""
    rag_context: str = ""
    rag_query: str = ""

logger = logging.getLogger('uvicorn.error')


class ReplyAgentUsecase:
    RAG_TOP_K = 5
    RAG_SCORE_THRESHOLD = 0.6

    def __init__(self):
        self.llm = AI()
        self.instruction = Instruction()        
        self.checkpointer = MongoDB.get_checkpointer()
        self.wa_integration_tool = WhatsAppIntegration()
        self.agent_usecase = ChatAgentUseCase()
        self.time_tool = TimeTool()
        self.config = Config()
        self._process_diagram_mermaid = ""
        self._reasoning = True

        self.doc_loader = DocumentLoader(collection_name=REPLY_AGENT)

        self.sys_msg = (
            "You are a reply agent named Sofia, a helpful assistant. "
            "If document context is provided, use it to answer accurately. "
            "If not relevant or empty, answer from general knowledge. "
            "Always be concise and clear."
        )
        self.react_graph = self.__agent_logic()

    
    def __llm(self, max_retry=3, retry_count=0, set_default_on_error=None):
        try:
            if retry_count >= max_retry:
                raise Exception("Maximum retry attempts reached for LLM configuration. Please check your configurations and try again.")
            
            if not set_default_on_error:
                ai_config =  self.config.get_agent_model_config()
                agent_model_config = ai_config.reply_model if ai_config.reply_model.agent_provider != "default" else None
            else:
                agent_model_config = None
            return self.llm.LLM(reasoning=self._reasoning, providers=agent_model_config)
        except Exception as e:
            logger.error(f"Error initializing LLM: {str(e)}. Retry count: {retry_count}")
            if "internal Server Error" in str(e).lower() and "status code: -1" in str(e).lower():
                return self.llm.LLM(reasoning=self._reasoning, retry_count=retry_count + 1, set_default_on_error=True)
            raise e
    
    def __retrieve(self, state: State):
        """
        Retrieve relevant documents from the vector DB based on the latest user message.
        This node ONLY reads from the 'documents' collection.
        Conversation messages are NEVER written to the vector DB.
        """
        try:
            query = str(state.get("rag_query") or "").strip()
            if not query:
                last_msg = state["messages"][-1]
                query = (
                    last_msg.content
                    if isinstance(last_msg.content, str)
                    else str(last_msg.content)
                )

            docs_with_scores = self.doc_loader.similarity_search_with_relevance_scores(
                query=query,
                k=self.RAG_TOP_K,
                score_threshold=self.RAG_SCORE_THRESHOLD,
            )

            if docs_with_scores:
                context_parts = [
                    (
                        f"[source: {doc.metadata.get('source', '?')} | "
                        f"chunk: {doc.metadata.get('chunk_index', doc.metadata.get('chunk_id', '?'))} | "
                        f"score: {score:.3f}]\n{doc.page_content}"
                    )
                    for doc, score in docs_with_scores
                ]
                rag_context = "\n\n".join(context_parts)
                logger.info(f"RAG: {len(docs_with_scores)} relevant documents found")
            else:
                rag_context = ""
                logger.info("RAG: no relevant documents passed the threshold, fallback to general knowledge")

            return {"rag_context": rag_context}

        except Exception as e:
            logger.error(f"RAG retrieve error: {e}")
            return {"rag_context": ""}
    
    

    def __conversation(self, state: State):
        """
        Generate a response based on:
        - sys_msg (Sofia's base instruction)
        - summary (previous conversation summary, if available)
        - rag_context (relevant documents from vector DB, if available)
        - messages (chat history from the checkpointer)
        
        No writes to vector DB happen in this node.
        """
        rag_context = state.get("rag_context", "")
        summary     = state.get("summary", "")

        context_block = (
            rag_context
            if rag_context
            else "No relevant uploaded document context was found."
        )

        summary_block = summary if summary else "No prior summary is available."

        system_content = (
            "ROLE:\n"
            "You are Sofia, a helpful and reliable reply assistant.\n\n"
            "PRIMARY INSTRUCTIONS:\n"
            "1. Prioritize factual accuracy and clarity.\n"
            "2. If uploaded document context is relevant, prioritize it over general knowledge.\n"
            "3. If uploaded document context is missing or irrelevant, use general knowledge.\n"
            "4. Keep answers concise, direct, and easy to understand.\n"
            "5. Do not mention hidden reasoning steps. Provide only the final answer.\n\n"
            "CONVERSATION SUMMARY:\n"
            f"{summary_block}\n\n"
            "DOCUMENT CONTEXT:\n"
            f"{context_block}"
        )

        messages = [SystemMessage(content=system_content)] + state["messages"]
        response = self.__llm().invoke(messages)
        return {"messages": [response]}
    
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
        

    def __save_process_diagram(self, react_graph: Any) -> None:
        try:
            self._process_diagram_mermaid = react_graph.get_graph().draw_mermaid()
            logger.info(
                "Reply agent process diagram mermaid cached (%d chars)",
                len(self._process_diagram_mermaid),
            )
        except Exception as err:
            logger.warning("Failed to render reply agent process diagram mermaid: %s", err)
            self._process_diagram_mermaid = ""

    def get_process_diagram_mermaid(self) -> str:
        diagram = (self._process_diagram_mermaid or "").strip()
        if diagram:
            return diagram
        # Fallback: regenerate from graph
        try:
            fallback = self.react_graph.get_graph().draw_mermaid().strip()
        except Exception:
            fallback = ""
        self._process_diagram_mermaid = fallback
        return fallback

    def __agent_logic(self):
        builder = StateGraph(State)
        builder.add_node("retrieve", self.__retrieve)
        builder.add_node("conversation", self.__conversation)
        builder.add_node("summarize_conversation", self.summarize_conversation)

        builder.add_edge(START, "retrieve")
        builder.add_edge("retrieve", "conversation")
        builder.add_conditional_edges("conversation", self.should_continue)
        builder.add_edge("summarize_conversation", END)
        
        react_graph = builder.compile(checkpointer=self.checkpointer)
        self.__save_process_diagram(react_graph)
        return react_graph
    
    async def ReplyAgent(
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
            
            input_messages.append(HumanMessage(content=f"Current user identifier: {number}, contact info: {contact_info.data.model_dump() if contact_info.data else 'N/A'} send at {self.time_tool.current_time()}. Message: {messages}"))
            result = await self.react_graph.ainvoke({
                "messages": input_messages,
                "sender": group_id if group_id else number,
                "rag_query": messages,
                "rag_context": "",
                # "summary": "",
            }, config=config)
            
            results_message = self.agent_usecase.extract_ai_content(result.get('messages', []))
            logger.info(f"Detail contact information for {number}: {contact_info}")
            self.wa_integration_tool.store_ai_message_to_inbox(results_message, number)
            self.wa_integration_tool.request_send_message(send_to, results_message, send_to)
        except Exception as e:
            logger.error(f"An error occurred while processing the research agent: {str(e)}")
            error_message = f"An error occurred: {str(e)}"
            self.wa_integration_tool.request_send_message(send_to, error_message, send_to)
            
