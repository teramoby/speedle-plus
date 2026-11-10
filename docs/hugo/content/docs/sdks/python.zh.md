+++
title = "Python SDK"
description = "添加 Speedle Python SDK，向本地 Speedle 或 Speedle Cloud 发起授权检查。"
weight = 20
aliases = ["/zh/docs/api/sdk-python/"]
draft = false
toc = true
tocheading = "h2"
tags = ["api", "sdk"]
categories = ["docs"]
bref = "用 pip 或 uv 安装 speedle 0.2.0，再向本地 Speedle 或 Speedle Cloud 调用 is_allowed 与 check。"
lastmod = 2026-11-08T11:21:16+08:00
+++

Python SDK 需要 Python 3.10 或更高版本，唯一的运行时依赖是 `httpx`。它提供同步的 `Client` 和 asyncio 的 `AsyncClient`，两者参数相同。软件包：PyPI 上的 [`speedle`](https://pypi.org/project/speedle/) 0.2.0。参考文档：[speedle-sdk/python](https://github.com/teramoby/speedle-sdk/tree/main/python)。

示例使用的策略见 [SDK 概览](/docs/sdks/#示例策略)。

## 添加依赖

使用 pip，在虚拟环境中安装：

```bash
python3 -m venv .venv
. .venv/bin/activate
pip install speedle==0.2.0
```

或者使用 uv，在 uv 项目中添加：

```bash
uv add speedle==0.2.0
```

## 调用本地 Speedle

按 [SDK 概览](/docs/sdks/#示例策略)中的说明启动 Speedle 并创建示例策略。将以下程序保存为 `main.py`：

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

在虚拟环境中用 `python main.py` 运行，或者使用 uv：

```bash
uv run main.py
```

输出：

```text
alice get /docs/public: allowed=True
alice delete /docs/public: allowed=False reason=<Reason.NO_APPLICABLE_POLICIES: 3>
```

## 调用 Speedle Cloud

> **Speedle Cloud 即将上线。**

程序从 `SPEEDLE_SERVICE` 读取服务名，从 `SPEEDLE_TOKEN` 读取该服务的令牌。`SPEEDLE_ENDPOINT` 可覆盖默认端点 `https://cloud.speedle.io`。将 `main.py` 替换为：

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

设置环境变量后运行。`read -rs` 不会把令牌写入 shell 历史，也不会显示在屏幕上：

```bash
export SPEEDLE_SERVICE=docs
read -rs SPEEDLE_TOKEN && export SPEEDLE_TOKEN
python main.py
```

服务中已有示例策略时，输出为：

```text
alice get /docs/public: allowed=True
alice delete /docs/public: allowed=False reason=<Reason.NO_APPLICABLE_POLICIES: 3>
```

令牌是服务端密钥，切勿放进浏览器、移动应用或桌面客户端。

## 错误处理

授权请求返回 Deny 并不代表调用出错：`is_allowed` 返回 `False`，`check` 返回 `allowed` 为 false 的结果及原因，如上文的 `delete` 调用。在默认设置下，调用无法得出授权结果时，SDK 抛出 `SpeedleError`。示例捕获该异常，向标准错误输出 `Speedle error: <消息>`，并以状态码 1 退出。
