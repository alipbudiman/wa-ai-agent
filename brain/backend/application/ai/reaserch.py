import logging
import traceback
import sys
import os

from langchain.messages import HumanMessage, RemoveMessage, AIMessage, SystemMessage
from langgraph.checkpoint.mongodb import MongoDBSaver
from langgraph.types import Send
from langgraph.graph import END, START, MessagesState, StateGraph
from typing import Any, List, Literal, Optional
from typing_extensions import TypedDict

from pydantic import AliasChoices, BaseModel, Field, model_validator

from backend.application.ai.ai import AI
from backend.infrastructure.database.database import MongoDB
from backend.application.ai.tools.models import LevelSystem
from backend.application.ai.tools.whatsapp_integration import WhatsAppIntegration
from backend.application.ai.agents import ChatAgentUseCase

from backend.application.ai.instruction import Instruction
from backend.application.ai.tools.time import TimeTool
from backend.application.ai.tools.websearch import WebSearchTool
from config.config import Config
from typing import Annotated
import operator


def _merge_string_list(left: Any, right: Any) -> list[str]:
    merged: list[str] = []

    for value in (left, right):
        if value is None:
            continue
        if isinstance(value, list):
            for item in value:
                if isinstance(item, str):
                    text = item.strip()
                else:
                    text = str(item).strip()
                if text:
                    merged.append(text)
            continue

        if isinstance(value, str):
            text = value.strip()
        else:
            text = str(value).strip()

        if text:
            merged.append(text)

    return merged


class State(MessagesState):
    summary: str = ""
    sender: str = ""
    topic: str
    question: str
    answer: Any
    context: Annotated[list[str], _merge_string_list]
    subjects: list[dict[str, str]]
    search: Annotated[list[str], _merge_string_list]
    sum_search_msg: str
    
class KnowlageFunction(BaseModel):
    name: str
    description: str

logger = logging.getLogger('uvicorn.error')


class QuerySearch(BaseModel):
    subjects: list[str]

class SummarizeSearch(BaseModel):
    sum_search: str
    
class OverallState(TypedDict):
    topic: str
    subjects: list[str]
    search: Annotated[list[str], _merge_string_list]
    sum_search_msg: str

class WebSearchState(TypedDict):
    subject: str | dict[str, str]

class WebSearchResult(BaseModel):
    context: str


