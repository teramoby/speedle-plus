+++
title = "ポリシー検出"
description = "分離されたテスト環境でアクセスリクエストを記録し、認可ポリシーを生成および確認"
weight = 4
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
tags = ["policy", "core", "discovery"]
categories = ["docs"]
bref = "アクセスリクエストを記録し、認可ポリシーを生成して、通常の認可チェックに戻します。"
lastmod = 2026-10-29T11:32:30+08:00
+++

## ポリシー検出の役割

検出モードはリクエストを記録し、記録操作が成功した場合に理由 5 で `allowed=true` を返します。これは既存のポリシーが操作を認可したことを証明するものではありません。分離されたテスト環境でのみ使用し、生成されたグラントを確認してから、通常の `is-allowed` による強制を復元してください。アサーションやストレージのエラーは引き続き適切に処理する必要があり、エラーを認可と見なしてはなりません。

## 再現可能なローカルサンプルの開始

[クイックスタート](/ja/quick-start/) の手順でバイナリをビルドし、チェックアウトのルートからすべてのターミナルを実行します。開始前に新しいファイルを使用し、前のページのフィクスチャを停止してください。

<!-- docs-check: setup -->
```bash
printf '{}\n' > docs-policies.json
```

ターミナル 1:

<!-- docs-check: pms -->
```bash
./bin/speedle-pms --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6733 --insecure=true
```

ターミナル 2:

<!-- docs-check: ads -->
```bash
./bin/speedle-ads --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6734 --insecure=true
```

両方を実行したままにします。ターミナル 3 で、1 つのリクエストを記録します:

<!-- docs-check: discover-record -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"observed","resource":"book","action":"read"}' \
  http://127.0.0.1:6734/authz-check/v1/discover
```

キャプチャされたリクエストを調査し、提案されるサービス定義を生成します:

<!-- docs-check: discover-list -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ discover request --service-name observed
```
<!-- docs-check: discover-export -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ discover policy --service-name observed > discovered-service.json
```

生成されたロール、アイデンティティドメイン、リソースを含め、`discovered-service.json` を確認します。この新しい例では `observed` という既存のサービスがないため直接インポートできます。既存のサービスの場合は、ブラインドで再作成するのではなくポリシーを調整してください。

<!-- docs-check: discover-import -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service --json-file discovered-service.json
```

## 通常の認可チェックへの復帰と検証

再度 `is-allowed` を呼び出します。ADS がポリシーの更新を読み込んだ後、Alice は許可され、Bob は拒否される必要があります:

<!-- docs-check: discover-allow -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"observed","resource":"book","action":"read"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```
<!-- docs-check: discover-deny -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"bob"}]},"serviceName":"observed","resource":"book","action":"read"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```

両方のサービスを Ctrl+C で停止します。gRPC では生成されたクライアントで `Discover`/`IsAllowed` を使用し、組み込み Go では `evaluator.Discover(ctx)`/`evaluator.IsAllowed(ctx)` を使用します。CLI オプションについては `./bin/spctl discover --help` を実行してください。実際の本番アプリケーションを `/discover` に向けたままにしないでください。
