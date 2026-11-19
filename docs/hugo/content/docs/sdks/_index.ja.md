+++
title = "SDK"
description = "Java、Python、Go、.NET、Rust、Node.js/TypeScript 向けの Speedle 認可チェック用クライアントライブラリ"
weight = 305
draft = false
toc = true
linksection = true
tocheading = "h2"
tags = ["sdk"]
categories = ["docs"]
bref = "Speedle 決定 API の 6 言語クライアントライブラリ。セルフホストの Speedle ADS に対応しています。"
lastmod = 2026-11-10T14:45:43+08:00
+++

Speedle SDK は、決定 API `POST /authz-check/v1/is-allowed` のクライアントライブラリです。セルフホストの Speedle ADS で動作します。各言語ページではパッケージの追加方法を説明し、セルフホスト環境向けにテスト済みの完全なプログラムを提供しています。ソースコードと完全なリファレンスは [teramoby/speedle-sdk](https://github.com/teramoby/speedle-sdk) にあります。これらのサンプルで PMS と ADS を実行する方法については、[デプロイ](/ja/docs/deployment/) と [Docker イメージ](/ja/docs/deployment/docker/) を参照してください。

## パッケージ

6 つの SDK はすべてバージョン 1.9.0 で公開されています。各言語のインストールコマンドは各ページに記載されています。

- [Java](/ja/docs/sdks/java/): Maven Central `io.speedle:speedle-sdk`、公開済み。
- [Python](/ja/docs/sdks/python/): PyPI `speedle`、公開済み。
- [Go](/ja/docs/sdks/go/): Go モジュール `github.com/teramoby/speedle-sdk/go`、公開済み。
- [C#/.NET](/ja/docs/sdks/dotnet/): NuGet `Speedle.Sdk`、公開済み。
- [Rust](/ja/docs/sdks/rust/): crates.io `speedle`、公開済み。
- [Node.js / TypeScript](/ja/docs/sdks/node/): npm `@speedle/sdk`、公開済み。

## ランタイムサポート

各 SDK は 2026-11-19 を終点とする過去 3 年間にリリースされた言語またはランタイムのバージョンをサポートします。より古いバージョンも動作する場合があります。宣言された最小バージョンと CI のビルド・テスト対象バージョンは [speedle-sdk README](https://github.com/teramoby/speedle-sdk#runtime-support) に記載されています。

| SDK | 最小バージョン | CI テストバージョン |
| --- | --- | --- |
| Go | Go 1.26（v1.9.0）、v1.9.1 から Go 1.22 | 1.22, 1.27 |
| Python | Python 3.10 | 3.10, 3.11, 3.12, 3.13, 3.14, 3.15 |
| Node.js | Node.js 22 | 22, 24, 26 |
| Java | Java 17 | 17, 21, 25, 27 |
| .NET | .NET 8 | net8.0, net10.0, net11.0 |
| Rust | Rust 1.75 | 1.75, stable |

公開済みの Go モジュールは v1.9.0 で Go 1.26 を宣言しています。Go SDK が宣言する最低バージョンは v1.9.1 から Go 1.22 になります。

## 機能

| 機能 | ステータス |
| --- | --- |
| `isAllowed`: 認可決定を返す | 6 つの SDK すべて |
| `check`: 認可決定と理由（該当するポリシーがない等）を返す | 6 つの SDK すべて |
| セルフホストの Speedle ADS | 6 つの SDK すべて |

メソッド名は各言語の命名規則に従っています。Python と Rust では `is_allowed` と `check`、Go では `IsAllowed` と `Check`、.NET では `IsAllowedAsync` と `CheckAsync` です。

付与されたアクセス許可の照会、診断、ポリシー管理などのその他の操作には、REST API（[認可決定 API](/ja/docs/api/decision_api/) および [ポリシー管理 API](/ja/docs/api/management_api/)）を使用してください。

<a id="example-policy"></a>
## ポリシー例

すべてのサンプルは、サービス `docs` 内のリソース `/docs/public` に対して、ユーザー `alice` が `get` および `delete` を実行できるかどうかを確認します。ローカルの Speedle からドキュメント通りの回答を得るには、[クイックスタート](/ja/quick-start/) の手順に従ってビルドし、ポート 6733 で PMS を、ポート 6734 で ADS を起動してから、サービスとポリシーを作成します:

```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service docs
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user alice get,list /docs/public" --service-name=docs
```

ローカルの例は、認証情報なしで `http://localhost:6734` に接続します。SDK はループバックホストに対してのみ平文の `http://` を受け入れます。他の ADS には HTTPS を使用してください。
