from backend.bootstrap.app_factory import create_app
from backend.application.ai.tools.filex import FileX

app = create_app()
filex_tool = FileX()

print("Playground folder:", filex_tool._playground_root)
print("RunSubagent folder:", filex_tool._run_subagent_root)
print("Agent skills folder:", filex_tool._agent_skills_root)
print("Claude instructions folder:", filex_tool._claude_instructions_root)


if __name__ == "__main__":
    import uvicorn

    uvicorn.run(
        "app:app",
        port=8000,
        reload=False,
        log_level="info",
        # log_config=None,
    )
