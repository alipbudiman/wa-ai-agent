import os
import json

from backend.application.ai import AI
from config.config import Config
from langchain_tavily import TavilySearch

from langgraph.graph import StateGraph, START
from langgraph.graph import MessagesState
from langgraph.prebuilt import ToolNode
from langgraph.prebuilt import tools_condition
from langchain_core.messages import AIMessage, HumanMessage

# tools

def _extract_tavily_results(raw_results) -> list:
    """Normalize Tavily output into a list of result dictionaries."""

    if isinstance(raw_results, str):
        try:
            raw_results = json.loads(raw_results)
        except json.JSONDecodeError:
            return []

    if isinstance(raw_results, dict):
        results = raw_results.get('results', [])
    elif isinstance(raw_results, list):
        results = raw_results
    else:
        return []

    normalized = []
    for item in results:
        if not isinstance(item, dict):
            continue

        normalized.append({
            'title': str(item.get('title', 'Untitled')),
            'url': str(item.get('url', '-')),
            'summary': str(item.get('summary') or item.get('content') or ''),
        })

    return normalized

def websearch(query: str, max_results: int = 3, topic: str = 'general') -> str:
    """Search the web with Tavily and return relevant results for the query."""

    if Config.TAVILY_API_KEY:
        os.environ['TAVILY_API_KEY'] = Config.TAVILY_API_KEY

    if not os.getenv('TAVILY_API_KEY'):
        raise ValueError('TAVILY_API_KEY is not set. Add it to your environment file.')

    search = TavilySearch(
        max_results=max_results,
        topic=topic
    )
    raw_results = search.run(query)
    results = _extract_tavily_results(raw_results)
    if not results:
        return 'No web results found for that query.'

    summerized_results = []
    for result in results:
        summerized_results.append(f"Title: {result['title']}\nURL: {result['url']}\nSummary: {result['summary']}\n")
    
    llm = AI(api_key=Config.OPENROUTER_API_KEY).LLM()
    prompt = f"Here are the search results for the query: '{query}':\n\n" + "\n".join(summerized_results) + "\n\nPlease provide a concise summary of the most relevant information from these search results that can help answer the query."
    summary = llm.invoke(prompt)

    return summary.content if hasattr(summary, 'content') else str(summary)

class ChatAgentUseCase:
    def __init__(self):
        self.llm = AI(api_key=Config.OPENROUTER_API_KEY).LLM()
    
    def __tools(self) -> list:
        builder = StateGraph(MessagesState)
        builder.add_node("tool_calling_llm", self.__tool_calling_llm)
        builder.add_node("tools", ToolNode([websearch]))
        builder.add_edge(START, "tool_calling_llm")
        builder.add_conditional_edges(
            "tool_calling_llm",
            # If the latest message (result) from assistant is a tool call -> tools_condition routes to tools
            # If the latest message (result) from assistant is a not a tool call -> tools_condition routes to END
            tools_condition,
        )
        builder.add_edge("tools", "tool_calling_llm")
        graph = builder.compile()
        
        return graph

    def __tool_calling_llm(self, state: MessagesState):
        llm_with_tools = self.llm.bind_tools([websearch])
        return {"messages": [llm_with_tools.invoke(state["messages"])]}

    def __extract_ai_content(self, messages: list) -> str:
        ai_messages = [message for message in messages if isinstance(message, AIMessage)]
        if not ai_messages:
            return ''

        final_ai_message = ai_messages[-1]
        content = final_ai_message.content
        if isinstance(content, str):
            return content.strip()

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
            return '\n'.join(text_parts).strip()

        return str(content).strip()
    
    def ChatAgent(self, messages: str):
        graph = self.__tools()
        input_messages = [HumanMessage(content=messages)]
        result = graph.invoke({"messages": input_messages})
        return self.__extract_ai_content(result.get('messages', []))