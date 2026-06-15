import logging
import traceback
import sys
import os
import uuid

from langchain.messages import HumanMessage, RemoveMessage, AIMessage, SystemMessage, ToolMessage
from langchain_community.tools import StructuredTool
from langgraph.checkpoint.mongodb import MongoDBSaver
from langgraph.types import Send
from langgraph.graph import END, START, MessagesState, StateGraph
from langgraph.prebuilt import ToolNode
from typing import Any, List, Literal, Optional
from typing_extensions import TypedDict

from pydantic import AliasChoices, BaseModel, Field, model_validator

from backend.application.ai.ai import AI
from backend.infrastructure.database.database import MongoDB
from backend.application.ai.tools.models import LevelSystem
from backend.application.ai.tools.whatsapp_integration import WhatsAppIntegration

from backend.application.ai.tools.time import TimeTool
from backend.application.ai.tools.tools_builder import ToolsBuilder
from backend.application.ai.tools.websearch import WebSearchTool
from backend.application.ingestion.loader import DocumentLoader
from backend.application.ai.models import REASEARCH_AGENT
from backend.application.ai.helper import minimax_helper
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
    rag_context: str = ""
    rag_query: str = ""
    custom_tool_context: str = ""
    custom_tool_query: str = ""
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

class AgentResearchInput(BaseModel):
    messages: str = Field(
        description=(
            "The input messages that provide context for the research. "
            "This can include the user's question, any relevant conversation history, and additional instructions or information to guide the research process."
        )
    )
    question: str = Field(
        description=(
            "The specific question or topic that the research agent should focus on. "
            "This is the main query that the agent will use to perform web searches and gather information to generate an answer."
        )
    )
    topic: str = Field(
        description=(
            "The broader topic or category related to the question. "
            "This can help the research agent contextualize the question and perform more targeted web searches."
        )
    )


