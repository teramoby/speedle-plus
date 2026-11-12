+++
title = "API リファレンス"
description = "公開されている Speedle Cloud API の読み取り専用 Swagger UI です。Try it out は無効です。"
date = 2026-11-12T10:00:00+08:00
lastmod = 2026-11-12T10:00:00+08:00
weight = 35
draft = false
toc = false
categories = ["docs"]
customcss = true
customcssfile = "swagger-custom.css"
bref = "公開 Cloud API を Swagger UI で表示します。Try it out は無効です。"
+++

このページは公開されている Speedle Cloud API を表示します。仕様の本文は、このサイトのどの言語でも英語です。Try it out はオフなので、アプリケーション トークンをブラウザーに入力しません。

表示されるファイルは[同梱している公開サブセット](/openapi/cloud-public.yaml)です。Speedle Cloud が公開 OpenAPI 文書を公開するまでのプレースホルダーで、認可チェックのパスだけを含みます。[Cloud ドキュメントの索引](/ja/docs/cloud/) からリンクしている、リポジトリにチェックインされた完全な仕様ではありません。

[認可 API プレビュー](/ja/docs/cloud/authorization-api/) は IsAllowed 呼び出しを説明します。[制限とエラー](/ja/docs/cloud/limits-and-errors/) はクォータとエラー応答を列挙します。

{{< swagger-spec url="/openapi/cloud-public.yaml" tryitout="false" >}}
