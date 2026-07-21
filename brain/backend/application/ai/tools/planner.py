class Planner:
    def __init__(self, tools: list):
        self.tools = tools

    def plan(self, task: str) -> str:
        # Implement planning logic here
        return f"Planning for task: {task} using tools: {', '.join(self.tools)}"