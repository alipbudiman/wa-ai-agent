from __future__ import annotations

import inspect
import json
import re
import uuid
from typing import Any, Callable

from langchain_core.messages import AIMessage, HumanMessage, SystemMessage, ToolMessage


def content_to_text(content: Any) -> str:
    if isinstance(content, str):
        return content.strip()
    if hasattr(content, "content"):
        return content_to_text(getattr(content, "content"))
    if isinstance(content, list):
        text_parts: list[str] = []
        for part in content:
            if isinstance(part, str):
                text_parts.append(part)
                continue
            if isinstance(part, dict):
                text = part.get("text")
                if isinstance(text, str) and text.strip():
                    text_parts.append(text.strip())
        return "\n".join(text_parts).strip()
    if content is None:
        return ""
    return str(content).strip()


def strip_think(text: str) -> str:
    return re.sub(r"<think>.*?</think>", "", str(text or ""), flags=re.IGNORECASE | re.DOTALL).strip()


def parse_approval_decision(text: Any) -> str:
    raw = content_to_text(text).strip().lower()
    if not raw:
        return ""

    # Cancellation markers must win over generic approval words so phrases
    # like "do not send" are not misread as approve because they include send.
    cancel_markers = {
        "cancel",
        "canceled",
        "cancelled",
        "reject",
        "rejected",
        "deny",
        "denied",
        "no",
        "nope",
        "tidak",
        "nggak",
        "ga",
        "gak",
        "batal",
        "jangan",
        "/unsend",
    }
    approve_markers = {
        "approve",
        "approved",
        "confirm",
        "confirmed",
        "yes",
        "y",
        "ok",
        "okay",
        "ya",
        "iya",
        "setuju",
        "lanjut",
        "kirim",
        "send",
        "/send",
    }

    tokens = set(re.findall(r"/?[a-zA-Z0-9_]+", raw))
    if tokens & cancel_markers:
        return "cancel"
    if tokens & approve_markers:
        return "approve"

    if "is_approved" in raw:
        if "cancel" in raw:
            return "cancel"
        if "approve" in raw:
            return "approve"

    return ""


def selected_provider(config: Any, agent_model_field: str) -> str:
    try:
        ai_config = config.get_agent_model_config()
        model_config = getattr(ai_config, agent_model_field, None)
        provider = str(getattr(model_config, "agent_provider", "") or "").strip().lower()
        if provider and provider != "default":
            return provider
    except Exception:
        pass

    try:
        provider_config = config.get_provider_config()
        provider = str(getattr(provider_config, "providers", "") or "").strip().lower()
        return provider.split(",", 1)[0].strip()
    except Exception:
        return ""


def is_minimax_provider(config: Any, agent_model_field: str) -> bool:
    return selected_provider(config, agent_model_field) == "minimax"


def is_executed_tools_log(text: str) -> bool:
    return str(text or "").strip().lower().startswith("[executed tools:")


def tool_object_name(tool: Any) -> str:
    return str(getattr(tool, "name", None) or getattr(tool, "__name__", "") or "").strip()


def looks_like_textual_tool_call(text: str) -> bool:
    normalized = (text or "").strip().upper()
    return (
        "[TOOL_CALL]" in normalized and "[/TOOL_CALL]" in normalized
    ) or (
        "<MINIMAX:TOOL_CALL>" in normalized and "</MINIMAX:TOOL_CALL>" in normalized
    )


def canonical_tool_name(requested_name: str, tools: list) -> str:
    requested = str(requested_name or "").strip()
    if not requested:
        return ""

    aliases: dict[str, str] = {}
    for tool in tools:
        name = tool_object_name(tool)
        if not name:
            continue
        aliases[name] = name
        aliases[name.lower()] = name
        aliases[name.replace("-", "_").lower()] = name
        aliases[name.replace("_", "-").lower()] = name

    return aliases.get(
        requested,
        aliases.get(
            requested.lower(),
            aliases.get(requested.replace("-", "_").lower(), requested),
        ),
    )


