from datetime import datetime
import json
import os
import logging
from langchain_tavily import TavilySearch

from backend.application.ai.ai import AI
from config.config import Config


logger = logging.getLogger('uvicorn.error')


class WebSearchTool:
    def __init__(self, ai: AI):
        self.llm = ai
    
    def _extract_tavily_results(self, raw_results) -> list:
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
    
    def websearch(self, query: str, topic: str = 'general') -> str:
        """Search the web with Tavily and return relevant results for the query."""

        if Config.TAVILY_API_KEY:
            os.environ['TAVILY_API_KEY'] = Config.TAVILY_API_KEY

        if not os.getenv('TAVILY_API_KEY'):
            raise ValueError('TAVILY_API_KEY is not set. Add it to your environment file.')
        
        time_humanize_now = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
        
        tavily_search_tool = TavilySearch(
            max_results=3,
            topic=topic
        )
        
        query = f"{query} (Search Date: {time_humanize_now})"
        raw_results = tavily_search_tool.run(query)
        results = self._extract_tavily_results(raw_results)
        if not results:
            return 'No web results found for that query.'

        summerized_results = []
        source_urls = []
        for result in results:
            summerized_results.append(f"Title: {result['title']}\nURL: {result['url']}\nSummary: {result['summary']}\n")
            source_urls.append(result['url'])
            logger.info(
                "Tavily result:\nTitle: %s\nURL: %s\nSummary: %s\n",
                result['title'],
                result['url'],
                result['summary'],
            )
        
        prompt = f"Here are the search results for the query: '{query}':\n\n" + "\n".join(summerized_results) + "\n\nPlease provide a concise summary of the most relevant information from these search results that can help answer the query."
        summary = self.llm.invoke(prompt)

        final_content = "*Using Web Search*\n"
        final_content += summary.content if hasattr(summary, 'content') else str(summary)
        final_content += "\n\n*Sources:*\n"
        for url in source_urls:
            final_content += f"- {url}\n"
        
        return final_content
    

             