class ResearchAgentUsecase:
    RAG_TOP_K = 5
    RAG_SCORE_THRESHOLD = 0.6
    CUSTOM_TOOL_TOP_K = 5
    MAX_CUSTOM_TOOL_INFORMATION_CHARS = 4000

    def __init__(self):
        self.llm = AI()
        self.checkpointer = MongoDB.get_checkpointer()
        self._process_diagram_mermaid = ""
        self.wa_integration_tool = WhatsAppIntegration()
        self.time_tool = TimeTool()
        self.config = Config()
        self.web_search_tool = WebSearchTool()
        self.tools_builder = ToolsBuilder()
        self.doc_loader = DocumentLoader(collection_name=REASEARCH_AGENT)
        self.react_graph = self.__agent_logic()
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

    def __active_research_provider(self) -> str:
        return minimax_helper.selected_provider(self.config, "research_model")

    def __uses_textual_tool_calls(self) -> bool:
        return self.__active_research_provider() == "minimax"

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

    @staticmethod
    def __compact_text(text: str | None, max_chars: int) -> str:
        compact = str(text or "").strip()
        if len(compact) <= max_chars:
            return compact
        return compact[: max_chars - 3].rstrip() + "..."

    def __extract_ai_content(self, messages: list) -> str:
        """
        Extract the latest non-empty assistant text from the research graph.

        The research graph can append intermediate messages before the final
        answer. Only messages after the latest human input are considered so a
        previous checkpoint cannot leak into the current tool result.
        """
        if not messages:
            return ""

        latest_human_index = -1
        for index in range(len(messages) - 1, -1, -1):
            if isinstance(messages[index], HumanMessage):
                latest_human_index = index
                break

        current_turn_messages = messages[latest_human_index + 1:] if latest_human_index >= 0 else messages
        tool_fallback = ""
        for message in reversed(current_turn_messages):
            if not isinstance(message, ToolMessage):
                continue
            tool_text = self.__content_to_text(getattr(message, "content", ""))
            if tool_text:
                tool_fallback = tool_text
                break

        for message in reversed(current_turn_messages):
            if not isinstance(message, AIMessage):
                continue

            text = self.__content_to_text(message)
            if self.__looks_like_textual_tool_call(text):
                continue
            if text:
                return text

        return tool_fallback

    def __extract_latest_human_message(self, state: State) -> str:
        for message in reversed(state.get("messages", [])):
            if not isinstance(message, HumanMessage):
                continue
            content = self.__content_to_text(getattr(message, "content", ""))
            if content:
                return content
        return ""

    def __get_tools(self, state: State) -> list:
        if not self.tools_builder.get_tools_builder_list(agent_type=REASEARCH_AGENT):
            return []

        sender = str(state.get("sender", "") or "").strip()
        return [
            self.tools_builder.find_relevant_tools_builder(agent_type=REASEARCH_AGENT),
            self.tools_builder.execute_tools_builder(agent_type=REASEARCH_AGENT, sender=sender),
        ]

    @staticmethod
    def __tool_object_name(tool: Any) -> str:
        return minimax_helper.tool_object_name(tool)

    @staticmethod
    def __looks_like_textual_tool_call(text: str) -> bool:
        return minimax_helper.looks_like_textual_tool_call(text)

    def __extract_textual_tool_calls(self, text: str, tools: list) -> list[dict]:
        return minimax_helper.extract_textual_tool_calls(text, tools, id_prefix="research-minimax")

    def __coerce_textual_tool_response(self, response: Any, tools: list) -> Any:
        return minimax_helper.coerce_textual_tool_response(response, tools, logger=logger, id_prefix="research-minimax")

    def __tool_args_schema_text(self, tool: Any) -> str:
        return minimax_helper.tool_args_schema_text(tool)

    def __textual_tool_instruction(self, tools: list) -> str:
        return minimax_helper.textual_tool_instruction(
            tools,
            prefer_find_first=True,
            result_rule=(
                "- If current custom tool context already answers the research question, answer from it. "
                "If a code-backed custom tool is needed for mutable/current custom data, call execute_custom_tool. "
                "If no current source supports an internal or mutable claim, say the current source data is unavailable."
            ),
        )

    @staticmethod
    def __has_pending_tool_calls(state: State) -> bool:
        for message in reversed(state.get("messages", [])):
            if not isinstance(message, AIMessage):
                continue

            if getattr(message, "tool_calls", None):
                return True

            additional_kwargs = getattr(message, "additional_kwargs", {}) or {}
            if additional_kwargs.get("tool_calls"):
                return True

            return False

        return False

    async def __tool_node(self, state: State):
        tools = self.__get_tools(state)
        if not tools:
            return {
                "messages": [
                    AIMessage(content="No custom database tools are configured for the research agent.")
                ]
            }

        return await ToolNode(tools).ainvoke(state)

    def __route_after_generate_answer(self, state: State) -> Literal["tools", "end"]:
        if self.__has_pending_tool_calls(state):
            return "tools"
        return "end"

    def __custom_tool_context(self, state: State) -> str:
        latest_human_index = -1
        messages = state.get("messages", [])
        for index in range(len(messages) - 1, -1, -1):
            if isinstance(messages[index], HumanMessage):
                latest_human_index = index
                break

        current_turn_messages = messages[latest_human_index + 1:] if latest_human_index >= 0 else messages
        tool_context_parts: list[str] = []
        for message in current_turn_messages:
            if not isinstance(message, ToolMessage):
                continue

            tool_name = str(getattr(message, "name", "") or "custom_tool").strip()
            tool_text = self.__content_to_text(getattr(message, "content", ""))
            if tool_text:
                tool_context_parts.append(f"[tool: {tool_name}]\n{tool_text}")

        return "\n\n".join(tool_context_parts).strip()

    def __load_custom_tool_context(self, state: State):
        """
        Load current-turn Tools Builder context before answer generation.

        This preloads information-only tool data as research source context and
        flags code-backed tools so the LLM can execute them when mutable custom
        data is required.
        """
        try:
            query = str(
                state.get("custom_tool_query")
                or state.get("rag_query")
                or state.get("question")
                or state.get("topic")
                or ""
            ).strip()
            if not query:
                query = self.__extract_latest_human_message(state)

            if not query:
                return {"custom_tool_context": ""}

            relevant_tools = self.tools_builder.find_relevant_tools(
                agent_type=REASEARCH_AGENT,
                query=query,
                limit=self.CUSTOM_TOOL_TOP_K,
            )
            if not relevant_tools:
                logger.info("Research tools: no relevant custom tools found for current turn")
                return {"custom_tool_context": ""}

            context_parts: list[str] = []
            for tool in relevant_tools:
                safe_name = self.tools_builder.safe_tool_name(tool.name)
                has_code = bool(tool.code and tool.code.code and tool.code.code.strip())
                information = self.__compact_text(
                    tool.information,
                    self.MAX_CUSTOM_TOOL_INFORMATION_CHARS,
                )
                tool_parts = [
                    f"[custom_tool: {tool.name} | callable: {safe_name} | code: {'available' if has_code else 'not configured'}]",
                    f"Description: {tool.description}",
                ]
                if information:
                    tool_parts.append(f"Information:\n{information}")
                elif has_code:
                    tool_parts.append(
                        "Information: no static information is configured; execute this tool when current output is needed."
                    )
                else:
                    tool_parts.append("Information: no source data is configured for this tool.")

                context_parts.append("\n".join(tool_parts))

            logger.info("Research tools: %d relevant custom tools found", len(relevant_tools))
            return {"custom_tool_context": "\n\n".join(context_parts)}

        except Exception as err:
            logger.error("Research custom tool context error: %s", err)
            return {"custom_tool_context": ""}

    def __retrieve(self, state: State):
        """
        Retrieve uploaded document context for the research agent.

        This augments web/Wikipedia research with the research-agent vector
        collection while preserving the existing web-first workflow when no
        relevant documents are available.
        """
        try:
            query = str(
                state.get("rag_query")
                or state.get("question")
                or state.get("topic")
                or ""
            ).strip()
            if not query:
                query = self.__extract_latest_human_message(state)

            if not query:
                return {"rag_context": ""}

            docs_with_scores = self.doc_loader.similarity_search_with_relevance_scores(
                query=query,
                k=self.RAG_TOP_K,
                score_threshold=self.RAG_SCORE_THRESHOLD,
            )

            if not docs_with_scores:
                logger.info("Research agent RAG: no relevant documents passed the threshold")
                return {"rag_context": ""}

            context_parts = [
                (
                    f"[source: {doc.metadata.get('source', '?')} | "
                    f"chunk: {doc.metadata.get('chunk_index', doc.metadata.get('chunk_id', '?'))} | "
                    f"score: {score:.3f}]\n{doc.page_content}"
                )
                for doc, score in docs_with_scores
            ]
            logger.info("Research agent RAG: %d relevant documents found", len(docs_with_scores))
            return {"rag_context": "\n\n".join(context_parts)}

        except Exception as err:
            logger.error("Research agent RAG retrieve error: %s", err)
            return {"rag_context": ""}

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
            return "collect_search_results"

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
            return "collect_search_results"
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
            summary_message += (
                "\n\nImportant: conversation history is not a source of truth for mutable facts. "
                "Summarize it only for continuity, mark facts from previous turns as historical unless they were verified by current tool/RAG/web context, "
                "and do not copy raw custom tool outputs into the summary."
            )

        else:
            summary_message = (
                "Create a summary of the conversation above.\n\n"
                "Important: conversation history is not a source of truth for mutable facts. "
                "Summarize it only for continuity, mark facts from previous turns as historical unless they were verified by current tool/RAG/web context, "
                "and do not copy raw custom tool outputs into the summary."
            )

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
        rag_context = str(state.get("rag_context") or "").strip()
        executed_custom_tool_context = self.__custom_tool_context(state)
        preload_custom_tool_context = str(state.get("custom_tool_context") or "").strip()
        custom_tool_context = "\n\n".join(
            part for part in [executed_custom_tool_context, preload_custom_tool_context] if part
        )

        # Template
        answer_template = (
            "Answer the following question accurately, in detail, and based on data.\n"
            "SOURCE-OF-TRUTH ORDER FOR THIS TURN:\n"
            "1. Current-turn custom database tool output and Tools Builder context.\n"
            "2. Current-turn uploaded document context from RAG.\n"
            "3. Current-turn web search and Wikipedia context.\n"
            "4. The latest user question/topic.\n"
            "5. Conversation history only as continuity context; do not use it as authority for mutable facts.\n\n"
            "Rules:\n"
            "- If current sources conflict with older conversation history, current tool/RAG/web context wins.\n"
            "- If a mutable internal/business claim is unsupported by current sources, say the current source data is unavailable.\n"
            "- If the request is general research and internal sources are unrelated, use web/Wikipedia and general knowledge with clear caveats.\n\n"
            "Question: {question}\n\n"
            "Document Context:\n{document_context}\n\n"
            "Web Search Context:\n{web_context}\n\n"
            "Wikipedia Context:\n{wiki_context}\n\n"
            "Custom Tool Context:\n{custom_tool_context}\n"
        )
        answer_instructions = answer_template.format(
            question=question or "Topic not available",
            document_context=rag_context or "No relevant uploaded document context available.",
            web_context=search_context or "No relevant web search results available.",
            wiki_context=wiki_context or "No relevant Wikipedia data available.",
            custom_tool_context=custom_tool_context or "No custom tool output available."
        )

        system_content = (
            "You are MR.Senku, a helpful research assistant. "
            "Use current-turn source context before conversation history."
        )
        custom_tools_catalog = self.tools_builder.custom_tools_catalog(agent_type=REASEARCH_AGENT)
        if custom_tools_catalog and not executed_custom_tool_context:
            system_content += (
                "\n\nCUSTOM DATABASE TOOL ACCESS:\n"
                f"{custom_tools_catalog}\n\n"
                "CUSTOM TOOL USAGE:\n"
                "- You have exactly two custom-tool gateway functions: "
                "`find_relevant_custom_tools` and `execute_custom_tool`.\n"
                "- The current-turn custom tool context is the first relevance scan.\n"
                "- If that context lists a matching code-backed tool, call `execute_custom_tool` with the matching original_name or callable_tool_name when mutable/current custom data is required.\n"
                "- If no matching context is listed but the question may still match a custom tool, call `find_relevant_custom_tools` first.\n"
                "- Treat callable_tool_name values as identifiers for `execute_custom_tool`, not as directly callable functions.\n"
                "- Do not call `access_tools_builder`; it is not available in this agent.\n"
                "- Use custom tool output as authoritative research context in the final answer."
            )
        
        # Answer
        llm = self.__llm()
        tools = self.__get_tools(state)
        if tools and not executed_custom_tool_context:
            if self.__uses_textual_tool_calls():
                system_content += "\n\n" + self.__textual_tool_instruction(tools)
            else:
                llm = llm.bind_tools(tools)

        answer = llm.invoke(
            [
                SystemMessage(content=system_content),
                HumanMessage(content=answer_instructions),
            ]
        )

        if not isinstance(answer, AIMessage):
            answer = AIMessage(content=self.__content_to_text(answer))
        if tools and not executed_custom_tool_context and self.__uses_textual_tool_calls():
            answer = self.__coerce_textual_tool_response(answer, tools)
        
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
        builder.add_node("retrieve", self.__retrieve)
        builder.add_node("load_custom_tool_context", self.__load_custom_tool_context)
        builder.add_node("generate_query_search_prompt", self.__generate_query_search_prompt)
        builder.add_node("web_search", self.__search_web)
        builder.add_node("collect_search_results", self.__collect_search_results)  # barrier
        builder.add_node("summarize_search_results", self.__summarize_search_results)
        builder.add_node("search_wikipedia", self.search_wikipedia)
        builder.add_node("generate_answer", self.generate_answer)
        builder.add_node("tools", self.__tool_node)

        builder.add_edge(START, "retrieve")
        builder.add_edge(START, "load_custom_tool_context")
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
        builder.add_edge(["summarize_search_results", "search_wikipedia", "retrieve", "load_custom_tool_context"], "generate_answer")
        builder.add_conditional_edges(
            "generate_answer",
            self.__route_after_generate_answer,
            {
                "tools": "tools",
                "end": END,
            },
        )
        builder.add_edge("tools", "generate_answer")
        # Lazy initialize checkpointer only when needed
       
        react_graph = builder.compile(checkpointer=self.checkpointer)
        self.__save_process_diagram(react_graph)
        return react_graph
    
    def make_agent_reaserch_tool(self, sender: str):
        """ Factory - creates a callable that can be used to invoke the research agent logic. """

        async def reaserch(messages: str, question: str, topic: str):
            config = {"configurable": {"thread_id": f"research-tool:{sender}:{uuid.uuid4()}"}}
            result = await self.react_graph.ainvoke({
                "messages": [HumanMessage(content=f"Current Time: {self.time_tool.current_time()}\nMessage:\n{messages}")],
                "sender": sender,
                "question": question,
                "topic": topic,
                "rag_query": question or topic or messages,
                "rag_context": "",
                "custom_tool_query": question or topic or messages,
                "custom_tool_context": "",
            }, config=config)
            results_message = self.__extract_ai_content(result.get('messages', []))
            if not results_message:
                results_message = self.__content_to_text(result.get("answer", ""))
            if not results_message:
                results_message = "I could not generate a valid answer from the research process."
            return results_message

        def reaserch_sync(*args, **kwargs) -> str:
            return "[ERROR] Research agent tool must be called asynchronously."

        return StructuredTool.from_function(
            func=reaserch_sync,
            coroutine=reaserch,
            name="research_agent_tool",
            description=(
                "Primary tool for explicit research/riset requests. Use this when the user asks to research, "
                "riset, investigate, compare sources/options, analyze trends, market/news/background context, "
                "or produce a detailed sourced synthesis. "
                "A tool to research and answer questions based on a given message, question, and topic. "
                "The input is a string message, a string question, and a string topic. "
                "The output is a string answer generated by the research agent after performing reaserch, return the complete results. DONT SUMMARIZE OR SIMPLIFY THE ANSWER, JUST RETURN THE COMPLETE ANSWER. "
            ),
            args_schema=AgentResearchInput,  # No structured args schema since we're using a custom input format
        )

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
                "rag_query": messages,
                "rag_context": "",
                "custom_tool_query": messages,
                "custom_tool_context": "",
            }, config=config)
            
            results_message = self.__extract_ai_content(result.get('messages', []))
            if not results_message:
                results_message = self.__content_to_text(result.get("answer", ""))
            if not results_message:
                results_message = "I could not generate a valid answer from the research process."

            results_message = minimax_helper.strip_think(results_message)
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
            
