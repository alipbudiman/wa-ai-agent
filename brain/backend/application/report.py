from langsmith import Client

from backend.domain.report import LangSmithLogReport, LangSmithRunReport
from config.config import Config

class ReportUseCase:
    def __init__(self):
        self.langsmith_client = Client()  # Initialize LangSmith client for tracing
        self.config = Config()

    def _collect_tool_names(self, run) -> set[str]:
        """Collect tool run names recursively from a LangSmith run tree."""
        collected: set[str] = set()

        for child in (getattr(run, "child_runs", None) or []):
            child_name = str(getattr(child, "name", "") or "").strip()
            child_type = str(getattr(child, "run_type", "") or "").strip().lower()

            if child_type == "tool" and child_name and child_name.lower() != "tools":
                collected.add(child_name)

            nested_names = self._collect_tool_names(child)
            if nested_names:
                collected.update(nested_names)

        return collected
    
    def get_list(self) -> list[LangSmithRunReport]:
        langsmith_project_name = self.config.get_langsmith_config().project_name or "default"
        runs = self.langsmith_client.list_runs(
            project_name=langsmith_project_name,
            execution_order=1,
            limit=10
        )
        
        list_report = []

        for run in runs:
            report = LangSmithRunReport(
                id=str(run.id),
                name=run.name,
                run_type=run.run_type
            )
            list_report.append(report)
        return list_report
    
    def select_run(self, selected_run_id: str) -> LangSmithLogReport:
        run_detail = self.langsmith_client.read_run(
            run_id=selected_run_id,
            load_child_runs=True,  # include child spans/runs
        )

        extra_payload = dict(run_detail.extra or {})
        tool_names = sorted(self._collect_tool_names(run_detail))
        if tool_names:
            extra_payload["tool_names"] = tool_names

        child_runs = []
        for child in (getattr(run_detail, "child_runs", None) or []):
            child_runs.append(
                LangSmithRunReport(
                    id=str(child.id),
                    name=child.name or "",
                    run_type=child.run_type or "",
                )
            )

        return LangSmithLogReport(
            id=str(run_detail.id),
            name=run_detail.name or "",
            start_time=run_detail.start_time,
            run_type=run_detail.run_type or "",
            end_time=run_detail.end_time,
            extra=extra_payload,
            error=run_detail.error,
            serialized=run_detail.serialized,
            events=run_detail.events or [],
            inputs=run_detail.inputs or {},
            outputs=run_detail.outputs or {},
            reference_example_id=str(run_detail.reference_example_id) if run_detail.reference_example_id else None,
            parent_run_id=str(run_detail.parent_run_id) if run_detail.parent_run_id else None,
            tags=run_detail.tags or [],
            attachments=run_detail.attachments or {},
            session_id=str(run_detail.session_id) if run_detail.session_id else None,
            child_run_ids=[str(child_id) for child_id in (run_detail.child_run_ids or [])],
            child_runs=child_runs,
            feedback_stats=run_detail.feedback_stats,
            app_path=run_detail.app_path,
            manifest_id=str(run_detail.manifest_id) if run_detail.manifest_id else None,
            status=run_detail.status,
            prompt_tokens=run_detail.prompt_tokens,
            completion_tokens=run_detail.completion_tokens,
            total_tokens=run_detail.total_tokens,
            prompt_token_details=run_detail.prompt_token_details or {},
            completion_token_details=run_detail.completion_token_details or {},
            first_token_time=run_detail.first_token_time,
            total_cost=run_detail.total_cost,
            prompt_cost=run_detail.prompt_cost,
            completion_cost=run_detail.completion_cost,
            prompt_cost_details=run_detail.prompt_cost_details or {},
            completion_cost_details=run_detail.completion_cost_details or {},
            parent_run_ids=[str(parent_id) for parent_id in (run_detail.parent_run_ids or [])],
            trace_id=str(run_detail.trace_id) if run_detail.trace_id else None,
            dotted_order=run_detail.dotted_order,
            in_dataset=run_detail.in_dataset,
        )