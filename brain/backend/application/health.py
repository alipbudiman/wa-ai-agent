from datetime import datetime, timezone

from backend.domain.health import HealthStatus


class CheckHealthUseCase:
    def execute(self) -> HealthStatus:
        return HealthStatus(
            status="ok",
            service="brain-agent-api",
            timestamp=datetime.now(timezone.utc),
        )
