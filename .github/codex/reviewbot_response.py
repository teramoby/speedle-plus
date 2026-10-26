# Adapted from Pavu a7b458e; modified for Speedle+. Apache-2.0; see .github/codex/PAVU-LICENSE.txt.
"""Extract an Ark review without logging response text. Exit 10 permits one retry."""

import json
from pathlib import Path
import re
import sys


STATUSES = {"completed", "incomplete", "failed", "in_progress", "queued", "cancelled"}
REASONS = {"max_output_tokens", "content_filter"}
ITEM_TYPES = {"message", "reasoning", "function_call", "web_search_call"}


def format_markdown(message):
    """Unwrap a whole ReviewBot report, preserving code examples inside it."""
    message = message.strip()
    wrapper = re.fullmatch(
        r"(?P<fence>`{3,}|~{3,})[ \t]*(?:text|markdown|md)?[ \t]*\r?\n"
        r"(?P<body>[\s\S]*?)\r?\n(?P=fence)[ \t]*",
        message, re.IGNORECASE,
    )
    if wrapper and re.match(r"## (?:Ark )?ReviewBot\b", wrapper["body"].lstrip()):
        message = wrapper["body"].strip()
    message = re.sub(r"\A## Ark ReviewBot(?=[ \t]|\r?$)",
                     "## ReviewBot", message, flags=re.MULTILINE)
    old_signature = "— **Ark ReviewBot**"
    if message.endswith(old_signature):
        message = message[:-len(old_signature)] + "— **ReviewBot**"
    return message


def label(value, allowed):
    if value is None:
        return "missing"
    return value if isinstance(value, str) and value in allowed else "other"


def diagnose(reason, response=None):
    # All string labels are allowlisted. Never log raw values, model text,
    # reasoning summaries, error messages, input, or JSON parser exceptions.
    response = response if isinstance(response, dict) else {}
    details = response.get("incomplete_details")
    details = details if isinstance(details, dict) else {}
    output = response.get("output")
    output = output if isinstance(output, list) else []
    usage = response.get("usage")
    usage = usage if isinstance(usage, dict) else {}
    tokens = {}
    for key in ("input_tokens", "output_tokens", "total_tokens"):
        value = usage.get(key)
        if type(value) is int and 0 <= value <= 10**12:
            tokens[key] = value
    output_details = usage.get("output_tokens_details")
    if isinstance(output_details, dict):
        value = output_details.get("reasoning_tokens")
        if type(value) is int and 0 <= value <= 10**12:
            tokens["reasoning_tokens"] = value
    diagnostic = {
        "reason_code": reason,
        "status": label(response.get("status"), STATUSES),
        "incomplete_reason": label(details.get("reason"), REASONS),
        "output_types": sorted({label(item.get("type"), ITEM_TYPES)
                                if isinstance(item, dict) else "other" for item in output}),
        "usage": tokens,
    }
    print("ReviewBot response: " + json.dumps(diagnostic, separators=(",", ":")), file=sys.stderr)


def extract(response_path, result_path, requested_model):
    try:
        response = json.loads(Path(response_path).read_text())
    except (OSError, ValueError, RecursionError):
        diagnose("malformed_json")
        return 1
    if not isinstance(response, dict):
        diagnose("invalid_response")
        return 1

    status = response.get("status")
    details = response.get("incomplete_details")
    reason = details.get("reason") if isinstance(details, dict) else None
    if status == "incomplete" and reason == "max_output_tokens":
        diagnose("output_token_limit", response)
        return 10
    if status == "incomplete":
        diagnose("incomplete_response", response)
        return 1
    if status not in (None, "completed") or response.get("error"):
        diagnose("unsuccessful_response", response)
        return 1

    output = response.get("output")
    if not isinstance(output, list):
        diagnose("invalid_response", response)
        return 1
    parts = []
    for item in output:
        if not isinstance(item, dict) or item.get("type") != "message":
            continue
        if item.get("status") not in (None, "completed"):
            diagnose("incomplete_message", response)
            return 1
        content = item.get("content")
        if not isinstance(content, list):
            continue
        for part in content:
            if isinstance(part, dict) and part.get("type") == "output_text":
                text = part.get("text")
                if isinstance(text, str) and text.strip():
                    parts.append(text)
    message = format_markdown("\n".join(parts))
    if not message:
        diagnose("empty_output", response)
        return 1

    model = response.get("model")
    if not isinstance(model, str) or not model.strip():
        model = requested_model
    Path(result_path).write_text(json.dumps({"message": message, "model": model}))
    return 0


if __name__ == "__main__":
    sys.exit(extract(*sys.argv[1:]))
