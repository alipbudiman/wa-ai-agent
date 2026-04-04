# WaNexus V2

Special Thanks to Amazing Technologies that made this project possible.
- LangChain: https://www.langchain.com/
- Tavily: https://tavily.com/
- WhatsMeow: https://whatsmeow.dev/


WhatsApp Bot server docs: http://localhost:8080/swagger/index.html
Brain API docs: http://localhost:8000/docs

Manager UI: http://localhost:8080/access

**System Highlights:**

* Integrated large language model (LLM) for reasoning and response generation.
* LangChain/LangGraph agent flow for routing, tool usage, and memory-aware execution.
* Multi-API handlers for WhatsApp operations (message sending, contact lookup, help menu, broadcast).
* Interconnected architecture between Go WhatsApp runtime and Python Brain API.
* Automated task handler pipelines for consistent and repeatable workflows.
* Cron-ready scheduler for reminders, periodic checks, and notifications.
* Multi-step workflow chaining (lookup → enrich → summarize → send).
* Observability and tracing for debugging and performance optimization.
