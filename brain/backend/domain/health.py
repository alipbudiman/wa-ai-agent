from datetime import datetime
from pydantic import BaseModel


class HealthStatus(BaseModel):
    status: str
    service: str
    timestamp: datetime
