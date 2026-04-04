from backend.bootstrap.app_factory import create_app
from backend.application.report import ReportUseCase

app = create_app()


if __name__ == "__main__":
    import uvicorn

    uvicorn.run("app:app", port=8000, reload=False, log_level="info")
