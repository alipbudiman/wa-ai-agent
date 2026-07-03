"""Cosine similarity (relevancy) between a user question and the agent response.

Embeddings come from the OpenAI provider config -- the same source the ingestion
pipeline uses (``application/ingestion/loader.py``) -- so a single configured key
powers both RAG and scoring. The score is a float in ``[-1.0, 1.0]`` (1.0 means
the question and response point in the same direction).
"""
from __future__ import annotations

import logging

import numpy as np
from langchain_openai import OpenAIEmbeddings

from config.config import Config

logger = logging.getLogger("uvicorn.error")

# Keep in sync with `application/ingestion/loader.py`.
EMBEDDING_MODEL = "text-embedding-3-small"


def _build_embeddings() -> OpenAIEmbeddings:
    """Build the embedding client from the configured OpenAI key."""
    openai_config = Config().get_openai_config()
    api_key = (openai_config.api_key or "").strip()
    if not api_key:
        raise RuntimeError(
            "OpenAI API key is not configured. Set it via /openai-config "
            "(manager_bot_config) before using cosine-similarity scoring."
        )
    return OpenAIEmbeddings(model=EMBEDDING_MODEL, api_key=api_key)


def cosine_similarity(question_vec, response_vec) -> float:
    """Pure cosine similarity between two equal-length vectors."""
    q = np.asarray(question_vec, dtype=float)
    r = np.asarray(response_vec, dtype=float)
    denom = float(np.linalg.norm(q) * np.linalg.norm(r))
    if denom == 0.0:
        return 0.0
    return float(np.dot(q, r) / denom)


async def relevance_score(question: str, response: str) -> float:
    """Embed ``question`` and ``response`` and return their cosine similarity."""
    if not question or not response:
        return 0.0
    embeddings = _build_embeddings()
    question_vec = await embeddings.aembed_query(question)
    response_vec = await embeddings.aembed_query(response)
    return cosine_similarity(question_vec, response_vec)