def tool_args_schema_text(tool: Any) -> str:
    args_schema = getattr(tool, "args_schema", None)
    if args_schema:
        try:
            schema = args_schema.model_json_schema()
            properties = schema.get("properties", {}) if isinstance(schema, dict) else {}
            required = set(schema.get("required", [])) if isinstance(schema, dict) else set()
            if properties:
                parts = []
                for arg_name, arg_schema in properties.items():
                    arg_type = arg_schema.get("type", "any") if isinstance(arg_schema, dict) else "any"
                    suffix = " required" if arg_name in required else " optional"
                    parts.append(f"{arg_name}: {arg_type}{suffix}")
                return ", ".join(parts)
        except Exception:
            pass

    args = getattr(tool, "args", None)
    if isinstance(args, dict) and args:
        parts = []
        for arg_name, arg_schema in args.items():
            arg_type = arg_schema.get("type", "any") if isinstance(arg_schema, dict) else "any"
            parts.append(f"{arg_name}: {arg_type}")
        return ", ".join(parts)

    try:
        signature = inspect.signature(tool)
        params = []
        for name, parameter in signature.parameters.items():
            if name in {"self", "cls"}:
                continue
            required = parameter.default is inspect.Parameter.empty
            suffix = " required" if required else " optional"
            params.append(f"{name}: any{suffix}")
        return ", ".join(params)
    except (TypeError, ValueError):
        return ""


def render_textual_tool_catalog(tools: list) -> str:
    lines = []
    for tool in tools:
        name = tool_object_name(tool)
        if not name:
            continue

        description = str(
            getattr(tool, "description", None)
            or inspect.getdoc(tool)
            or ""
        ).strip()
        args_text = tool_args_schema_text(tool)

        line = f"- {name}"
        if args_text:
            line += f"({args_text})"
        if description:
            line += f": {description}"
        lines.append(line)

    return "\n".join(lines)


def textual_tool_instruction(
    tools: list,
    *,
    prefer_find_first: bool = False,
    result_rule: str | None = None,
) -> str:
    rules = [
        "- Use only tool names listed below.",
        "- Include one <parameter> tag for each argument the tool needs.",
    ]
    if prefer_find_first:
        rules.append("- Prefer `find_relevant_custom_tools` first when unsure.")
        rules.append("- Use `execute_custom_tool` after the matching tool is known.")
    rules.append(result_rule or "- If no tool is needed, answer normally.")
    rules.append("- After a tool result appears in the conversation, use it to produce the final answer; do not call the same tool again unless more information is required.")

    return (
        "TEXTUAL TOOL-CALL MODE:\n"
        "Native provider tool_calls are disabled for this turn. When a tool is needed, "
        "return exactly one textual tool call and no other prose.\n\n"
        "Format:\n"
        "<minimax:tool_call><invoke name=\"tool_name\">"
        "<parameter name=\"arg_name\">arg value</parameter>"
        "</invoke></minimax:tool_call>\n\n"
        "Rules:\n"
        + "\n".join(rules)
        + "\n\nAVAILABLE TOOLS:\n"
        + render_textual_tool_catalog(tools)
    )


def _coerce_tool_args(args: Any) -> dict:
    if isinstance(args, dict):
        return args
    if isinstance(args, str):
        stripped = args.strip()
        if not stripped:
            return {}
        try:
            parsed = json.loads(stripped)
            return parsed if isinstance(parsed, dict) else {"value": parsed}
        except json.JSONDecodeError:
            return {"raw": stripped}
    if args is None:
        return {}
    return {"value": str(args)}


