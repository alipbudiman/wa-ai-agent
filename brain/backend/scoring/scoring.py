import json
import logging
from datetime import datetime, timezone
from typing import Any, Dict, List, Optional

from pydantic import BaseModel, Field

from backend.infrastructure.database.database import MongoDB
from backend.scoring import cosine_similarity, ragas

logger = logging.getLogger("uvicorn.error")


class RAGASDataValidation(BaseModel):
    """Raw inputs + computed RAGAS Faithfulness score for one agent response."""
    question: str
    response: str
    retrieved_contexts: List[str] = Field(default_factory=list)
    reference: Optional[str] = None
    faithfulness: Optional[float] = Field(default=None, ge=0.0, le=1.0)


class CosineSimilarityDataValidation(BaseModel):
    """Question/response pair + computed cosine similarity (relevancy) score."""
    question: str
    response: str
    similarity: Optional[float] = Field(default=None, ge=-1.0, le=1.0)


class ScoringStore(BaseModel):
    """One scored log persisted in ``MongoDB.SCORING_COLLECTION``.

    Captured at agent-call time (fast, no LLM): the ragas/cosine raw inputs plus
    the cheap indicators (latency, response length, tool-call count).
    Computed later by ``ScoringAgent.score`` when ``/show-agent-score`` runs:
    ``ragas.faithfulness``, ``cosine_similarity.similarity``, ``aspects``.
    """
    agent_name: str  # one of: main, reply, research, code
    ragas: RAGASDataValidation
    cosine_similarity: CosineSimilarityDataValidation
    aspects: Optional[Dict[str, Optional[int]]] = None
    latency_ms: Optional[int] = None
    response_length: Optional[int] = None
    tool_call_count: Optional[int] = None
    timestamp: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    scored: bool = False


def _mean(values: List[float]) -> Optional[float]:
    """Mean of a list, or ``None`` when empty (so JSON stays null, not NaN)."""
    return sum(values) / len(values) if values else None


def count_tool_calls(result: Any) -> int:
    """Count tool-invocation messages in a LangGraph result (no imports needed)."""
    messages = (result.get("messages") or []) if isinstance(result, dict) else []
    return sum(1 for m in messages if getattr(m, "type", "") == "tool")


def _content_to_text(content: Any) -> str:
    """Best-effort coercion of a tool/state value into a plain string."""
    if content is None:
        return ""
    if isinstance(content, str):
        return content
    try:
        return json.dumps(content, ensure_ascii=False, default=str)
    except Exception:
        return str(content)


def extract_retrieved_contexts(result: Any) -> List[str]:
    """Gather ALL grounding sources an agent used, for RAGAS Faithfulness.

    Without this, responses grounded in catalog / tool output / web results
    were scored faithfulness=0 because only ``rag_context`` was captured.
    Sources captured (deduped by exact string; never raises):
      1. ``rag_context`` -- main vector-RAG chunks (all agents)
      2. ``custom_tool_context`` -- tools-builder metadata (all agents)
      3. ``catalog_context`` -- catalog price/product preview (reply agent)
      4. executed ``ToolMessage`` contents in ``result['messages']``
         (main/reply/research; e.g. get_catalog_tool_by_id pricing payload)
      5. ``search`` / ``sum_search_msg`` / ``context`` (research web/wiki)
    """
    if not isinstance(result, dict):
        return []

    out: List[str] = []
    seen: set = set()

    def add(text: Any) -> None:
        t = _content_to_text(text).strip()
        if t and t not in seen:
            seen.add(t)
            out.append(t)

    add(result.get("rag_context"))
    add(result.get("custom_tool_context"))
    add(result.get("catalog_context"))
    for m in (result.get("messages") or []):
        if getattr(m, "type", "") != "tool":
            continue
        name = str(getattr(m, "name", "") or "tool").strip() or "tool"
        body = _content_to_text(getattr(m, "content", "")).strip()
        if body:
            add(f"[tool: {name}]\n{body}")
    for s in (result.get("search") or []):
        add(s)
    add(result.get("sum_search_msg"))
    for c in (result.get("context") or []):
        add(c)

    return out