class ResearchAgentUsecase:
    def __init__(self):
        self.llm = AI()
        self.instruction = Instruction()        
        self.checkpointer = MongoDB.get_checkpointer()
        self.react_graph = self.__agent_logic()
        self.wa_integration_tool = WhatsAppIntegration()
        self.agent_usecase = ChatAgentUseCase()
        self.time_tool = TimeTool()
        self.config = Config()
        self.web_search_tool = WebSearchTool()
        self._process_diagram_mermaid = ""
        self.access_knowledge = [
            KnowlageFunction(name=self.web_search_tool.websearch_tool.__name__, description=self.web_search_tool.websearch_tool.__doc__).model_dump(),
            KnowlageFunction(name=self.web_search_tool.search_wikipedia.__name__, description=self.web_search_tool.search_wikipedia.__doc__).model_dump(),
        ]
        self.sys_msg = (
            "Im Reasearch Agent, your name is MR.Senku, you are a helpful assistant for researching and answering questions,"
            "you will receive a question and you need to research on the question and answer it."
            "Add info:\n"
            f"Your Access to Knowledge:\n{self.access_knowledge}\n"
            f"Your Workflow:\n{self._process_diagram_mermaid}\n"
        )
        self._reasoning = True
    

    def __llm(self, max_retry=3, retry_count=0, set_default_on_error=None, structured_output: Optional[Any] = None):
        try:
            if retry_count >= max_retry:
                raise Exception("Maximum retry attempts reached for LLM configuration. Please check your configurations and try again.")
            
            if not set_default_on_error:
                ai_config =  self.config.get_agent_model_config()
                agent_model_config = ai_config.research_model if ai_config.research_model.agent_provider != "default" else None
            else:
                agent_model_config = None
            llm = self.llm.LLM(reasoning=self._reasoning, providers=agent_model_config)
            if structured_output is not None:
                return llm.with_structured_output(structured_output)
            return llm
        except Exception as e:
            logger.error(f"Error initializing LLM: {str(e)}. Retry count: {retry_count}")
            if "internal Server Error" in str(e).lower() and "status code: -1" in str(e).lower():
                return self.llm.LLM(reasoning=self._reasoning, retry_count=retry_count + 1, set_default_on_error=True)
            raise e

    @staticmethod
    def __content_to_text(content: Any) -> str:
        if isinstance(content, str):
            return content.strip()
        if isinstance(content, AIMessage):
            return ResearchAgentUsecase.__content_to_text(content.content)
        if hasattr(content, "content"):
            return ResearchAgentUsecase.__content_to_text(getattr(content, "content"))
        if isinstance(content, list):
            text_parts: list[str] = []
            for part in content:
                if isinstance(part, str):
                    text_parts.append(part)
                    continue
                if isinstance(part, dict):
                    text = part.get("text")
                    if isinstance(text, str) and text.strip():
                        text_parts.append(text.strip())
            return "\n".join(text_parts).strip()
        if content is None:
            return ""
        return str(content).strip()

    def __extract_latest_human_message(self, state: State) -> str:
        for message in reversed(state.get("messages", [])):
            if not isinstance(message, HumanMessage):
                continue
            content = self.__content_to_text(getattr(message, "content", ""))
            if content:
                return content
        return ""

    def __generate_query_search_prompt(self, state: State) -> dict[str, list[dict[str, str]]]:
        topic = (state.get("topic") or state.get("question") or self.__extract_latest_human_message(state)).strip()
        if not topic:
            return {"subjects": []}

        prompt = (
            "Generate 3-5 concise web search query strings for the user topic below.\n"
            "Return a JSON object with a single key 'subjects' whose value is a list of search query strings.\n"
            "Example: {\"subjects\": [\"query one\", \"query two\", \"query three\"]}\n"
            f"User topic: {topic}"
        )
        try:
            response = self.__llm(structured_output=QuerySearch).invoke(prompt)
            logger.info(f"Generated query search response: {response}")
        except Exception as err:
            logger.warning("Failed to parse structured query-search output: %s", err)
            return {"subjects": [{"query": topic, "topic": "general"}]}

        normalized_subjects: list[dict[str, str]] = []
        for subject in response.subjects:
            # subjects is list[str] — convert each string to the expected dict format
            if isinstance(subject, str):
                query = subject.strip()
                topic_label = "general"
            elif isinstance(subject, dict):
                query = str(subject.get("query", subject.get("subject", ""))).strip()
                topic_label = str(subject.get("topic", "general")).strip() or "general"
            else:
                continue
            if not query:
                continue
            normalized_subjects.append({"query": query, "topic": topic_label})

        if not normalized_subjects:
            normalized_subjects.append({"query": topic, "topic": "general"})

        return {"subjects": normalized_subjects}
    
    def __continue_to_web_search(self, state: State):
        subjects = state.get("subjects", [])
        if not subjects:
            return "summarize_search_results"

        sends: list[Send] = []
        for subject in subjects:
            if isinstance(subject, str):
                payload = {"query": subject, "topic": "general"}
            elif isinstance(subject, dict):
                payload = {
                    "query": str(subject.get("query", "")).strip(),
                    "topic": str(subject.get("topic", "general")).strip() or "general",
                }
            else:
                continue

            if not payload.get("query"):
                continue
            payload["sender"] = state.get("sender", "")
            sends.append(Send("web_search", payload))

        if not sends:
            return "summarize_search_results"
        return sends

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

    def __search_web(self, state: State):
        
        # Retrieve docs from web search.
        # When invoked via Send(), the payload keys (query, topic, sender) are
        # placed at the TOP LEVEL of state — not nested under a "subject" key.
        query = str(state.get("query", "")).strip()
        topic = str(state.get("topic", "general")).strip() or "general"
        sender = str(state.get("sender", "")).strip()

        # Fallback: if not invoked via Send(), try the legacy "subject" dict
        if not query:
            subject = state.get("subject", {})
            if not subject:
                subjects = state.get("subjects", [])
                if subjects:
                    subject = subjects[0]

            if isinstance(subject, str):
                query = subject.strip()
                topic = "general"
            elif isinstance(subject, dict):
                query = str(subject.get("query", "")).strip()
                topic = str(subject.get("topic", "general")).strip() or "general"
                sender = sender or str(subject.get("sender", "")).strip()

        # Last resort: pull from question/topic fields or message history
        if not query:
            query = (
                state.get("question")
                or self.__extract_latest_human_message(state)
                or ""
            ).strip()

        if not query:
            return {"search": []}

        if not sender:
            sender = state.get("sender", "")

        logger.info(f"Web search — query: '{query}', topic: '{topic}', sender: '{sender}'")
        results = self.web_search_tool.websearch(
            sender=sender,
            query=query,
            topic=topic,
        )

        return {"search": [results]}

    def __collect_search_results(self, state: State) -> dict:
        """Barrier/fan-in node: waits for all parallel web_search nodes to complete.
        
        LangGraph will only proceed past this node once all Send("web_search", ...)
        instances have finished and their 'search' results are fully accumulated
        into state via operator.add. This node itself is a no-op pass-through.
        """
        logger.info(f"All web searches collected. Total results: {len(state.get('search', []))}")
        return {}

    def __summarize_search_results(self, state: State):
        """ Summarize search results into a concise summary that can be used in the conversation flow. """

        search_items = [item for item in state.get("search", []) if isinstance(item, str) and item.strip()]
        if not search_items:
            return {"sum_search_msg": ""}

        search_results = "\n\n".join(search_items)
        prompt = (
            "Create a DETAILED research summary in structured format based on the following web search data.\n"
            "Use the mandatory format below:\n"
            "1) Executive Summary (4-6 sentences).\n"
            "2) Key Facts per Topic (minimum 8 points) including numbers, dates/times, and context.\n"
            "3) Cross-source comparison if data differs between sources.\n"
            "4) Trend analysis and implications (2-4 points).\n"
            "5) Actionable final conclusions.\n"
            "Rules:\n"
            "- Do not be too brief.\n"
            "- Do not omit numbers, value ranges, percentages, volumes, or available timestamps.\n"
            "- Include sources/URLs when available.\n"
            "- If conflicting data exists, explain possible causes.\n\n"
            f"Web search results data:\n{search_results}"
        )
        response = self.__llm().invoke(
            [
                SystemMessage(content="You are a senior research analyst. Responses must be comprehensive, detailed, and data-driven."),
                HumanMessage(content=prompt),
            ]
        )
        summary_text = self.__content_to_text(response)
        if not summary_text:
            summary_text = search_results[:1000]
        return {"sum_search_msg": summary_text}
    
    def search_wikipedia(self, state: State):
        """ search wikipedia and return formatted results. for search on wikipedia, we can directly return the search result without sending progress notification to user, because wikipedia search is usually fast enough and we can include the search result in the conversation flow seamlessly without breaking the user experience. """
        # Search
        question = (state.get("question") or self.__extract_latest_human_message(state)).strip()
        if not question:
            return {"context": []}
        search_docs = self.web_search_tool.search_wikipedia(question=question)
        
        logger.info(f"Wikipedia search results for question '{question}'")

        return {"context": [search_docs]}
    
    def generate_answer(self, state: State):
        
        """ Node to answer a question """

        # Get state
        question = (state.get("question") or self.__extract_latest_human_message(state)).strip()
        search_summary = state.get("sum_search_msg", "").strip()
        raw_search_results = [item for item in state.get("search", []) if isinstance(item, str) and item.strip()]
        search_context = search_summary or "\n\n".join(raw_search_results)
        wiki_context_items = [item for item in state.get("context", []) if isinstance(item, str) and item.strip()]
        wiki_context = "\n\n".join(wiki_context_items)

        # Template
        answer_template = (
            "Answer the following question accurately, in detail, and based on data.\n"
            "Question: {question}\n\n"
            "Web Search Context:\n{web_context}\n\n"
            "Wikipedia Context:\n{wiki_context}\n"
        )
        answer_instructions = answer_template.format(
            question=question or "Topic not available",
            web_context=search_context or "No relevant web search results available.",
            wiki_context=wiki_context or "No relevant Wikipedia data available."
        )
        
        # Answer
        answer = self.__llm().invoke(
            [
                SystemMessage(content="You are MR.Senku, a helpful research assistant."),
                HumanMessage(content=answer_instructions),
            ]
        )

        if not isinstance(answer, AIMessage):
            answer = AIMessage(content=self.__content_to_text(answer))
        
        # Append it to state
        return {"answer": answer, "messages": [answer]}
    
    def __save_process_diagram(self, react_graph: Any) -> None:
        try:
            self._process_diagram_mermaid = react_graph.get_graph().draw_mermaid()
            logger.info(
                "Research agent process diagram mermaid cached (%d chars)",
                len(self._process_diagram_mermaid),
            )
        except Exception as err:
            logger.warning("Failed to render research agent process diagram mermaid: %s", err)
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
        builder.add_node("generate_query_search_prompt", self.__generate_query_search_prompt)
        builder.add_node("web_search", self.__search_web)
        builder.add_node("collect_search_results", self.__collect_search_results)  # barrier
        builder.add_node("summarize_search_results", self.__summarize_search_results)
        builder.add_node("search_wikipedia", self.search_wikipedia)
        builder.add_node("generate_answer", self.generate_answer)

        builder.add_edge(START, "generate_query_search_prompt")
        builder.add_edge(START, "search_wikipedia")
        builder.add_conditional_edges(
            "generate_query_search_prompt",
            self.__continue_to_web_search,
            ["web_search", "collect_search_results"],  # fallback to barrier if no subjects
        )
        # All parallel web_search nodes fan-in to the barrier first
        builder.add_edge("web_search", "collect_search_results")
        # Barrier fan-out: wait for all searches + wikipedia, then summarize
        builder.add_edge("collect_search_results", "summarize_search_results")
        builder.add_edge(["summarize_search_results", "search_wikipedia"], "generate_answer")
        builder.add_edge("generate_answer", END)
        # Lazy initialize checkpointer only when needed
       
        react_graph = builder.compile(checkpointer=self.checkpointer)
        self.__save_process_diagram(react_graph)
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
            
            input_messages.append(HumanMessage(content=f"Current user identifier: {number}, contact info: {contact_info.data.model_dump() if contact_info.data else 'N/A'} send at {self.time_tool.current_time()}. Message: {messages}"))
            result = await self.react_graph.ainvoke({
                "messages": input_messages,
                "sender": group_id if group_id else number,
                "question": messages,
                "topic": messages,
            }, config=config)
            
            results_message = self.agent_usecase.extract_ai_content(result.get('messages', []))
            if not results_message:
                results_message = self.__content_to_text(result.get("answer", ""))
            if not results_message:
                results_message = "I could not generate a valid answer from the research process."
            logger.info(f"Detail contact information for {number}: {contact_info}")
            self.wa_integration_tool.store_ai_message_to_inbox(results_message, number)
            self.wa_integration_tool.request_send_message(send_to, results_message, send_to)
        except Exception as e:
            logger.error(f"An error occurred while processing the research agent: {str(e)}")
            error_message = f"An error occurred: {str(e)}"
            traceback.format_exc()
            traceback.print_exc()
            self.wa_integration_tool.request_send_message(send_to, error_message, send_to)
            
