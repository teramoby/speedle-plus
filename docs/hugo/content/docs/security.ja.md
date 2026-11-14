+++
title = "セキュリティ"
description = "Speedle の REST および gRPC エンドポイントに TLS、相互 TLS、およびアクセス制御を構成。"
weight = 310
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
tags = ["security", "guide", "tls", "https"]
categories = ["docs"]
bref = ""
lastmod = 2026-10-29T11:53:01+08:00
+++

<a id="protect-both-api-transports"></a>
## 両方の API トランスポートを保護

独立して動作する PMS と ADS は **REST と gRPC** を公開します。現在、両者とも同じ `--insecure`、`--cert`、`--key`、`--client-cert`、および `--force-client-cert` トランスポート設定を使用します。証明書または信頼設定が無効な場合、起動は失敗します。セキュアモードがプレーンテキストにフォールバックすることはありません。

TLS は接続を暗号化し、サーバーを認証します。認証されたクライアントを要求するには、`--client-cert` でクライアント CA を提供し、**かつ** `--force-client-cert=true` を設定します。そのフラグがない場合、構成された CA はクライアントが証明書を提供した場合にのみクライアント証明書を検証します。TLS だけでは管理者の認可や Bearer トークンの検証は提供されません。管理、診断、および検出エンドポイントには、認証済みゲートウェイまたは同等のアクセス制御を使用してください。`--enable-authz` フラグは、組み込みのエンドポイント認可ポリシーを提供するものではありません。

## リッスンアドレスと互換性

| サービス | デフォルト REST アドレス | デフォルト gRPC アドレス |
| --- | --- | --- |
| PMS | `0.0.0.0:6733` | `--grpc-endpoint` が設定されるまで無効 |
| ADS | `0.0.0.0:6734` | `--grpc-endpoint` が設定されるまで無効 |

REST と gRPC のリッスンアドレスは個別に構成されます。`--endpoint` のみを設定しても gRPC は有効になりません。PMS 用には `--grpc-endpoint 127.0.0.1:50001`、ADS 用には `--grpc-endpoint 127.0.0.1:50002` を明示的に指定してください。`127.0.0.1:0` は分離テスト用にエフェメラルポートを要求します。必要な場合は IPv6 アドレスを明示的に指定してください。

構成の優先順位は、コマンドライン引数、`SPDL_*` 環境変数、JSON 構成、デフォルト値の順です。gRPC のオーバーライドは `--grpc-endpoint`、`SPDL_GRPC_ENDPOINT`、および `serverConfig.grpcEndpoint` です。RPC 名前空間とクライアント生成については、[現在の proto 契約](/ja/docs/api/) を参照してください。

## ローカルテスト用証明書の生成

[クイックスタート](/ja/quick-start/) に従ってバイナリをビルドします。リポジトリのルートから、Go 1.27.4（または同じリリースブランチの新しいパッチバージョン）を使用してローカル CA と個別のサーバー/クライアント証明書を生成します。サーバー証明書には `localhost`、`127.0.0.1`、および `::1` の SAN が含まれます。秘密鍵の権限は 0600 です。これらは **24時間有効なデモ用証明書** であり、本番用の資格情報ではありません。ジェネレーターは既存のファイルの上書きを拒否します。再生成する場合は新しい出力ディレクトリを選択してください。

<!-- docs-check: certificates -->
```bash
go run ./website/tests/examples/certificates --out ./tls
```

PMS と ADS 用に空のストアファイルを準備します:

<!-- docs-check: setup -->
```bash
printf '{}
' > tls-policies.json
```

## mTLS を使用して PMS を起動

クライアント証明書を必須とする TLS で PMS を起動します:

<!-- docs-check: secure-pms -->
```bash
./bin/speedle-pms   --store-type file   --filestore-loc ./tls-policies.json   --endpoint 127.0.0.1:6733   --grpc-endpoint 127.0.0.1:0   --cert ./tls/server.crt   --key ./tls/server.key   --client-cert ./tls/ca.crt   --force-client-cert=true
```

## mTLS を使用して ADS を起動

クライアント証明書を必須とする TLS で ADS を起動します:

<!-- docs-check: secure-ads -->
```bash
./bin/speedle-ads   --store-type file   --filestore-loc ./tls-policies.json   --endpoint 127.0.0.1:6734   --grpc-endpoint 127.0.0.1:0   --cert ./tls/server.crt   --key ./tls/server.key   --client-cert ./tls/ca.crt   --force-client-cert=true
```

## spctl で接続をテスト

spctl を使用してサービスを作成します:

<!-- docs-check: secure-cli -->
```bash
./bin/spctl   --pms-endpoint https://127.0.0.1:6733/policy-mgmt/v1/   --cacert ./tls/ca.crt   --cert ./tls/client.crt   --key ./tls/client.key   create service bookstore
```

## curl で接続をテスト

curl を使用して認可決定を要求します:

<!-- docs-check: secure-curl -->
```bash
curl --fail --silent --show-error   --cacert ./tls/ca.crt   --cert ./tls/client.crt   --key ./tls/client.key   --header 'Content-Type: application/json'   --data '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"bookstore","resource":"book","action":"read"}'   https://127.0.0.1:6734/authz-check/v1/is-allowed
```