class ScoringAgent:
    """Captures raw agent logs and scores them with RAGAS + cosine similarity."""

    def __init__(self, model: Optional[Any] = None):
        self.model = model
        self.scoring_collection = MongoDB.SCORING_COLLECTION

    def _collection(self):
        return MongoDB.get_collection(self.scoring_collection)

    def store_scoring_data(self, data: ScoringStore) -> bool:
        """Persist one raw scoring log (captured at agent-call time)."""
        try:
            result = self._collection().insert_one(data.model_dump())
            return bool(result.inserted_id)
        except Exception:
            logger.exception("Failed to store scoring data for agent=%s", data.agent_name)
            return False

    def get_scoring_data_lists(
        self, agent_name: str, max_results: int = 10
    ) -> List[Dict[str, Any]]:
        """Return the most recent raw scoring docs for an agent (newest first)."""
        cursor = (
            self._collection()
            .find({"agent_name": agent_name})
            .sort("timestamp", -1)
            .limit(max_results)
        )
        return list(cursor)

    def clear_scoring_data(self, agent_name: str) -> int:
        """Delete scoring docs. ``agent_name='all'`` clears every agent."""
        filt = {} if agent_name == "all" else {"agent_name": agent_name}
        try:
            result = self._collection().delete_many(filt)
            return int(result.deleted_count)
        except Exception:
            logger.exception("Failed to clear scoring data for agent=%s", agent_name)
            return 0

    def get_raw_scoring_data(
        self, agent_name: str, max_results: int = 50
    ) -> List[Dict[str, Any]]:
        """Return JSON-safe raw scoring docs for export.

        Includes the user input (``ragas.question``) and the AI output
        (``ragas.response``) plus every captured/computed field.
        """
        filt = {} if agent_name == "all" else {"agent_name": agent_name}
        cursor = (
            self._collection().find(filt).sort("timestamp", -1).limit(max_results)
        )
        docs: List[Dict[str, Any]] = []
        for doc in cursor:
            doc["_id"] = str(doc.get("_id", ""))
            ts = doc.get("timestamp")
            if isinstance(ts, datetime):
                doc["timestamp"] = ts.isoformat()
            docs.append(doc)
        return docs

    async def score(
        self, agent_name: str, max_results: int = 10
    ) -> Dict[str, Any]:
        """Score the last ``max_results`` logs and return per-log scores + averages."""
        raw_docs = self.get_scoring_data_lists(agent_name, max_results)

        logs: List[Dict[str, Any]] = []
        ragas_scores: List[float] = []
        cosine_scores: List[float] = []
        aspect_lists: Dict[str, List[int]] = {name: [] for name, _ in ragas.ASPECTS}
        latency_list: List[float] = []
        length_list: List[float] = []
        toolcall_list: List[float] = []

        for doc in raw_docs:
            stored = ScoringStore.model_validate(doc)
            ragas_val = await self._safe_faithfulness(stored)
            cosine_val = await self._safe_similarity(stored)
            aspects = await self._safe_aspects(stored.ragas.question, stored.ragas.response)

            if ragas_val is not None:
                ragas_scores.append(ragas_val)
            if cosine_val is not None:
                cosine_scores.append(cosine_val)
            for name, verdict in aspects.items():
                if verdict is not None:
                    aspect_lists.setdefault(name, []).append(verdict)
            if stored.latency_ms is not None:
                latency_list.append(stored.latency_ms)
            if stored.response_length is not None:
                length_list.append(stored.response_length)
            if stored.tool_call_count is not None:
                toolcall_list.append(stored.tool_call_count)

            logs.append({
                "timestamp": stored.timestamp.isoformat() if stored.timestamp else None,
                "question": stored.ragas.question,
                "response": stored.ragas.response,
                "ragas_faithfulness": ragas_val,
                "cosine_similarity": cosine_val,
                "aspects": aspects,
                "latency_ms": stored.latency_ms,
                "response_length": stored.response_length,
                "tool_call_count": stored.tool_call_count,
            })
            self._write_back_score(doc.get("_id"), ragas_val, cosine_val, aspects)

        return {
            "agent_name": agent_name,
            "count": len(logs),
            "logs": logs,
            "averages": {
                "ragas_faithfulness": _mean(ragas_scores),
                "cosine_similarity": _mean(cosine_scores),
                "aspects": {name: _mean(vals) for name, vals in aspect_lists.items()},
                "latency_ms": _mean(latency_list),
                "response_length": _mean(length_list),
                "tool_call_count": _mean(toolcall_list),
            },
        }

    async def _safe_faithfulness(self, stored: ScoringStore) -> Optional[float]:
        try:
            return await ragas.faithfulness_score(
                question=stored.ragas.question,
                response=stored.ragas.response,
                retrieved_contexts=stored.ragas.retrieved_contexts,
            )
        except Exception:
            logger.exception("RAGAS Faithfulness failed for a %s log", stored.agent_name)
            return None

    async def _safe_similarity(self, stored: ScoringStore) -> Optional[float]:
        try:
            return await cosine_similarity.relevance_score(
                stored.cosine_similarity.question, stored.cosine_similarity.response
            )
        except Exception:
            logger.exception("Cosine similarity failed for a %s log", stored.agent_name)
            return None

    async def _safe_aspects(self, question: str, response: str) -> Dict[str, Optional[int]]:
        try:
            return await ragas.aspect_scores(question, response)
        except Exception:
            logger.exception("Aspect scoring failed")
            return {name: None for name, _ in ragas.ASPECTS}

    def _write_back_score(
        self,
        doc_id: Any,
        ragas_val: Optional[float],
        cosine_val: Optional[float],
        aspects: Dict[str, Optional[int]],
    ) -> None:
        """Cache computed scores back onto the source document."""
        if doc_id is None:
            return
        try:
            self._collection().update_one(
                {"_id": doc_id},
                {"$set": {
                    "ragas.faithfulness": ragas_val,
                    "cosine_similarity.similarity": cosine_val,
                    "aspects": aspects,
                    "scored": True,
                }},
            )
        except Exception:
            logger.warning("Failed to write back scores for doc_id=%s", doc_id)


