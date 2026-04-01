


from datetime import datetime


class TimeTool:
    def __init__(self):
        pass
    
    def current_time(self) -> str:
        """Returns the current time as a string. some tools include the current time in their query,
        so don't use the system time directly in the agent's response to avoid confusion."""
        time_humanize_now = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
        return f"The current time is: {time_humanize_now}"