def _extract_json_like_args(block: str) -> dict:
    args_match = re.search(r"args\s*=>\s*\{(.*)\}", block, flags=re.IGNORECASE | re.DOTALL)
    if not args_match:
        return {}

    raw_args = re.sub(r"--[a-zA-Z0-9_-]+--", "", args_match.group(1))
    json_like = "{" + raw_args + "}"
    json_like = json_like.replace("=>", ":")
    try:
        parsed = json.loads(json_like)
        return parsed if isinstance(parsed, dict) else {}
    except json.JSONDecodeError:
        return {}


def _gateway_tool_name(valid_names: set[str]) -> str:
    for candidate in ("execute_custom_tool", "access_tools_builder"):
        if candidate in valid_names:
            return candidate
    return ""


def _normalize_textual_tool_call(raw_tool_name: str, args: dict, tools: list) -> tuple[str, dict]:
    canonical_name = canonical_tool_name(raw_tool_name, tools)
    valid_names = {tool_object_name(tool) for tool in tools if tool_object_name(tool)}
    if not valid_names or canonical_name in valid_names:
        return canonical_name, args

    gateway_name = _gateway_tool_name(valid_names)
    if not gateway_name:
        return canonical_name, args

    gateway_args = dict(args)
    gateway_args.setdefault("tool_name", raw_tool_name)

    extra_payload = {
        key: value
        for key, value in gateway_args.items()
        if key not in {"tool_name", "tool_input"}
    }
    if extra_payload and "tool_input" not in gateway_args:
        gateway_args["tool_input"] = json.dumps(extra_payload, ensure_ascii=False)
        for key in extra_payload:
            gateway_args.pop(key, None)

    return gateway_name, gateway_args


def extract_textual_tool_calls(text: str, tools: list | None = None, *, id_prefix: str = "minimax") -> list[dict]:
    source = (text or "").strip()
    if not looks_like_textual_tool_call(source):
        return []

    tools = tools or []
    parsed_calls: list[dict] = []

    minimax_blocks = re.findall(
        r"<minimax:tool_call>(.*?)</minimax:tool_call>",
        source,
        flags=re.IGNORECASE | re.DOTALL,
    )
    for block in minimax_blocks:
        invokes = re.findall(
            r"<?\s*invoke\s+name=(['\"])(.*?)\1\s*>(.*?)</\s*invoke\s*>",
            block,
            flags=re.IGNORECASE | re.DOTALL,
        )
        for _, raw_tool_name, params_block in invokes:
            args = {}
            params = re.findall(
                r"<\s*parameter\s+name=(['\"])(.*?)\1\s*>(.*?)</\s*parameter\s*>",
                params_block,
                flags=re.IGNORECASE | re.DOTALL,
            )
            for _, param_name, param_value in params:
                args[param_name.strip()] = param_value.strip()

            tool_name, args = _normalize_textual_tool_call(raw_tool_name, args, tools)
            parsed_calls.append({
                "name": tool_name,
                "args": args,
                "id": f"{id_prefix}-{uuid.uuid4().hex[:12]}",
                "type": "tool_call",
            })

    blocks = re.findall(r"\[TOOL_CALL\](.*?)\[/TOOL_CALL\]", source, flags=re.IGNORECASE | re.DOTALL)
    for block in blocks:
        tool_match = re.search(r"tool\s*=>\s*[\"']?([a-zA-Z0-9_.:-]+)", block, flags=re.IGNORECASE)
        if not tool_match:
            continue

        raw_tool_name = tool_match.group(1).strip()
        args = _extract_json_like_args(block)
        if not args:
            for key, value in re.findall(
                r'"([a-zA-Z0-9_:-]+)"\s*:\s*"((?:\\.|[^"\\])*)"',
                block,
                flags=re.DOTALL,
            ):
                args[key] = value

        tool_name, args = _normalize_textual_tool_call(raw_tool_name, _coerce_tool_args(args), tools)
        parsed_calls.append({
            "name": tool_name,
            "args": args,
            "id": f"{id_prefix}-{uuid.uuid4().hex[:12]}",
            "type": "tool_call",
        })

    return parsed_calls


