+++
title = "Python SDK"
description = "Add the Speedle Python SDK and check authorization against a local Speedle."
weight = 20
aliases = ["/docs/api/sdk-python/"]
draft = false
toc = true
tocheading = "h2"
tags = ["api", "sdk"]
categories = ["docs"]
bref = "Install speedle 0.2.0 with pip or uv, then call is_allowed and check against a local Speedle."
lastmod = 2026-11-07T19:43:15+08:00
+++

The Python SDK needs Python 3.10 or later; its only runtime dependency is `httpx`. It has a sync `Client` and an asyncio `AsyncClient` with the same arguments. Package: [`speedle`](https://pypi.org/project/speedle/) 0.2.0 on PyPI. Reference: [speedle-sdk/python](https://github.com/teramoby/speedle-sdk/tree/main/python).

The examples check the policy described in the [SDKs overview](/docs/sdks/#example-policy).

## Add the dependency

With pip, in a virtual environment:

```bash
python3 -m venv .venv
. .venv/bin/activate
pip install speedle==0.2.0
```

Or with uv, in a uv project:

```bash
uv add speedle==0.2.0
```

## Call a local Speedle

Start Speedle and create the example policy as described in the [SDKs overview](/docs/sdks/#example-policy). Save this program as `main.py`:

```python
import sys

from speedle import Auth, Client, SpeedleError

try:
    with Client(
        backend="speedle",
        endpoint="http://localhost:6734",
        auth=Auth.none(),
        service_name="docs",
    ) as speedle:
        allowed = speedle.is_allowed("alice", "get", "/docs/public")
        print(f"alice get /docs/public: allowed={allowed}")

        decision = speedle.check("alice", "delete", "/docs/public")
        print(f"alice delete /docs/public: allowed={decision.allowed} reason={decision.reason!r}")
except SpeedleError as e:
    print(f"Speedle error: {e}", file=sys.stderr)
    sys.exit(1)
```

Run it with `python main.py` in the virtual environment, or with uv:

```bash
uv run main.py
```

Output:

```text
alice get /docs/public: allowed=True
alice delete /docs/public: allowed=False reason=<Reason.NO_APPLICABLE_POLICIES: 3>
```


## Error handling

A denied request is not an error: `is_allowed` returns `False`, and `check` returns a decision with `allowed` false and the reason, as in the `delete` call above. With the default settings, when a call cannot return a decision, the SDK raises `SpeedleError`. The examples catch it, print `Speedle error: <message>` to standard error and exit with status 1.
