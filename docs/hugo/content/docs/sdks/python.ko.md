+++
title = "Python SDK"
description = "Speedle Python SDK를 추가하고 로컬 Speedle에 대해 인가 확인 수행"
weight = 20
aliases = ["/ko/docs/api/sdk-python/"]
draft = false
toc = true
tocheading = "h2"
tags = ["api", "sdk"]
categories = ["docs"]
bref = "pip 또는 uv로 speedle 0.2.0을 설치하고 로컬 Speedle에 대해 is_allowed 및 check를 호출합니다."
lastmod = 2026-11-07T19:43:15+08:00
+++

Python SDK는 Python 3.10 이상이 필요합니다. 런타임 의존성은 `httpx`뿐입니다. 동일한 인수를 사용하는 동기식 `Client`와 asyncio용 `AsyncClient`를 모두 제공합니다. 패키지: PyPI의 [`speedle`](https://pypi.org/project/speedle/) 0.2.0. 참조 문서: [speedle-sdk/python](https://github.com/teramoby/speedle-sdk/tree/main/python).

예제에서는 [SDK 개요](/docs/sdks/#example-policy)에서 설명한 정책을 확인합니다.

## 의존성 추가

가상 환경에서 pip를 사용하는 경우:

```bash
python3 -m venv .venv
. .venv/bin/activate
pip install speedle==0.2.0
```

또는 uv 프로젝트에서 uv를 사용하는 경우:

```bash
uv add speedle==0.2.0
```

## 로컬 Speedle 호출

[SDK 개요](/docs/sdks/#example-policy)의 설명에 따라 Speedle을 시작하고 예제 정책을 생성합니다. 이 프로그램을 `main.py`로 저장합니다:

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

가상 환경에서 `python main.py`를 실행하거나 uv로 실행합니다:

```bash
uv run main.py
```

출력:

```text
alice get /docs/public: allowed=True
alice delete /docs/public: allowed=False reason=<Reason.NO_APPLICABLE_POLICIES: 3>
```


## 오류 처리

거부된 요청은 오류가 아닙니다. 위의 `delete` 호출과 같이 `is_allowed`는 `False`를 반환하고, `check`는 `allowed`가 false이며 이유를 포함하는 결정을 반환합니다. 기본 구성에서 호출이 결정을 반환할 수 없는 경우 SDK는 `SpeedleError`를 발생시킵니다. 예제에서는 이를 포착하여 표준 오류로 `Speedle error: <message>`를 출력하고 종료 상태 1로 종료합니다.
