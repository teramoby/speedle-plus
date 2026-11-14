+++
title = "Python SDK"
description = "Speedle Python SDK を追加し、ローカルの Speedle に対して認可チェックを実行"
weight = 20
aliases = ["/ja/docs/api/sdk-python/"]
draft = false
toc = true
tocheading = "h2"
tags = ["api", "sdk"]
categories = ["docs"]
bref = "pip または uv で speedle 0.2.0 をインストールし、ローカルの Speedle に対して is_allowed と check を呼び出します。"
lastmod = 2026-11-07T19:43:15+08:00
+++

Python SDK は Python 3.10 以降が必要です。ランタイム依存関係は `httpx` のみです。同じ引数を持つ同期の `Client` と asyncio の `AsyncClient` を備えています。パッケージ: PyPI の [`speedle`](https://pypi.org/project/speedle/) 0.2.0。リファレンス: [speedle-sdk/python](https://github.com/teramoby/speedle-sdk/tree/main/python)。

サンプルでは、[SDK 概要](/ja/docs/sdks/#example-policy) で説明されているポリシーをチェックします。

## 依存関係の追加

仮想環境で pip を使用する場合:

```bash
python3 -m venv .venv
. .venv/bin/activate
pip install speedle==0.2.0
```

または uv プロジェクトで uv を使用する場合:

```bash
uv add speedle==0.2.0
```

## ローカルの Speedle の呼び出し

[SDK 概要](/ja/docs/sdks/#example-policy) の説明に従って Speedle を起動し、サンプルポリシーを作成します。このプログラムを `main.py` として保存します:

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

仮想環境で `python main.py` を実行するか、uv で実行します:

```bash
uv run main.py
```

出力:

```text
alice get /docs/public: allowed=True
alice delete /docs/public: allowed=False reason=<Reason.NO_APPLICABLE_POLICIES: 3>
```


## エラー処理

拒否されたリクエストはエラーではありません。上記の `delete` 呼び出しのように、`is_allowed` は `False` を返し、`check` は `allowed` が false で理由を含む決定を返します。デフォルト設定では、呼び出しが決定を返せない場合、SDK は `SpeedleError` を発生させます。サンプルではそれをキャッチし、標準エラー出力に `Speedle error: <message>` を出力して終了ステータス 1 で終了します。
