"""Seal a bounded review for the same-run publisher.

Never log review text, secret values, or which secret matched.
"""

import json
import os
from pathlib import Path
import re
import sys


MAX_BYTES = 262144
# Shorter values collide with JSON keys and ordinary review words, which would
# fail every publication. Longer values are still scanned.
MIN_SECRET_LENGTH = 8
RETRY_NOTICE = (
    "\n\n_Validation gap: ReviewBot retried after an output-token limit "
    "with unified=3 diff context (initial unified=80)._"
)


def _nonempty_string(value):
    return isinstance(value, str) and value != ""


def seal(result_path, artifact_path):
    destination = Path(artifact_path)
    # A failed attempt must never leave a previously valid report to upload.
    try:
        destination.unlink(missing_ok=True)
        source = Path(result_path)
        if source.stat().st_size > MAX_BYTES:
            raise ValueError()
        result = json.loads(source.read_text(encoding="utf-8"))
        secrets = json.loads(os.environ["REVIEWBOT_SECRETS"])
        if not isinstance(secrets, dict) or not secrets:
            raise ValueError()
        # The job token is not always present under one spelling. Scan the
        # explicit job token and every value in the secrets context.
        secrets["GITHUB_TOKEN"] = os.environ["GH_TOKEN"]
        if not _nonempty_string(secrets.get("GITHUB_TOKEN")):
            raise ValueError()
        # Either provider key may be unset. At least one model credential
        # must be part of the scan, or the guard would not be looking at it.
        if not (_nonempty_string(secrets.get("ARK_API_KEY"))
                or _nonempty_string(secrets.get("DEEPSEEK_API_KEY"))):
            raise ValueError()
        if not isinstance(result, dict) or set(result) != {"message", "model"}:
            raise ValueError()
        message, model = result["message"], result["model"]
        if not isinstance(message, str) or not message.strip() or len(message.encode("utf-8")) > 240000:
            raise ValueError()
        if not isinstance(model, str) or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._:/-]{0,199}", model):
            raise ValueError()
        if os.environ["REVIEWBOT_RETRIED"] == "true":
            message += RETRY_NOTICE
        elif os.environ["REVIEWBOT_RETRIED"] != "false":
            raise ValueError()
        report = {"version": 1, "message": message, "model": model}
        for field, variable in (
            ("run_id", "GITHUB_RUN_ID"),
            ("run_attempt", "GITHUB_RUN_ATTEMPT"),
            ("pull_number", "PULL_NUMBER"),
            ("head_sha", "HEAD_SHA"),
            ("command", "COMMAND"),
        ):
            report[field] = os.environ[variable]
        for field in ("run_id", "run_attempt", "pull_number"):
            if not re.fullmatch(r"[1-9][0-9]{0,15}", report[field]):
                raise ValueError()
        if (not re.fullmatch(r"[a-f0-9]{40}", report["head_sha"])
                or report["command"] not in {"review", "recheck", "explain"}):
            raise ValueError()
        # Inspect decoded fields and the serialized JSON. Escapes must not hide
        # a credential. No value, name, substring, digest or encoding is logged.
        payload = json.dumps(report, ensure_ascii=False)
        for name, secret in secrets.items():
            # GitHub secret names start with a letter or underscore. The
            # secrets context also includes lowercase github_token.
            if not isinstance(name, str) or not re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*", name):
                print("ReviewBot reason_code=secret_name_invalid", file=sys.stderr)
                return 1
            if not isinstance(secret, str):
                raise ValueError()
            if (len(secret) >= MIN_SECRET_LENGTH
                    and any(secret in text for text in (payload, *report.values()) if isinstance(text, str))):
                print("ReviewBot reason_code=secret_match", file=sys.stderr)
                return 1
        if len(payload.encode("utf-8")) > MAX_BYTES:
            raise ValueError()
        destination.parent.mkdir(parents=True, exist_ok=True)
        destination.write_text(payload, encoding="utf-8")
        destination.chmod(0o600)
        return 0
    except (OSError, KeyError, ValueError, TypeError, RecursionError, json.JSONDecodeError):
        print("ReviewBot reason_code=invalid_handoff", file=sys.stderr)
        return 1


if __name__ == "__main__":
    if len(sys.argv) != 3:
        print("ReviewBot reason_code=invalid_handoff", file=sys.stderr)
        sys.exit(1)
    sys.exit(seal(sys.argv[1], sys.argv[2]))
