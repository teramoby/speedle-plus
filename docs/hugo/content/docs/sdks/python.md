+++
title = "Python SDK"
description = "Add the Speedle Python SDK and check authorization against a local Speedle or Speedle Cloud."
weight = 20
aliases = ["/docs/api/sdk-python/"]
draft = false
toc = true
tocheading = "h2"
tags = ["api", "sdk"]
categories = ["docs"]
bref = "Install speedle 0.1.0 with pip or uv, then call is_allowed and check against a local Speedle or Speedle Cloud."
+++

The Python SDK needs Python 3.10 or later; its only runtime dependency is `httpx`. It has a sync `Client` and an asyncio `AsyncClient` with the same arguments. Package: [`speedle`](https://pypi.org/project/speedle/) 0.1.0 on PyPI. Reference: [speedle-sdk/python](https://github.com/teramoby/speedle-sdk/tree/main/python).

The examples check the policy described in the [SDKs overview](/docs/sdks/#example-policy).

## Add the dependency

With pip, in a virtual environment:

```bash
python3 -m venv .venv
. .venv/bin/activate
pip install speedle==0.1.0
```

Or with uv, in a uv project:

```bash
uv add speedle==0.1.0
```

## Call a local Speedle

Start Speedle and create the example policy as described in the [SDKs overview](/docs/sdks/#example-policy). Save this program as `main.py`:

```python
import sys

from speedle import Auth, Client, SpeedleError

try:
    with Client(
        backend="speedle-plus",
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

## Call Speedle Cloud

> **Speedle Cloud is coming soon.**

The program reads the service name from `SPEEDLE_SERVICE` and the service's token from `SPEEDLE_TOKEN`. `SPEEDLE_ENDPOINT` overrides the default endpoint `https://cloud.speedle.io`. Replace `main.py` with:

```python
import os
import sys

from speedle import Auth, Client, SpeedleError

try:
    with Client(
        backend="speedle-cloud",
        endpoint=os.environ.get("SPEEDLE_ENDPOINT", "https://cloud.speedle.io"),
        auth=Auth.bearer(os.environ["SPEEDLE_TOKEN"]),
        service_name=os.environ["SPEEDLE_SERVICE"],
    ) as speedle:
        allowed = speedle.is_allowed("alice", "get", "/docs/public")
        print(f"alice get /docs/public: allowed={allowed}")

        decision = speedle.check("alice", "delete", "/docs/public")
        print(f"alice delete /docs/public: allowed={decision.allowed} reason={decision.reason!r}")
except SpeedleError as e:
    print(f"Speedle error: {e}", file=sys.stderr)
    sys.exit(1)
```

Set the environment and run it. `read -rs` keeps the token out of your shell history and screen:

```bash
export SPEEDLE_SERVICE=docs
read -rs SPEEDLE_TOKEN && export SPEEDLE_TOKEN
python main.py
```

Output, when the service has the example policy:

```text
alice get /docs/public: allowed=True
alice delete /docs/public: allowed=False reason=<Reason.NO_APPLICABLE_POLICIES: 3>
```

The token is a server-side secret. Never ship it to browsers, mobile apps or desktop clients.

## Error handling

A denied request is not an error: `is_allowed` returns `False`, and `check` returns a decision with `allowed` false and the reason, as in the `delete` call above. With the default settings, when a call cannot return a decision, the SDK raises `SpeedleError`. The examples catch it, print `Speedle error: <message>` to standard error and exit with status 1.
