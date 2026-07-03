"""RAGAS scoring for an agent response: Faithfulness + custom AspectCritic.

- Faithfulness: is the response grounded in the retrieved context (hallucination)?
- AspectCritic: binary (0/1) LLM-judge on business-relevant aspects (courtesy,
  language match, conciseness, scope adherence).

Both are LLM-judge only (no embeddings). The evaluator LLM is a langchain
``ChatOpenAI`` (gpt-4o by default, from the OpenAI provider config) wrapped via
ragas ``LangchainLLMWrapper``. ragas judges at near-zero temperature, which
reasoning models (e.g. gpt-5) reject, so a non-reasoning judge model is required
(override with the ``SCORING_JUDGE_MODEL`` env var).

RAGAS is imported lazily so the brain API still boots if ``ragas`` is missing;
scoring only fails when actually invoked.
"""
from __future__ import annotations

import logging
import math
import os
from typing import Dict, List, Optional, Tuple

from config.config import Config

logger = logging.getLogger("uvicorn.error")

_evaluator_llm = None
_faithfulness_metric = None
_aspect_critics = None

# Custom aspects judged per response: (name, judging definition). Each returns
# binary 0/1; the average across logs is the pass-rate for that aspect.
ASPECTS: List[Tuple[str, str]] = [
    ("courtesy", "Is the response polite, respectful, and courteous to the user?"),
    (
        "language_match",
        "Does the response reply in the same language the user used in their "
        "question (e.g. Indonesian if the question is in Indonesian)?",
    ),
    (
        "conciseness",
        "Is the response concise and free of unnecessary verbosity, suitable "
        "for a WhatsApp chat message?",
    ),
    (
        "scope_adherence",
        "Does the response stay on-topic and directly address the user's "
        "question without introducing unrelated information?",
    ),
]


def _get_evaluator_llm():
    """Build and cache the RAGAS evaluator LLM (langchain ChatOpenAI wrapped)."""
    global _evaluator_llm
    if _evaluator_llm is not None:
        return _evaluator_llm

    from langchain_openai import ChatOpenAI
    from ragas.llms import LangchainLLMWrapper

    openai_config = Config().get_openai_config()
    api_key = (openai_config.api_key or "").strip()
    if not api_key:
        raise RuntimeError(
            "OpenAI API key is not configured. Set it via /openai-config "
            "(manager_bot_config) before using RAGAS scoring."
        )

    chat_llm = ChatOpenAI(model=os.getenv("SCORING_JUDGE_MODEL", "gpt-4o"), api_key=api_key)
    _evaluator_llm = LangchainLLMWrapper(chat_llm)
    return _evaluator_llm


def _get_faithfulness_metric():
    global _faithfulness_metric
    if _faithfulness_metric is None:
        from ragas.metrics import Faithfulness

        _faithfulness_metric = Faithfulness(llm=_get_evaluator_llm())
    return _faithfulness_metric


def _get_aspect_critics():
    global _aspect_critics
    if _aspect_critics is None:
        from ragas.metrics import AspectCritic

        llm = _get_evaluator_llm()
        _aspect_critics = [
            AspectCritic(name=name, definition=definition, llm=llm)
            for name, definition in ASPECTS
        ]
    return _aspect_critics


async def faithfulness_score(
    question: str, response: str, retrieved_contexts: List[str]
) -> Optional[float]:
    """Return RAGAS Faithfulness in ``[0.0, 1.0]``, or ``None`` if uncomputable."""
    from ragas.dataset_schema import SingleTurnSample

    if not response or not retrieved_contexts:
        return None

    sample = SingleTurnSample(
        user_input=question,
        response=response,
        retrieved_contexts=list(retrieved_contexts),
    )
    # ragas 0.3.x: the deprecated `ascore` mis-dispatches; use single_turn_ascore.
    score = await _get_faithfulness_metric().single_turn_ascore(sample)
    if score is None or (isinstance(score, float) and math.isnan(score)):
        logger.warning("RAGAS Faithfulness returned no score for a sample.")
        return None
    return float(score)


async def aspect_scores(question: str, response: str) -> Dict[str, Optional[int]]:
    """Judge each custom aspect; return ``{aspect_name: 0 | 1 | None}``."""
    from ragas.dataset_schema import SingleTurnSample

    if not response:
        return {name: None for name, _ in ASPECTS}

    sample = SingleTurnSample(user_input=question or "", response=response)
    results: Dict[str, Optional[int]] = {}
    for critic in _get_aspect_critics():
        try:
            verdict = await critic.single_turn_ascore(sample)
            results[critic.name] = int(verdict) if verdict is not None else None
        except Exception:
            logger.exception("AspectCritic '%s' failed", critic.name)
            results[critic.name] = None
    return results