def coerce_textual_tool_response(
    response: Any,
    tools: list,
    *,
    logger: Any | None = None,
    id_prefix: str = "minimax",
) -> Any:
    if not isinstance(response, AIMessage):
        return response

    native_calls = getattr(response, "tool_calls", None) or []
    if native_calls:
        return response

    text_content = content_to_text(response.content)
    parsed_calls = extract_textual_tool_calls(text_content, tools, id_prefix=id_prefix)
    if not parsed_calls:
        return response

    if logger:
        logger.warning("Assistant returned textual tool-call markup; coercing to structured tool call.")
    return AIMessage(content="", tool_calls=parsed_calls)


def tool_result_message_text(tool_name: str, content: str) -> str:
    return (
        f"Tool '{tool_name or 'unknown'}' result:\n{content}\n\n"
        "Use this result to answer the user's latest request. "
        f"Do not call `{tool_name or 'unknown'}` again unless more information is required."
    )


def scrub_tool_history_for_plain_llm(
    messages: list,
    *,
    minimax_provider: bool,
    content_to_text_func: Callable[[Any], str] = content_to_text,
) -> list:
    scrubbed_messages = []
    for msg in messages:
        if isinstance(msg, AIMessage):
            tool_calls = getattr(msg, "tool_calls", [])
            if tool_calls:
                if minimax_provider:
                    continue
                tool_names = [call.get("name", "unknown") for call in tool_calls if isinstance(call, dict)]
                content = content_to_text_func(getattr(msg, "content", ""))
                text = f"{content}\n[Executed tools: {', '.join(tool_names)}]" if content else f"[Executed tools: {', '.join(tool_names)}]"
                scrubbed_messages.append(AIMessage(content=text.strip()))
                continue
            scrubbed_messages.append(msg)
            continue

        if isinstance(msg, ToolMessage):
            tool_name = str(getattr(msg, "name", "unknown") or "unknown")
            content = content_to_text_func(getattr(msg, "content", ""))
            tool_result_text = tool_result_message_text(tool_name, content)
            if minimax_provider:
                scrubbed_messages.append(HumanMessage(content=tool_result_text))
            else:
                scrubbed_messages.append(SystemMessage(content=tool_result_text))
            continue

        scrubbed_messages.append(msg)

    return scrubbed_messages


def minimax_safe_payload(
    messages: list,
    *,
    is_internal_log: Callable[[str], bool] | None = None,
    content_to_text_func: Callable[[Any], str] = content_to_text,
) -> list:
    system_parts: list[str] = []
    safe_messages: list = []

    for message in messages:
        content = content_to_text_func(getattr(message, "content", "")).strip()
        if not content:
            continue

        if isinstance(message, SystemMessage):
            system_parts.append(content)
            continue

        if isinstance(message, ToolMessage):
            tool_name = getattr(message, "name", "unknown")
            safe_messages.append(HumanMessage(content=f"Tool '{tool_name}' result:\n{content}"))
            continue

        if isinstance(message, AIMessage):
            if getattr(message, "tool_calls", None):
                continue
            if looks_like_textual_tool_call(content):
                continue
            if is_internal_log and is_internal_log(content):
                continue
            safe_messages.append(AIMessage(content=content))
            continue

        if isinstance(message, HumanMessage):
            safe_messages.append(HumanMessage(content=content))
            continue

        safe_messages.append(HumanMessage(content=content))

    if system_parts:
        safe_messages.insert(0, SystemMessage(content="\n\n".join(system_parts)))

    return safe_messages


def format_generic_tool_feedback(tool_name: str, tool_text: str, *, max_chars: int = 12000) -> str:
    text = (tool_text or "").strip()
    if not text:
        return f"Tool `{tool_name}` finished, but returned no output."
    if len(text) > max_chars:
        text = text[:max_chars].rstrip() + "\n... [output truncated] ..."
    return text