def capture_agent_log(
    agent_name: str,
    question: str,
    response: str,
    retrieved_context: Optional[Any] = None,
    latency_ms: Optional[int] = None,
    tool_call_count: Optional[int] = None,
) -> bool:
    """Fire-and-forget capture of one raw agent log. Never raises.

    ``latency_ms`` and ``tool_call_count`` are the cheap indicators captured at
    call time; the expensive RAGAS + cosine + aspect scoring happens later when
    ``/show-agent-score`` runs.
    """
    try:
        if not (question or response):
            return False

        if isinstance(retrieved_context, str):
            contexts = [retrieved_context] if retrieved_context.strip() else []
        elif isinstance(retrieved_context, (list, tuple)):
            contexts = [c for c in retrieved_context if c]
        else:
            contexts = []

        question = question or ""
        response = response or ""
        store = ScoringStore(
            agent_name=agent_name,
            ragas=RAGASDataValidation(
                question=question,
                response=response,
                retrieved_contexts=contexts,
            ),
            cosine_similarity=CosineSimilarityDataValidation(question=question, response=response),
            latency_ms=latency_ms,
            response_length=len(response),
            tool_call_count=tool_call_count,
        )
        return ScoringAgent().store_scoring_data(store)
    except Exception:
        logger.exception("capture_agent_log failed for agent=%s", agent_name)
        return False
