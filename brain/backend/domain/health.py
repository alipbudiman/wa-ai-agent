from dataclasses import dataclass
from datetime import datetime


@dataclass(frozen=True)
class HealthStatus:
    status: str
    service: str
    timestamp: datetime
