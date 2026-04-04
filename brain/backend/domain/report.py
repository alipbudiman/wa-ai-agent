from datetime import datetime
from typing import Any

from pydantic import BaseModel, Field

class LangSmithRunReport(BaseModel):
    id: str
    name: str
    run_type: str

class LangSmithRunEvent(BaseModel):
    name: str
    time: str | None = None


class LangSmithLogReport(BaseModel):
    id: str
    name: str
    start_time: datetime | None = None
    run_type: str
    end_time: datetime | None = None

    extra: dict[str, Any] = Field(default_factory=dict)
    error: str | None = None
    serialized: Any = None
    events: list[LangSmithRunEvent] = Field(default_factory=list)
    inputs: dict[str, Any] = Field(default_factory=dict)
    outputs: dict[str, Any] = Field(default_factory=dict)

    reference_example_id: str | None = None
    parent_run_id: str | None = None
    tags: list[str] = Field(default_factory=list)
    attachments: dict[str, Any] = Field(default_factory=dict)
    session_id: str | None = None
    child_run_ids: list[str] = Field(default_factory=list)
    child_runs: list[LangSmithRunReport] = Field(default_factory=list)
    feedback_stats: Any = None

    app_path: str | None = None
    manifest_id: str | None = None
    status: str | None = None

    prompt_tokens: int | None = None
    completion_tokens: int | None = None
    total_tokens: int | None = None
    prompt_token_details: dict[str, Any] = Field(default_factory=dict)
    completion_token_details: dict[str, Any] = Field(default_factory=dict)

    first_token_time: datetime | None = None
    total_cost: float | None = None
    prompt_cost: float | None = None
    completion_cost: float | None = None
    prompt_cost_details: dict[str, Any] = Field(default_factory=dict)
    completion_cost_details: dict[str, Any] = Field(default_factory=dict)

    parent_run_ids: list[str] = Field(default_factory=list)
    trace_id: str | None = None
    dotted_order: str | None = None
    in_dataset: bool | None = None
